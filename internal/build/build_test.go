package build

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
)

// example copies the shared example into a new directory.
func example(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../testdata/example")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MAAGO_DATABASE_URL", "sqlite:app.db")
	return root
}

// noEnv leaves MAAGO_DATABASE_URL unset for the test, so .env is read.
func noEnv(t *testing.T) {
	t.Setenv("MAAGO_DATABASE_URL", "") // restores it afterwards
	os.Unsetenv("MAAGO_DATABASE_URL")
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func text(ds []diag.Diagnostic) string {
	var b strings.Builder
	diag.Write(&b, ds)
	return b.String()
}

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := map[string]string{}
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			m[p] = read(t, p)
		}
		return err
	})
	return m
}

// M1 completion condition 1, through the whole pipeline.
func TestMaxlen(t *testing.T) {
	root := example(t)
	models := filepath.Join(root, "blog", "models.go")
	write(t, models, strings.Replace(read(t, models), "max=200", "maxlen=200", 1))
	before := snapshot(t, root)

	_, ds := Gen(root)

	out := text(ds)
	if !strings.Contains(out, `blog/models.go:15:32: unknown tag key "maxlen" in Post.Title`) || !strings.Contains(out, `did you mean "max"?`) || len(ds) != 1 {
		t.Errorf("got:\n%s", out)
	}
	if after := snapshot(t, root); !maps.Equal(before, after) {
		t.Errorf("gen wrote files despite an error")
	}
}

// Route errors come from one stage, so a taken name and a missing view
// show together, and nothing is written (D-60, D-61).
func TestRoutes(t *testing.T) {
	root := example(t)
	write(t, filepath.Join(root, "blog", "urls.go"), "package blog\n\nimport \"github.com/maago-web/maago\"\n\nvar Routes = maago.Routes{\n\tmaago.GET(\"/a\", \"x\", A),\n\tmaago.GET(\"/b\", \"x\", B),\n}\n\nfunc A() {}\n")
	write(t, filepath.Join(root, "main.go"), "package main\n\nimport \"example.com/app/blog\"\n\nvar _ = blog.App\n")
	before := snapshot(t, root)

	_, ds := Gen(root)

	want := `blog/urls.go:7:18: route name "x" is taken by blog/urls.go:6
  fix: route names are one namespace across the project; rename one
blog/urls.go:7:23: no function B in package blog
  fix: did you mean "A"?
`
	if out := text(ds); out != want {
		t.Errorf("got:\n%s", out)
	}
	if after := snapshot(t, root); !maps.Equal(before, after) {
		t.Errorf("gen wrote files despite an error")
	}
}

// A syntax error stops the run at parsing: the tag error in another
// package, which checking would report, does not appear.
func TestStagesStop(t *testing.T) {
	root := example(t)
	write(t, filepath.Join(root, "blog", "broken.go"), "package blog\n\nfunc (\n")
	write(t, filepath.Join(root, "shop", "models.go"), "package shop\n\nimport \"github.com/maago-web/maago\"\n\ntype Item struct {\n\tmaago.Model\n\tID int64\n\tName string `maago:\"uniqe\"`\n}\n")

	out := text(Check(root))
	if !strings.Contains(out, "blog/broken.go") || strings.Contains(out, "uniqe") {
		t.Errorf("got:\n%s", out)
	}
}

// Outside a module neither command may pass: there is nothing it checked.
func TestNoModule(t *testing.T) {
	if _, ds := Gen(t.TempDir()); !strings.Contains(text(ds), "go mod init") {
		t.Errorf("gen: got:\n%s", text(ds))
	}
	if ds := Check(t.TempDir()); !strings.Contains(text(ds), "go mod init") {
		t.Errorf("check: got:\n%s", text(ds))
	}
}

// Constraint names that clash are caught before anything is written, even
// though no dialect is chosen yet.
func TestNameClash(t *testing.T) {
	root := example(t)
	write(t, filepath.Join(root, "shop", "models.go"), "package shop\n\nimport \"github.com/maago-web/maago\"\n\n"+
		"type A struct {\n\tmaago.Model `maago:\"table=a,unique=(B,C)\"`\n\tID int64\n\tB  string\n\tC  string\n}\n\n"+
		"type AB struct {\n\tmaago.Model `maago:\"table=a_b\"`\n\tID int64\n\tC  string `maago:\"unique\"`\n}\n")
	before := snapshot(t, root)
	_, ds := Gen(root)
	if out := text(ds); !strings.Contains(out, `constraint name "a_b_c_key" of AB is also used by A`) {
		t.Errorf("got:\n%s", out)
	}
	if after := snapshot(t, root); !maps.Equal(before, after) {
		t.Errorf("gen wrote files despite an error")
	}
}

// This module has no database driver: check reaches the database stage
// and fails there, as it must when the practice database cannot be
// reached (D-64), with nothing written.
func TestCheckWithoutDriver(t *testing.T) {
	root := example(t)
	before := snapshot(t, root)
	ds := Check(root)
	if out := text(ds); len(ds) != 1 || !strings.HasPrefix(out, `no "sqlite" database driver in this maago build`) || !strings.Contains(out, "fix: ") {
		t.Errorf("got:\n%s", out)
	}
	if _, ds := Gen(root); len(ds) != 1 {
		t.Errorf("gen: got:\n%s", text(ds))
	}
	if after := snapshot(t, root); !maps.Equal(before, after) {
		t.Errorf("wrote files despite an error")
	}
}

// A PostgreSQL project's names are checked for its length limit before
// any connection.
func TestPostgresNameLength(t *testing.T) {
	root := example(t)
	t.Setenv("MAAGO_DATABASE_URL", "postgres://127.0.0.1:1/nowhere")
	write(t, filepath.Join(root, "shop", "models.go"), "package shop\n\nimport \"github.com/maago-web/maago\"\n\n"+
		"type Item struct {\n\tmaago.Model `maago:\"table="+strings.Repeat("x", 64)+"\"`\n\tID int64\n}\n")
	if ds := Check(root); len(ds) != 2 || strings.Count(text(ds), "is longer than 63 bytes") != 2 { // the table and its key
		t.Errorf("got:\n%s", text(ds))
	}
}

// A bad .env, a tag error and an unfinished migration are independent,
// so one run shows all three (D-61).
func TestEnvTagsAndMigrations(t *testing.T) {
	root := example(t)
	noEnv(t)
	write(t, filepath.Join(root, ".env"), "MAAGO_DATABASE_URL=mysql://x\n")
	models := filepath.Join(root, "blog", "models.go")
	write(t, models, strings.Replace(read(t, models), "max=200", "maxlen=200", 1))
	write(t, filepath.Join(root, "migrations", "0001_initial.sql"), "CREATE TABLE t (a TEXT DEFAULT 'x);\n")
	out := text(Check(root))
	for _, want := range []string{".env:1:", `unknown tag key "maxlen"`, "migrations/0001_initial.sql:1:"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in:\n%s", want, out)
		}
	}
}
