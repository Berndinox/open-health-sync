// SPDX-License-Identifier: AGPL-3.0-only

package withings

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/soma-intelligence/open-health-sync/internal/config"
	"github.com/soma-intelligence/open-health-sync/internal/connector"
)

func TestKeysAndEnabled(t *testing.T) {
	c := New(Options{})
	assert.Equal(t, []string{"CLIENT_ID", "CLIENT_SECRET"}, c.RequiredKeys())
	assert.False(t, c.Enabled(connector.Config{"CLIENT_ID": "a"}))
	assert.False(t, c.Enabled(connector.Config{"CLIENT_ID": "a", "CLIENT_SECRET": ""}))
	assert.True(t, c.Enabled(connector.Config{"CLIENT_ID": "a", "CLIENT_SECRET": "b"}))

	reg := connector.NewRegistry()
	reg.Register(c)
	assert.Empty(t, reg.Enabled(map[string]config.ProviderKeys{"withings": {"CLIENT_ID": "a"}}))
	assert.Len(t, reg.Enabled(map[string]config.ProviderKeys{"withings": {"CLIENT_ID": "a", "CLIENT_SECRET": "b"}}), 1)
}

// Keys come from Configure (or Options), never from the environment.
func TestCredentials_OnlyConfigured(t *testing.T) {
	t.Setenv("OHS_WITHINGS_CLIENT_ID", "env-id")
	t.Setenv("OHS_WITHINGS_CLIENT_SECRET", "env-secret")
	c := New(Options{})
	id, secret := c.credentials()
	assert.Empty(t, id)
	assert.Empty(t, secret)
	_, err := c.AuthorizeURL("s", "r")
	assert.ErrorIs(t, err, errNotConfigured)

	assert.Error(t, c.Configure(connector.Config{"CLIENT_ID": "cfg-id"}), "both keys are required")
	require.NoError(t, c.Configure(connector.Config{"CLIENT_ID": "cfg-id", "CLIENT_SECRET": "cfg-secret"}))
	id, secret = c.credentials()
	assert.Equal(t, "cfg-id", id)
	assert.Equal(t, "cfg-secret", secret)
}

func TestBaseURLDefaultsAndOverride(t *testing.T) {
	c := New(Options{})
	assert.Equal(t, DefaultAPIBase, c.apiBase)
	assert.Equal(t, DefaultAccountBase, c.accountBase)
	c = New(Options{APIBase: "http://127.0.0.1:9999", AccountBase: "http://127.0.0.1:9998"})
	assert.Equal(t, "http://127.0.0.1:9999", c.apiBase)
	assert.Equal(t, "http://127.0.0.1:9998", c.accountBase)
}

func TestAuthorizeURL(t *testing.T) {
	f := newFakeWithings(t)
	raw, err := f.connector().AuthorizeURL("state-123", "http://localhost:8080/oauth/withings/callback")
	require.NoError(t, err)
	u, err := url.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, "/oauth2_user/authorize2", u.Path)
	q := u.Query()
	assert.Equal(t, "code", q.Get("response_type"))
	assert.Equal(t, "test-client-id", q.Get("client_id"))
	assert.Equal(t, "state-123", q.Get("state"))
	assert.Equal(t, "http://localhost:8080/oauth/withings/callback", q.Get("redirect_uri"))
	assert.Equal(t, "user.metrics,user.activity,user.sleepevents", q.Get("scope"))
}

func TestAuthorizeURL_NotConfigured(t *testing.T) {
	_, err := New(Options{}).AuthorizeURL("s", "r")
	assert.Error(t, err)
}

func TestExchange(t *testing.T) {
	f := newFakeWithings(t)
	tok, err := f.connector().Exchange(context.Background(), "code-1", "http://localhost/cb")
	require.NoError(t, err)
	assert.Equal(t, "fake-access-token-1", tok.Access)
	assert.Equal(t, "fake-refresh-token-1", tok.Refresh)
	assert.Equal(t, "424242", tok.ProviderUserID)
	assert.Equal(t, []string{"user.metrics", "user.activity", "user.sleepevents"}, tok.Scopes)
	assert.Equal(t, time.Date(2026, 3, 3, 3, 0, 0, 0, time.UTC), tok.Expiry, "now + expires_in")

	calls := f.calls("/v2/oauth2", "requesttoken")
	require.Len(t, calls, 1)
	form := calls[0].Form
	assert.Equal(t, "authorization_code", form.Get("grant_type"))
	assert.Equal(t, "code-1", form.Get("code"))
	assert.Equal(t, "http://localhost/cb", form.Get("redirect_uri"))
	assert.Equal(t, "test-client-id", form.Get("client_id"))
	assert.Equal(t, "test-client-secret", form.Get("client_secret"))
}

