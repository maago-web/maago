package build

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
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
	return root
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

func TestGenThenCheck(t *testing.T) {
	root := example(t)
	gen := filepath.Join(root, "blog", "models_gen.go")

	if removed, ds := Gen(filepath.Join(root, "blog")); len(ds) > 0 || len(removed) > 0 {
		t.Fatalf("gen: removed %v\n%s", removed, text(ds))
	}
	if got, want := read(t, gen), read(t, "../model/testdata/models_gen.golden"); got != want {
		t.Errorf("models_gen.go:\n%s\nwant:\n%s", got, want)
	}
	if ds := Check(root); len(ds) > 0 {
		t.Fatalf("check right after gen:\n%s", text(ds))
	}

	write(t, gen, read(t, gen)+"// edited by hand\n")
	if out := text(Check(root)); !strings.Contains(out, "blog/models_gen.go: generated file is out of date") {
		t.Errorf("check after an edit:\n%s", out)
	}
	os.Remove(gen)
	if out := text(Check(root)); !strings.Contains(out, "blog/models_gen.go: generated file is missing") {
		t.Errorf("check after removing it:\n%s", out)
	}

	// Once every field is a column, gen removes models_gen.go.
	Gen(root)
	models := filepath.Join(root, "blog", "models.go")
	var kept []string
	for _, line := range strings.SplitAfter(read(t, models), "\n") {
		if f := strings.Fields(line); len(f) > 0 && slices.Contains([]string{"Kind", "Count", "cache", "Digest", "Audit"}, f[0]) {
			continue
		}
		kept = append(kept, line)
	}
	write(t, models, strings.Join(kept, ""))
	removed, ds := Gen(root)
	if len(ds) > 0 || !slices.Equal(removed, []string{"blog/models_gen.go"}) {
		t.Errorf("removed %v\n%s", removed, text(ds))
	}
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

// Each run starts clean: two projects in a row do not mix.
func TestRunsAreIndependent(t *testing.T) {
	a, b := example(t), t.TempDir()
	write(t, filepath.Join(b, "go.mod"), "module example.com/b\n")
	if _, ds := Gen(a); len(ds) > 0 {
		t.Fatal(text(ds))
	}
	if removed, ds := Gen(b); len(ds) > 0 || len(removed) > 0 {
		t.Errorf("second project: removed %v\n%s", removed, text(ds))
	}
	if _, err := os.Stat(filepath.Join(b, "blog")); err == nil {
		t.Error("the first project's files leaked into the second")
	}
	if ds := Check(a); len(ds) > 0 {
		t.Errorf("first project after the second:\n%s", text(ds))
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
