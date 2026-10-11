// SPDX-License-Identifier: AGPL-3.0-only

package db

import "testing"

func TestConnURL(t *testing.T) {
	for in, want := range map[string]string{
		"postgres://ohs:pw@db:5432/ohs?sslmode=disable&pool_max_conns=20": "postgres://ohs:pw@db:5432/ohs?sslmode=disable",
		"postgres://ohs@db/ohs?pool_max_conns=20&pool_min_conns=2":        "postgres://ohs@db/ohs",
		"postgres://ohs@db/ohs?sslmode=disable":                           "postgres://ohs@db/ohs?sslmode=disable",
		"postgres://ohs@db/ohs":                                           "postgres://ohs@db/ohs",
		"host=db user=ohs pool_max_conns=20":                              "host=db user=ohs pool_max_conns=20",
	} {
		if got := ConnURL(in); got != want {
			t.Errorf("ConnURL(%q) = %q, want %q", in, got, want)
		}
	}
}
