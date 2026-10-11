// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/soma-intelligence/open-health-sync/internal/db"
)

func TestMigrateCLIDownAll(t *testing.T) {
	url := db.TestDB(t)
	t.Setenv("OHS_DATABASE_URL", url)
	t.Setenv("OHS_ENCRYPTION_KEY", "")

	var buf bytes.Buffer
	if err := run([]string{"migrate", "down"}, &buf); err == nil {
		t.Fatal("down without --all must refuse")
	}
	if v, _, err := db.Version(url); err != nil || v == 0 {
		t.Fatalf("refused down still changed the schema: version %d err %v", v, err)
	}
	if err := run([]string{"migrate", "down", "--all"}, &buf); err != nil {
		t.Fatalf("down --all: %v", err)
	}
	buf.Reset()
	if err := run([]string{"migrate", "version"}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "schema version 0") {
		t.Fatalf("after down --all: %q", buf.String())
	}
}
