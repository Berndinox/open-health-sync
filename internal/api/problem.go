// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/soma-intelligence/open-health-sync/internal/auth"
)

// maxJSONBody caps a JSON request body.
const maxJSONBody = 1 << 20

func problem(w http.ResponseWriter, status int, detail string) {
	auth.WriteProblem(w, status, http.StatusText(status), detail)
}

func badRequest(w http.ResponseWriter, detail string) { problem(w, http.StatusBadRequest, detail) }

func notFound(w http.ResponseWriter, what string) {
	problem(w, http.StatusNotFound, what+" not found")
}

// internalError logs the cause (never sent) and answers 500.
func internalError(w http.ResponseWriter, r *http.Request, msg string, err error) {
	slog.ErrorContext(r.Context(), msg, "error", err, "route", r.Pattern)
	problem(w, http.StatusInternalServerError, "an internal error occurred")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// decodeJSON reads a JSON object body into v. An empty body leaves v as is
// when allowEmpty. It writes the problem and returns false on failure.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any, allowEmpty bool) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody))
	dec.DisallowUnknownFields()
	err := dec.Decode(v)
	if errors.Is(err, io.EOF) && allowEmpty {
		return true
	}
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		problem(w, http.StatusRequestEntityTooLarge, "the request body is too large")
		return false
	case err != nil:
		badRequest(w, "the request body is not valid JSON for this endpoint: "+jsonErrDetail(err))
		return false
	}
	if dec.More() {
		badRequest(w, "the request body holds more than one JSON value")
		return false
	}
	return true
}

func jsonErrDetail(err error) string {
	var typ *json.UnmarshalTypeError
	if errors.As(err, &typ) && typ.Field != "" {
		return "field " + typ.Field + " has the wrong type"
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "unexpected end of input"
	}
	// encoding/json names the unknown field; nothing from the body beyond it.
	return err.Error()
}
