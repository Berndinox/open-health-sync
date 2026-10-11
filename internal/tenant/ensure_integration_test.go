// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

package tenant_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/soma-intelligence/open-health-sync/internal/auth"
)

func TestEnsureTenant(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	ten, key, err := s.EnsureTenant(ctx, "dev")
	require.NoError(t, err)
	require.Regexp(t, `^ohs_sk_`, key, "created now: the key comes back once")
	p, err := s.LookupKey(ctx, auth.Hash(key))
	require.NoError(t, err)
	assert.Equal(t, ten.ID, p.TenantID)

	again, key2, err := s.EnsureTenant(ctx, "dev")
	require.NoError(t, err)
	assert.Equal(t, ten.ID, again.ID)
	assert.Empty(t, key2, "an existing tenant never yields a key")

	// Concurrent first starts create one tenant.
	var wg sync.WaitGroup
	keys := make([]string, 8)
	for i := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, k, err := s.EnsureTenant(ctx, "race")
			assert.NoError(t, err)
			keys[i] = k
		}()
	}
	wg.Wait()
	created := 0
	for _, k := range keys {
		if k != "" {
			created++
		}
	}
	assert.Equal(t, 1, created)
	var n int
	require.NoError(t, s.Pool().QueryRow(ctx, `SELECT count(*) FROM ohs_core.tenants WHERE name = 'race'`).Scan(&n))
	assert.Equal(t, 1, n)
}
