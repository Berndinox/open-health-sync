// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestCommittedDocIsCurrent fails when docs/data-model.md is not what the
// generator writes. Fix it with: go run ./internal/model/cmd/gendocs
func TestCommittedDocIsCurrent(t *testing.T) {
	want, err := Generate("../..")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../../../docs/data-model.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("docs/data-model.md is out of date: run `go run ./internal/model/cmd/gendocs`")
	}
}

func TestGenerateCoversTypesAndUnits(t *testing.T) {
	doc, err := Generate("../..")
	if err != nil {
		t.Fatal(err)
	}
	s := string(doc)
	for _, want := range []string{
		"### QuantitySample", "### HeartbeatSample", "### MapResult", "### Observation",
		"| `heart_rate` | `bpm` |", "| `body_mass` | `kg` |",
		"Stored in `ohs_data.sleep_phases`.",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("generated document lacks %q", want)
		}
	}
	if strings.ContainsRune(s, rune(0x2014)) {
		t.Error("generated document holds an em dash")
	}
}

// TestNotesAndTablesMatchRealTypes fails when a note or table entry names a
// type or field that no longer exists, so the generator cannot rot silently.
func TestNotesAndTablesMatchRealTypes(t *testing.T) {
	types, err := structTypes(token.NewFileSet(), "../..")
	if err != nil {
		t.Fatal(err)
	}
	typeNames := map[string]bool{}
	fields := map[string]bool{} // Type.Field
	anyField := map[string]bool{}
	for _, st := range types {
		typeNames[st.name] = true
		for _, f := range st.st.Fields.List {
			for _, n := range f.Names {
				fields[st.name+"."+n.Name] = true
				anyField[n.Name] = true
			}
		}
	}
	for key := range notes {
		if strings.Contains(key, ".") {
			if !fields[key] {
				t.Errorf("note %q names no real Type.Field", key)
			}
		} else if !anyField[key] {
			t.Errorf("note %q names no real field", key)
		}
	}
	for name := range tables {
		if !typeNames[name] {
			t.Errorf("table entry %q names no real type", name)
		}
	}
	for _, st := range types {
		if _, ok := tables[st.name]; !ok && isRecord(st) && st.name != "MapResult" {
			t.Errorf("record type %s has no table entry in tables", st.name)
		}
	}
}
