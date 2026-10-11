// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/soma-intelligence/open-health-sync/internal/tenant"
)

type mintTokenRequest struct {
	Scopes []string `json:"scopes"`
	// TTLSeconds is the lifetime: 0 or absent means one hour. At most 86400,
	// or 2592000 (30 days) when read is the only scope.
	TTLSeconds int64 `json:"ttl_seconds"`
}

type mintTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Scopes    []string  `json:"scopes"`
}

// errTTLTooLong is the one message for a lifetime over the scope's limit,
// whichever check catches it.
const errTTLTooLong = "ttl_seconds is too long for these scopes: at most 86400, or 2592000 when read is the only scope"

func (s *server) mountTokens(mux *http.ServeMux) {
	// Not wrapped in idempotent: replaying would mean storing the plaintext
	// token. A retried mint simply mints another token; both are valid.
	s.tenantRoute(mux, "POST /v1/users/{id}/tokens", s.mintToken)
}

func (s *server) mintToken(w http.ResponseWriter, r *http.Request) {
	p, id, ok := pathUser(w, r)
	if !ok {
		return
	}
	var req mintTokenRequest
	if !decodeJSON(w, r, &req, false) {
		return
	}
	if req.TTLSeconds < 0 {
		badRequest(w, "ttl_seconds must not be negative")
		return
	}
	if req.TTLSeconds > int64(tenant.MaxReadTokenTTL/time.Second) {
		badRequest(w, errTTLTooLong)
		return
	}
	token, exp, err := s.d.Store.MintUserToken(r.Context(), p.TenantID, id, req.Scopes, time.Duration(req.TTLSeconds)*time.Second)
	switch {
	case errors.Is(err, tenant.ErrInvalidScopes):
		badRequest(w, "scopes must be a non-empty list of upload, connect and read")
		return
	case errors.Is(err, tenant.ErrTTLTooLong):
		badRequest(w, errTTLTooLong)
		return
	case errors.Is(err, tenant.ErrTTLInvalid):
		badRequest(w, "ttl_seconds is too short")
		return
	case errors.Is(err, tenant.ErrNotFound):
		notFound(w, "user")
		return
	case err != nil:
		internalError(w, r, "api: mint token", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, mintTokenResponse{Token: token, ExpiresAt: exp.UTC(), Scopes: dedup(req.Scopes)})
}

func dedup(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
