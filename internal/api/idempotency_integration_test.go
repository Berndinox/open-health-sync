// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/soma-intelligence/open-health-sync/internal/auth"
	"github.com/soma-intelligence/open-health-sync/internal/db"
	"github.com/soma-intelligence/open-health-sync/internal/tenant"
)

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("connection reset") }
func (failingBody) Close() error             { return nil }

func TestIdempotentEdges(t *testing.T) {
	ctx := context.Background()
	pool, err := db.Open(ctx, db.TestDB(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	st := tenant.New(pool)
	ten, key, err := st.CreateTenant(ctx, "Acme")
	require.NoError(t, err)
	s := &server{d: Deps{Pool: pool, Store: st}}

	panics := true
	h := auth.Middleware(st.LookupKey, auth.TenantKey)(s.idempotent(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if panics {
			panic("boom")
		}
		writeJSON(w, http.StatusCreated, map[string]string{"ok": "yes"})
	})))
	req := func(body []byte, idem string) *http.Request {
		r := httptest.NewRequest("POST", "/v1/users", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+key)
		r.Header.Set(HeaderIdempotencyKey, idem)
		return r
	}

	// A panic in the handler releases the claim and keeps panicking.
	assert.PanicsWithValue(t, "boom", func() { h.ServeHTTP(httptest.NewRecorder(), req([]byte(`{}`), "p-1")) })
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM ohs_core.idempotency_keys WHERE tenant_id = $1`, ten.ID).Scan(&n))
	assert.Zero(t, n, "the claim is released, so a retry is not stuck on 409")
	panics = false
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req([]byte(`{}`), "p-1"))
	assert.Equal(t, http.StatusCreated, rec.Code)

	// Too large a body: 413. A broken body: 400.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req([]byte(strings.Repeat("x", maxJSONBody+1)), "p-2"))
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	r := req(nil, "p-3")
	r.Body = failingBody{}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
