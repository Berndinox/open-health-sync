// SPDX-License-Identifier: AGPL-3.0-only

package withings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/soma-intelligence/open-health-sync/internal/connector"
)

// Scopes requested at consent.
var Scopes = []string{"user.metrics", "user.activity", "user.sleepevents"}

// errNotConfigured reports a call that needs the client keys before they were
// set.
var errNotConfigured = errors.New("withings: client id and secret are not configured")

// Withings inner status codes of the token endpoint.
//
//	0    success
//	301  invalid or expired refresh token (permanent)
//	401  invalid token (permanent)
//	2553 revoked token (permanent)
//	601  rate limit (transient)
func permanentTokenStatus(status int) bool {
	return status == 301 || status == 401 || status == 2553
}

// AuthorizeURL returns the Withings consent page URL.
func (c *Connector) AuthorizeURL(state, redirect string) (string, error) {
	id, _ := c.credentials()
	if id == "" {
		return "", errNotConfigured
	}
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", id)
	q.Set("scope", strings.Join(Scopes, ","))
	q.Set("redirect_uri", redirect)
	q.Set("state", state)
	return c.accountBase + "/oauth2_user/authorize2?" + q.Encode(), nil
}

// Exchange trades an authorization code for tokens.
func (c *Connector) Exchange(ctx context.Context, code, redirect string) (connector.Tokens, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirect)
	t, err := c.requestToken(ctx, form)
	if err != nil {
		return connector.Tokens{}, fmt.Errorf("withings: exchange: %w", err)
	}
	return t, nil
}

// Refresh trades a refresh token for new tokens. Withings rotates refresh
// tokens: the caller must store the returned pair. A refresh token Withings
// rejects for good (invalid_grant, status 301, 401 or 2553) gives
// connector.ErrNeedsReauth.
func (c *Connector) Refresh(ctx context.Context, t connector.Tokens) (connector.Tokens, error) {
	if t.Refresh == "" {
		return connector.Tokens{}, fmt.Errorf("withings: refresh: no refresh token: %w", connector.ErrNeedsReauth)
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", t.Refresh)
	nt, err := c.requestToken(ctx, form)
	if err != nil {
		return connector.Tokens{}, fmt.Errorf("withings: refresh: %w", err)
	}
	if nt.ProviderUserID == "" {
		nt.ProviderUserID = t.ProviderUserID
	}
	if len(nt.Scopes) == 0 {
		nt.Scopes = t.Scopes
	}
	return nt, nil
}

// requestToken posts to the v2 token endpoint (action=requesttoken) and
// unwraps the {"status", "body"} envelope Withings answers with, HTTP 200 even
// on error.
func (c *Connector) requestToken(ctx context.Context, form url.Values) (connector.Tokens, error) {
	id, secret := c.credentials()
	if id == "" || secret == "" {
		return connector.Tokens{}, errNotConfigured
	}
	form.Set("action", "requesttoken")
	form.Set("client_id", id)
	form.Set("client_secret", secret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase+"/v2/oauth2", strings.NewReader(form.Encode()))
	if err != nil {
		return connector.Tokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpc.Do(req)
	if err != nil {
		return connector.Tokens{}, fmt.Errorf("token request: %w", err)
	}
	body, err := readBody(resp.Body, 1<<20)
	resp.Body.Close()
	if err != nil {
		return connector.Tokens{}, err
	}

	var env envelope
	jsonErr := json.Unmarshal(body, &env)
	// An OAuth style error, whatever the HTTP status: {"error":"invalid_grant"}.
	if jsonErr == nil && strings.Contains(env.Error, "invalid_grant") {
		return connector.Tokens{}, connector.ErrNeedsReauth
	}
	if resp.StatusCode == http.StatusUnauthorized || (resp.StatusCode == http.StatusBadRequest && strings.Contains(string(body), "invalid_grant")) {
		return connector.Tokens{}, connector.ErrNeedsReauth
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return connector.Tokens{}, connector.RateLimited(resp.Header, c.now())
	}
	if resp.StatusCode >= 400 {
		return connector.Tokens{}, fmt.Errorf("token endpoint HTTP %d", resp.StatusCode)
	}
	if jsonErr != nil {
		return connector.Tokens{}, fmt.Errorf("token endpoint: decode answer: %w", jsonErr)
	}
	if env.Status == 601 {
		return connector.Tokens{}, connector.RateLimited(resp.Header, c.now())
	}
	if env.Status != 0 {
		if permanentTokenStatus(env.Status) {
			return connector.Tokens{}, connector.ErrNeedsReauth
		}
		return connector.Tokens{}, fmt.Errorf("token endpoint status %d", env.Status)
	}

	var tb tokenBody
	if err := json.Unmarshal(env.Body, &tb); err != nil {
		return connector.Tokens{}, fmt.Errorf("token endpoint: decode body: %w", err)
	}
	if tb.AccessToken == "" {
		return connector.Tokens{}, errors.New("token endpoint: answer holds no access token")
	}
	t := connector.Tokens{
		Access:         tb.AccessToken,
		Refresh:        tb.RefreshToken,
		ProviderUserID: string(tb.UserID),
	}
	if tb.ExpiresIn > 0 {
		t.Expiry = c.now().Add(time.Duration(tb.ExpiresIn) * time.Second).UTC()
	}
	for _, s := range strings.Split(tb.Scope, ",") {
		if s = strings.TrimSpace(s); s != "" {
			t.Scopes = append(t.Scopes, s)
		}
	}
	return t, nil
}
