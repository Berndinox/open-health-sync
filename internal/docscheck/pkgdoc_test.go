// SPDX-License-Identifier: AGPL-3.0-only

package docscheck

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// hasPrefix reports whether a doc comment starts the way Go expects: "Package
// name" for a library, "Command name" for a main package (whose name is the
// directory, so only the word is checked).
func hasPrefix(doc, pkg string) bool {
	if pkg == "main" {
		return strings.HasPrefix(doc, "Command ")
	}
	return strings.HasPrefix(doc, "Package "+pkg+" ")
}

// TestEveryPackageHasADocComment lists the packages without a package comment.
// A package is a directory with at least one non-test Go file; directories
// with none are skipped. Build tags are ignored, so files behind a tag count. Add the comment to any one file of
// the package (by convention doc.go, or the file named after the package).
func TestEveryPackageHasADocComment(t *testing.T) {
	root := filepath.Join("..", "..")
	documented := map[string]bool{}
	seen := map[string]bool{}
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		dir := filepath.Dir(path)
		seen[dir] = true
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.PackageClauseOnly)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			return nil
		}
		if f.Doc != nil && hasPrefix(f.Doc.Text(), f.Name.Name) {
			documented[dir] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 {
		t.Fatal("found no Go package: the walk root is wrong")
	}

	var missing []string
	for dir := range seen {
		if !documented[dir] {
			rel, _ := filepath.Rel(root, dir)
			missing = append(missing, filepath.ToSlash(rel))
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("packages without a package comment starting \"Package <name>\" (\"Command <name>\" for main):\n  %s", strings.Join(missing, "\n  "))
	}
}
