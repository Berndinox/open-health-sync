// SPDX-License-Identifier: AGPL-3.0-only

package withings

import (
	"context"
	"net/url"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/soma-intelligence/open-health-sync/internal/connector"
)

const testCallback = "https://ohs.example.org/inbound/withings"

func subConn() connector.Connection {
	return connector.Connection{Provider: Slug, Tokens: connector.Tokens{Access: "fake-access-token-1", ProviderUserID: "424242"}}
}

func TestSubscribe_RequestShape(t *testing.T) {
	f := newFakeWithings(t)
	c := f.connector()
	require.NoError(t, c.Subscribe(context.Background(), subConn(), testCallback))

	calls := f.calls("/notify", "subscribe")
	require.Len(t, calls, 4, "one call per category")
	signed, err := c.SignedCallbackURL(testCallback, "424242")
	require.NoError(t, err)
	var applis []string
	for _, call := range calls {
		assert.Equal(t, "Bearer fake-access-token-1", call.Auth)
		assert.Equal(t, signed, call.Query.Get("callbackurl"))
		applis = append(applis, call.Query.Get("appli"))
	}
	sort.Strings(applis)
	assert.Equal(t, []string{"1", "16", "44", "46"}, applis)

	// The URL Withings will call back verifies.
	u, err := url.Parse(calls[0].Query.Get("callbackurl"))
	require.NoError(t, err)
	assert.Equal(t, "/inbound/withings", u.Path)
}

func TestSubscribe_APIError(t *testing.T) {
	f := newFakeWithings(t)
	f.NotifyStatus = 2554
	err := f.connector().Subscribe(context.Background(), subConn(), testCallback)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2554")
	assert.Len(t, f.calls("/notify", ""), 1, "stops at the first failure")
}

func TestSubscribe_NeedsTokenAndUser(t *testing.T) {
	f := newFakeWithings(t)
	assert.Error(t, f.connector().Subscribe(context.Background(), connector.Connection{Tokens: connector.Tokens{ProviderUserID: "1"}}, testCallback))
	assert.Error(t, f.connector().Subscribe(context.Background(), connector.Connection{Tokens: connector.Tokens{Access: "a"}}, testCallback))
	assert.Empty(t, f.calls("/notify", ""))
}

func TestUnsubscribe(t *testing.T) {
	f := newFakeWithings(t)
	c := f.connector()
	require.NoError(t, c.Unsubscribe(context.Background(), subConn(), testCallback))
	calls := f.calls("/notify", "revoke")
	require.Len(t, calls, 4)
	signed, _ := c.SignedCallbackURL(testCallback, "424242")
	for _, call := range calls {
		assert.Equal(t, signed, call.Query.Get("callbackurl"), "revoke names the URL subscribe registered")
	}
}
