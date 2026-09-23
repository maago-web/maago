package maago

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestNoReflect enforces D-66: no non-test Go file in the repository, and no
// generator template, may import reflect. It walks the file tree rather than
// the package graph so that the nested cmd/maago module and testdata are
// covered too. go test runs in the package directory, which is the repo root.
func TestNoReflect(t *testing.T) {
	const fix = "fix: remove the import; runtime and generated code must not use reflect (D-66)"
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			t.Errorf("%s: %v (D-66 gate)", path, err)
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		switch {
		case strings.HasSuffix(path, "_test.go"):
		case strings.HasSuffix(path, ".go"):
			f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				t.Errorf("%v (D-66 gate cannot read imports)\n  fix: make the file valid Go, or give an intentionally broken fixture a non-.go extension", err)
				return nil
			}
			for _, imp := range f.Imports {
				if p, _ := strconv.Unquote(imp.Path.Value); p == "reflect" {
					t.Errorf("%s: imports \"reflect\" (D-66)\n  %s", fset.Position(imp.Path.Pos()), fix)
				}
			}
		case strings.HasSuffix(path, ".tmpl"):
			b, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("%s: %v (D-66 gate)", path, err)
				return nil
			}
			for i, line := range strings.Split(string(b), "\n") {
				if col := strings.Index(line, `"reflect"`); col >= 0 {
					t.Errorf("%s:%d:%d: template mentions \"reflect\" (D-66)\n  %s", path, i+1, col+1, fix)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
