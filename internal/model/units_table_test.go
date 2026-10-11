// SPDX-License-Identifier: AGPL-3.0-only

package model

import (
	"errors"
	"testing"
	"time"
)

// The storage unit table is the contract between the mappers, the
// quantity_codes seed and the alignment guard. StorageUnits hands out a copy,
// so a caller mutating it must not silently change what every future sample is
// converted into.

func TestStorageUnit(t *testing.T) {
	all := StorageUnits()
	if len(all) == 0 {
		t.Fatal("the storage unit table is empty")
	}
	for qt, want := range all {
		got, ok := StorageUnit(qt)
		if !ok {
			t.Errorf("%q is in the table but StorageUnit reports it undeclared", qt)
			continue
		}
		if got != want {
			t.Errorf("%q -> %q, want %q", qt, got, want)
		}
		if want == "" {
			t.Errorf("%q declares an empty storage unit", qt)
		}
	}
	// An undeclared type passes through rather than being forced into a unit.
	if _, ok := StorageUnit("something_nobody_shipped"); ok {
		t.Error("an unknown quantity type must report undeclared")
	}
	if _, ok := StorageUnit(""); ok {
		t.Error("an empty quantity type must report undeclared")
	}
}

func TestStorageUnitsReturnsACopy(t *testing.T) {
	first := StorageUnits()
	var key string
	for k := range first {
		key = k
		break
	}
	first[key] = "bogus"
	delete(first, key)

	second := StorageUnits()
	if _, ok := second[key]; !ok {
		t.Fatalf("mutating the returned map corrupted the table: %q disappeared", key)
	}
	if got, _ := StorageUnit(key); got == "bogus" {
		t.Fatalf("mutating the returned map rewrote the declared unit for %q", key)
	}
}

func TestUnitErrorUnwrapsToItsCause(t *testing.T) {
	cause := errors.New("no conversion from \"furlongs\" to storage unit \"m\"")
	e := UnitError{QuantityType: "distance_walking_running", Unit: "furlongs", Value: 3, Err: cause}
	if e.Error() != cause.Error() {
		t.Errorf("Error() = %q, want the cause message", e.Error())
	}
}

func TestParseNormalizedDates(t *testing.T) {
	start := "2026-06-10T08:00:00Z"
	end := "2026-06-10T09:00:00Z"

	gotStart, gotEnd, err := ParseNormalizedDates(map[string]any{"start_date": start, "end_date": end})
	if err != nil {
		t.Fatalf("ParseNormalizedDates: %v", err)
	}
	if !gotStart.Equal(time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("start = %v", gotStart)
	}
	if !gotEnd.Equal(time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("end = %v", gotEnd)
	}
	// Both results are normalised to UTC whatever offset the collector wrote.
	offset, _, err := ParseNormalizedDates(map[string]any{
		"start_date": "2026-06-10T10:00:00+02:00",
		"end_date":   "2026-06-10T11:00:00+02:00",
	})
	if err != nil {
		t.Fatalf("ParseNormalizedDates: %v", err)
	}
	if offset.Location() != time.UTC {
		t.Errorf("location = %v, want UTC", offset.Location())
	}
	if !offset.Equal(time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("start = %v, want the UTC instant", offset)
	}
}

func TestParseNormalizedDatesAcceptsNanoAndPlain(t *testing.T) {
	// The collector writes RFC3339Nano; older files carry plain RFC3339. Both
	// must parse, otherwise a replay of an old raw file fails.
	for _, pair := range [][2]string{
		{"2026-06-10T08:00:00.123456789Z", "2026-06-10T09:00:00.987Z"},
		{"2026-06-10T08:00:00Z", "2026-06-10T09:00:00Z"},
	} {
		if _, _, err := ParseNormalizedDates(map[string]any{
			"start_date": pair[0], "end_date": pair[1],
		}); err != nil {
			t.Errorf("%v: %v", pair, err)
		}
	}
}

func TestParseNormalizedDatesRejectsUnusableRecords(t *testing.T) {
	cases := map[string]map[string]any{
		"no start":           {"end_date": "2026-06-10T09:00:00Z"},
		"no end":             {"start_date": "2026-06-10T08:00:00Z"},
		"start not a string": {"start_date": 12345, "end_date": "2026-06-10T09:00:00Z"},
		"end not a string":   {"start_date": "2026-06-10T08:00:00Z", "end_date": 12345},
		"start unparseable":  {"start_date": "10/06/2026", "end_date": "2026-06-10T09:00:00Z"},
		"end unparseable":    {"start_date": "2026-06-10T08:00:00Z", "end_date": "later"},
		"empty record":       {},
	}
	for name, rec := range cases {
		if _, _, err := ParseNormalizedDates(rec); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
