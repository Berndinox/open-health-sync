// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/soma-intelligence/open-health-sync/internal/db"
)

func TestDoctorAllOK(t *testing.T) {
	out, err := runDoctorT(t, map[string]string{
		"OHS_DATABASE_URL":   db.TestDB(t),
		"OHS_ENCRYPTION_KEY": goodKey(),
		"OHS_RAW_STORE":      "file://" + t.TempDir(),
	}, false, nil)
	require.NoError(t, err, out)
	require.Contains(t, out, "ok    database")
	require.Contains(t, out, "ok    raw store")
}

// pool_max_conns in the URL (as the compose file sets it) is for the pool,
// not the server: the doctor's database check must still pass.
func TestDoctorDatabaseURLWithPoolParams(t *testing.T) {
	out, err := runDoctorT(t, map[string]string{
		"OHS_DATABASE_URL":   db.TestDB(t) + "&pool_max_conns=20",
		"OHS_ENCRYPTION_KEY": goodKey(),
		"OHS_RAW_STORE":      "file://" + t.TempDir(),
	}, true, nil)
	require.NoError(t, err, out)
	require.Contains(t, out, "ok    database")
}
