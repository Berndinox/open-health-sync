// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

package main

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/soma-intelligence/open-health-sync/internal/auth"
	"github.com/soma-intelligence/open-health-sync/internal/db"
	"github.com/soma-intelligence/open-health-sync/internal/tenant"
)

func TestAdminTenantCreate(t *testing.T) {
	url := db.TestDB(t)
	t.Setenv("OHS_DATABASE_URL", url)
	t.Setenv("OHS_ENCRYPTION_KEY", "")

	var buf bytes.Buffer
	if err := run([]string{"admin", "tenant", "create", "--name", "Acme"}, &buf); err != nil {
		t.Fatalf("admin tenant create: %v", err)
	}
	key := regexp.MustCompile(`ohs_sk_[0-9A-Za-z]{43}`).FindString(buf.String())
	if key == "" || strings.Count(buf.String(), "ohs_sk_") != 1 {
		t.Fatalf("the key is printed exactly once: %q", buf.String())
	}
	pool, err := db.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := tenant.New(pool).LookupKey(context.Background(), auth.Hash(key)); err != nil {
		t.Fatalf("printed key does not authenticate: %v", err)
	}
}
