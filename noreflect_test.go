package maago_test

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

	"github.com/maago-web/maago/internal/admingen"
	"github.com/maago-web/maago/internal/mapper"
	"github.com/maago-web/maago/internal/pipeline"
	"github.com/maago-web/maago/internal/web"
)

// TestNoReflect enforces D-66: no non-test Go file in the repository, and no
// generator template, may import reflect. It walks the file tree rather than
// the package graph so that files behind build constraints and testdata are
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
		"a/admin_gen.go":    pipeline.Marker + "\n\npackage a\n\nimport _ \"github.com/maago-web/maago/contrib/admin\"\n",
		"a/user_gen.go":     pipeline.Marker + "\n\npackage a\n\nimport maagouser \"board\"\n",
		"a/userr_gen.go":    pipeline.Marker + "\n\npackage a\n\nimport maagouser \"reflect\"\n",
		"a/other_gen.go":    pipeline.Marker + "\n\npackage a\n\nimport other \"board\"\n",
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
	want := []string{"a/broken_gen.go", "a/crlf_gen.go", "a/gen.go.tmpl", "a/os_gen.go", "a/other_gen.go", "a/reflect_gen.go", "a/template_gen.go", "a/userr_gen.go"}
	if slices.Sort(got); !slices.Equal(got, want) {
		t.Errorf("caught %q, want %q", got, want)
	}
}

// generatedImports walks root and returns a line for each generated Go
// file that imports what the generators may not, or cannot be read, and
// for each generator template that mentions text/template or reflect.
func generatedImports(root string) []string {
	const fix = "fix: generated code may import only mapper.Imports, web.Imports and admingen.Imports; never text/template or reflect (D-66)"
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
				p, _ := strconv.Unquote(imp.Path.Value)
				if imp.Name != nil && imp.Name.Name == "maagouser" && !slices.Contains([]string{"reflect", "text/template", "unsafe"}, p) {
					continue // the package of the user type, a project's (D-149)
				}
				if !slices.Contains(mapper.Imports, p) && !slices.Contains(web.Imports, p) && !slices.Contains(admingen.Imports, p) {
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

// TestRuntimeImports enforces D-09 now that one module holds the CLI and
// its drivers too (D-151): outside cmd/maago, a Go file may import only the
// standard library and this module. It reads files, not go list's package
// graph, so a file behind a build constraint is held to it too.
func TestRuntimeImports(t *testing.T) {
	for _, p := range runtimeImports(".") {
		t.Error(p)
	}
}

// The gate fails on what it should, and only on that.
func TestRuntimeImportsGate(t *testing.T) {
	root := filepath.Clean(t.TempDir()) // WalkDir's paths are clean
	files := map[string]string{
		"driver.go":             "package a\n\nimport _ \"modernc.org/sqlite\"\n",
		"linux.go":              "//go:build linux\n\npackage a\n\nimport _ \"github.com/jackc/pgx/v5/stdlib\"\n",
		"cgo.go":                "package a\n\nimport \"C\"\n",
		"lookalike.go":          "package a\n\nimport _ \"github.com/maago-web/maagox\"\n",
		"broken.go":             "package a\n\nimport (\n",
		"ok.go":                 "package a\n\nimport (\n\t_ \"net/http\"\n\t_ \"github.com/maago-web/maago\"\n\t_ \"github.com/maago-web/maago/internal/diag\"\n)\n",
		"ok_test.go":            "package a\n\nimport _ \"modernc.org/sqlite\"\n",
		"cmd/maago/main.go":     "package main\n\nimport _ \"modernc.org/sqlite\"\n",
		"x/testdata/fixture.go": "package fixture\n\nimport _ \"modernc.org/sqlite\"\n",
		"cmd/maagox/other.go":   "package main\n\nimport _ \"modernc.org/sqlite\"\n",
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
	for _, p := range runtimeImports(root) {
		got = append(got, strings.SplitN(strings.TrimPrefix(p, root+string(filepath.Separator)), ":", 2)[0])
	}
	want := []string{"broken.go", "cgo.go", "cmd/maagox/other.go", "driver.go", "linux.go", "lookalike.go"}
	if slices.Sort(got); !slices.Equal(got, want) {
		t.Errorf("caught %q, want %q", got, want)
	}
}

// runtimeImports walks root and returns a line for each non-test Go file
// outside cmd/maago and testdata that imports what is neither the standard
// library nor this module, or cannot be read.
func runtimeImports(root string) []string {
	const module = "github.com/maago-web/maago"
	const fix = "fix: only cmd/maago may import outside the standard library; the runtime and the generators use the standard library only (D-09)"
	var problems []string
	fset := token.NewFileSet()
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			problems = append(problems, fmt.Sprintf("%s: %v", path, err))
		case d.IsDir() && (d.Name() == ".git" || d.Name() == "testdata" || path == filepath.Join(root, "cmd", "maago")):
			return filepath.SkipDir
		case strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go"):
			f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%v\n  fix: make the file valid Go", err))
				return nil
			}
			for _, imp := range f.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				std := p != "C" && !strings.Contains(strings.Split(p, "/")[0], ".")
				if !std && p != module && !strings.HasPrefix(p, module+"/") {
					problems = append(problems, fmt.Sprintf("%s: imports %q\n  %s", fset.Position(imp.Path.Pos()), p, fix))
				}
			}
		}
		return nil
	})
	return problems
}
