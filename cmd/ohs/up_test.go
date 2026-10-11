// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"testing"
)

func TestUpStopsWhenMigrateFails(t *testing.T) {
	boom := errors.New("boom")
	served := false
	err := up(nil, func() error { return boom }, func([]string) error { served = true; return nil })
	if !errors.Is(err, boom) || served {
		t.Fatalf("err=%v served=%v, want migrate error and no serve", err, served)
	}
}

func TestUpServesAfterMigrate(t *testing.T) {
	var got []string
	err := up([]string{"--x"}, func() error { return nil }, func(a []string) error { got = a; return nil })
	if err != nil || len(got) != 1 || got[0] != "--x" {
		t.Fatalf("err=%v args=%v", err, got)
	}
}
