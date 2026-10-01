package maago

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/mapper"
	"github.com/maago-web/maago/internal/pipeline"
	"github.com/maago-web/maago/internal/web"
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

// TestGeneratedImports enforces M3's completion condition that generated
// code uses neither text/template nor reflect (D-66), more narrowly than
// TestNoReflect: the generator uses text/template, so only what maago
// generates is held to it. A generated file may import only what
// mapper.Imports lists, which also keeps Names' clash check honest. The
// gentest goldens under cmd/maago are generated files, so this looks at
// real output.
func TestGeneratedImports(t *testing.T) {
	for _, p := range generatedImports(".") {
		t.Error(p)
	}
}

// The gate fails on what it should, and only on that.
func TestGeneratedImportsGate(t *testing.T) {
	root := filepath.Clean(t.TempDir()) // WalkDir's paths are clean
	files := map[string]string{
		"a/template_gen.go": pipeline.Marker + "\n\npackage a\n\nimport _ \"text/template\"\n",
		"a/reflect_gen.go":  pipeline.Marker + "\n\npackage a\n\nimport _ \"reflect\"\n",
		"a/crlf_gen.go":     pipeline.Marker + "\r\n\r\npackage a\r\n\r\nimport _ \"text/template\"\r\n",
		"a/broken_gen.go":   pipeline.Marker + "\n\npackage a\n\nimport (\n",
		"a/gen.go.tmpl":     "import \"text/template\"\n",
		"a/generator.go":    "package a\n\nimport _ \"text/template\"\n",
		"a/os_gen.go":       pipeline.Marker + "\n\npackage a\n\nimport _ \"os\"\n", // allowed nowhere, not only forbidden
		"a/web_gen.go":      pipeline.Marker + "\n\npackage a\n\nimport (\n\t_ \"net/http\"\n\t_ \"strconv\"\n)\n",
		"a/ok_gen.go":       pipeline.Marker + "\n\npackage a\n\nimport (\n\t_ \"context\"\n\t_ \"github.com/maago-web/maago\"\n)\n",
	}
	for p, src := range files {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for _, p := range generatedImports(root) {
		got = append(got, strings.SplitN(strings.TrimPrefix(p, root+string(filepath.Separator)), ":", 2)[0])
	}
	want := []string{"a/broken_gen.go", "a/crlf_gen.go", "a/gen.go.tmpl", "a/os_gen.go", "a/reflect_gen.go", "a/template_gen.go"}
	if slices.Sort(got); !slices.Equal(got, want) {
		t.Errorf("caught %q, want %q", got, want)
	}
}

// generatedImports walks root and returns a line for each generated Go
// file that imports what the generators may not, or cannot be read, and
// for each generator template that mentions text/template or reflect.
func generatedImports(root string) []string {
	const fix = "fix: generated code may import only mapper.Imports and web.Imports; never text/template or reflect (D-66)"
	var problems []string
	fset := token.NewFileSet()
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			problems = append(problems, fmt.Sprintf("%s: %v", path, err))
		case d.IsDir() && d.Name() == ".git":
			return filepath.SkipDir
		case strings.HasSuffix(path, ".go"):
			src, err := os.ReadFile(path)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", path, err))
				return nil
			}
			if !pipeline.IsGenerated(src) {
				return nil
			}
			f, err := parser.ParseFile(fset, path, src, parser.ImportsOnly)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%v\n  fix: generated code must be valid Go", err))
				return nil
			}
			for _, imp := range f.Imports {
				if p, _ := strconv.Unquote(imp.Path.Value); !slices.Contains(mapper.Imports, p) && !slices.Contains(web.Imports, p) {
					problems = append(problems, fmt.Sprintf("%s: generated code imports %q\n  %s", fset.Position(imp.Path.Pos()), p, fix))
				}
			}
		case strings.HasSuffix(path, ".tmpl"):
			b, err := os.ReadFile(path)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", path, err))
				return nil
			}
			for _, pkg := range []string{`"text/template"`, `"reflect"`} {
				if strings.Contains(string(b), pkg) {
					problems = append(problems, fmt.Sprintf("%s: template mentions %s\n  %s", path, pkg, fix))
				}
			}
		}
		return nil
	})
	return problems
}
