package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/build"
)

// exampleProject copies the shared example into a new directory with a
// SQLite database and its first migration, the models' DDL (M1's golden).
func exampleProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../internal/testdata/example")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MAAGO_DATABASE_URL", "sqlite:app.db")
	writeFile(t, filepath.Join(root, "migrations", "0001_initial.sql"), readFile(t, "../../internal/dialect/sqlite/testdata/example.sql"))
	return root
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeFile(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// snapshot is every file under dir and its content.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := map[string]string{}
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			m[p] = readFile(t, p)
		}
		return err
	})
	return m
}

func TestGenThenCheck(t *testing.T) {
	root := exampleProject(t)
	gen := filepath.Join(root, "blog", "models_gen.go")

	if removed, ds := build.Gen(filepath.Join(root, "blog")); len(ds) > 0 || len(removed) > 0 {
		t.Fatalf("gen: removed %v\n%s", removed, text(ds))
	}
	if got, want := readFile(t, gen), readFile(t, "../../internal/model/testdata/models_gen.golden"); got != want {
		t.Errorf("models_gen.go:\n%s\nwant:\n%s", got, want)
	}
	if ds := build.Check(root); len(ds) > 0 {
		t.Fatalf("check right after gen:\n%s", text(ds))
	}

	writeFile(t, gen, readFile(t, gen)+"// edited by hand\n")
	if out := text(build.Check(root)); !strings.Contains(out, "blog/models_gen.go: generated file is out of date") {
		t.Errorf("check after an edit:\n%s", out)
	}
	os.Remove(gen)
	if out := text(build.Check(root)); !strings.Contains(out, "blog/models_gen.go: generated file is missing") {
		t.Errorf("check after removing it:\n%s", out)
	}

	// Once every field is a column, gen removes models_gen.go.
	build.Gen(root)
	models := filepath.Join(root, "blog", "models.go")
	var kept []string
	for _, line := range strings.SplitAfter(readFile(t, models), "\n") {
		if f := strings.Fields(line); len(f) > 0 && slices.Contains([]string{"Kind", "Count", "cache", "Digest", "Audit"}, f[0]) {
			continue
		}
		kept = append(kept, line)
	}
	writeFile(t, models, strings.Join(kept, ""))
	removed, ds := build.Gen(root)
	if len(ds) > 0 || !slices.Equal(removed, []string{"blog/models_gen.go"}) {
		t.Errorf("removed %v\n%s", removed, text(ds))
	}
}

// Each run starts clean: two projects in a row do not mix.
func TestRunsAreIndependent(t *testing.T) {
	a, b := exampleProject(t), t.TempDir()
	writeFile(t, filepath.Join(b, "go.mod"), "module example.com/b\n")
	if _, ds := build.Gen(a); len(ds) > 0 {
		t.Fatal(text(ds))
	}
	if removed, ds := build.Gen(b); len(ds) > 0 || len(removed) > 0 {
		t.Errorf("second project: removed %v\n%s", removed, text(ds))
	}
	if _, err := os.Stat(filepath.Join(b, "blog")); err == nil {
		t.Error("the first project's files leaked into the second")
	}
	if ds := build.Check(a); len(ds) > 0 {
		t.Errorf("first project after the second:\n%s", text(ds))
	}
}

// noEnv leaves MAAGO_DATABASE_URL unset for the test, so .env is read.
func noEnv(t *testing.T) {
	t.Setenv("MAAGO_DATABASE_URL", "") // restores it afterwards
	os.Unsetenv("MAAGO_DATABASE_URL")
}
