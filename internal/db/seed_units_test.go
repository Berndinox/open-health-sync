// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"regexp"
	"testing"

	"github.com/soma-intelligence/open-health-sync/internal/model"
)

// seedRowRe reads one quantity_codes seed row: canonical code and storage unit.
var seedRowRe = regexp.MustCompile(`\(\s*'([a-z0-9_]+)',\s*(?:'[^']*'|NULL),\s*(?:'[^']*'|NULL),\s*(?:'[^']*'|NULL),\s*'([^']+)'`)

// TestSeedRegistersEveryStorageUnit keeps quantity_codes and the storage unit
// table of internal/model in step: every declared type is seeded, with the
// same unit.
func TestSeedRegistersEveryStorageUnit(t *testing.T) {
	files, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	seeded := map[string]string{}
	for _, f := range files {
		if !regexp.MustCompile(`\.up\.sql$`).MatchString(f.Name()) {
			continue
		}
		raw, err := migrationsFS.ReadFile("migrations/" + f.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range seedRowRe.FindAllStringSubmatch(string(raw), -1) {
			seeded[m[1]] = m[2] // a later migration wins, as ON CONFLICT does
		}
	}
	for qt, unit := range model.StorageUnits() {
		got, ok := seeded[qt]
		if !ok {
			t.Errorf("%s declares storage unit %q but no migration seeds it in quantity_codes", qt, unit)
			continue
		}
		if got != unit {
			t.Errorf("%s is seeded with storage unit %q, internal/model declares %q", qt, got, unit)
		}
	}
}