// Withings has sent userid as a number as well as a string.
func TestExchange_NumericUserID(t *testing.T) {
	f := newFakeWithings(t)
	f.TokenBody = `{"status":0,"body":{"userid":424242,"access_token":"a","refresh_token":"r","expires_in":10800,"scope":"user.metrics"}}`
	tok, err := f.connector().Exchange(context.Background(), "code", "http://localhost/cb")
	require.NoError(t, err)
	assert.Equal(t, "424242", tok.ProviderUserID)
}

func TestRefresh(t *testing.T) {
	f := newFakeWithings(t)
	tok, err := f.connector().Refresh(context.Background(), connector.Tokens{Refresh: "old-refresh", ProviderUserID: "424242"})
	require.NoError(t, err)
	assert.Equal(t, "fake-access-token-1", tok.Access)
	assert.Equal(t, "fake-refresh-token-1", tok.Refresh, "Withings rotates the refresh token")
	calls := f.calls("/v2/oauth2", "requesttoken")
	require.Len(t, calls, 1)
	assert.Equal(t, "refresh_token", calls[0].Form.Get("grant_type"))
	assert.Equal(t, "old-refresh", calls[0].Form.Get("refresh_token"))
}

// A refresh token Withings rejects for good sends the connection to
// needs_reauth: inner status 401 (and its siblings 301, 2553), or an OAuth
// invalid_grant, whatever the HTTP status.
func TestRevokedRefreshToken(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"inner status 401", 0, `{"status":401,"body":{},"error":"XRequestID: Not provided invalid_token: The access token provided is invalid"}`},
		{"inner status 301", 0, `{"status":301,"body":{}}`},
		{"inner status 2553 revoked", 0, `{"status":2553,"body":{}}`},
		{"invalid_grant in body", 0, `{"status":503,"body":{},"error":"invalid_grant"}`},
		{"HTTP 400 invalid_grant", http.StatusBadRequest, `{"error":"invalid_grant","error_description":"refresh token is invalid"}`},
		{"HTTP 401", http.StatusUnauthorized, `{}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeWithings(t)
			f.TokenStatus, f.TokenBody = c.status, c.body
			_, err := f.connector().Refresh(context.Background(), connector.Tokens{Refresh: "revoked"})
			require.ErrorIs(t, err, connector.ErrNeedsReauth)
		})
	}
}

// A rate limited token endpoint gives the typed rate limit error.
func TestRefresh_RateLimited(t *testing.T) {
	f := newFakeWithings(t)
	f.TokenStatus, f.TokenBody = http.StatusTooManyRequests, `{}`
	_, err := f.connector().Refresh(context.Background(), connector.Tokens{Refresh: "r"})
	_, ok := connector.AsRateLimit(err)
	assert.True(t, ok, "got %v", err)

	f = newFakeWithings(t)
	f.TokenBody = `{"status":601,"body":{}}`
	_, err = f.connector().Refresh(context.Background(), connector.Tokens{Refresh: "r"})
	_, ok = connector.AsRateLimit(err)
	assert.True(t, ok, "got %v", err)
}

// Transient token endpoint failures must not look like a revoked grant.
func TestRefresh_TransientIsNotReauth(t *testing.T) {
	for _, body := range []string{`{"status":601,"body":{}}`, `{"status":503,"body":{}}`} {
		f := newFakeWithings(t)
		f.TokenBody = body
		_, err := f.connector().Refresh(context.Background(), connector.Tokens{Refresh: "r"})
		require.Error(t, err)
		assert.NotErrorIs(t, err, connector.ErrNeedsReauth, body)
	}
	f := newFakeWithings(t)
	f.TokenStatus, f.TokenBody = http.StatusBadGateway, `bad gateway`
	_, err := f.connector().Refresh(context.Background(), connector.Tokens{Refresh: "r"})
	require.Error(t, err)
	assert.NotErrorIs(t, err, connector.ErrNeedsReauth)
}

func TestRefresh_KeepsUserIDWhenAbsent(t *testing.T) {
	f := newFakeWithings(t)
	f.TokenBody = `{"status":0,"body":{"access_token":"a2","refresh_token":"r2","expires_in":10800}}`
	tok, err := f.connector().Refresh(context.Background(), connector.Tokens{Refresh: "r", ProviderUserID: "424242", Scopes: []string{"user.metrics"}})
	require.NoError(t, err)
	assert.Equal(t, "424242", tok.ProviderUserID)
	assert.Equal(t, []string{"user.metrics"}, tok.Scopes)
}

func TestRefresh_NoRefreshTokenNeedsReauth(t *testing.T) {
	_, err := New(Options{ClientID: "a", ClientSecret: "b"}).Refresh(context.Background(), connector.Tokens{})
	require.ErrorIs(t, err, connector.ErrNeedsReauth)
}
