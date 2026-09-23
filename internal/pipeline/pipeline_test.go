package pipeline

import (
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maago-web/maago/internal/diag"
)

// snapshot returns every file under dir with its content.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		m[path] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fail(msg string) Step {
	return func(*Run) []diag.Diagnostic {
		return []diag.Diagnostic{diag.New(token.Position{Filename: "x.go", Line: 1}, msg, "fix "+msg)}
	}
}

func text(ds []diag.Diagnostic) string {
	var b strings.Builder
	diag.Write(&b, ds)
	return b.String()
}

func TestGenStopsAtFailingStage(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "keep.go"), "old")
	before := snapshot(t, root)

	ran := false
	ds := Gen(root, [][]Step{
		// A generator in the failing stage: its output must not be written.
		{fail("first"), func(r *Run) []diag.Diagnostic {
			r.Files["keep.go"] = []byte("overwritten")
			return nil
		}, fail("second")},
		{func(r *Run) []diag.Diagnostic {
			ran = true
			return nil
		}},
	})

	if out := text(ds); len(ds) != 2 || !strings.Contains(out, "first") || !strings.Contains(out, "second") {
		t.Errorf("want both errors of the failing stage, got:\n%s", out)
	}
	if ran {
		t.Error("the stage after a failing stage ran")
	}
	if after := snapshot(t, root); !maps.Equal(before, after) {
		t.Errorf("files changed:\nbefore %v\nafter  %v", before, after)
	}
}

func TestGenStopsAtSingleError(t *testing.T) {
	root := t.TempDir()
	ran := false
	ds := Gen(root, [][]Step{
		{fail("only"), func(r *Run) []diag.Diagnostic {
			r.Files["new.go"] = []byte("x")
			return nil
		}},
		{func(*Run) []diag.Diagnostic { ran = true; return nil }},
	})
	if len(ds) != 1 || ran {
		t.Errorf("want one error and no later stage, got %d errors, later stage ran: %v", len(ds), ran)
	}
	if after := snapshot(t, root); len(after) != 0 {
		t.Errorf("files written: %v", after)
	}
}

func TestGenWrites(t *testing.T) {
	root := t.TempDir()
	same, changed := filepath.Join(root, "same.go"), filepath.Join(root, "changed.go")
	write(t, same, "same")
	write(t, changed, "old")
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(same, past, past); err != nil {
		t.Fatal(err)
	}

	secondRan := false
	ds := Gen(root, [][]Step{
		{func(*Run) []diag.Diagnostic { return nil }},
		{func(r *Run) []diag.Diagnostic {
			secondRan = true
			r.Files["same.go"] = []byte("same")
			r.Files["changed.go"] = []byte("new")
			r.Files["sub/dir/new.go"] = []byte("fresh")
			return nil
		}},
	})

	if len(ds) > 0 {
		t.Fatalf("unexpected errors:\n%s", text(ds))
	}
	if !secondRan {
		t.Fatal("second stage did not run")
	}
	want := map[string]string{same: "same", changed: "new", filepath.Join(root, "sub", "dir", "new.go"): "fresh"}
	if got := snapshot(t, root); !maps.Equal(got, want) {
		t.Errorf("files:\ngot  %v\nwant %v", got, want)
	}
	if fi, err := os.Stat(filepath.Join(root, "sub", "dir", "new.go")); err != nil || fi.Mode().Perm() != 0o644 {
		t.Errorf("new file mode: %v, %v", fi.Mode(), err)
	}
	if fi, err := os.Stat(filepath.Join(root, "sub", "dir")); err != nil || fi.Mode().Perm() != 0o755 {
		t.Errorf("new directory mode: %v, %v", fi.Mode(), err)
	}
	if fi, err := os.Stat(same); err != nil || !fi.ModTime().Equal(past) {
		t.Errorf("unchanged file was rewritten: %v, %v", fi.ModTime(), err)
	}
}

func TestWriteRejectsPathOutsideRoot(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "keep.go"), "old")
	before := snapshot(t, root)

	ds := Write(root, map[string][]byte{"ok.go": []byte("x"), "../escape.go": []byte("x"), "new\nline.go": []byte("x")})

	if out := text(ds); len(ds) != 2 || !strings.Contains(out, "../escape.go") || !strings.Contains(out, "line.go") {
		t.Errorf("want errors naming both bad paths, got:\n%s", out)
	}
	if after := snapshot(t, root); !maps.Equal(before, after) {
		t.Errorf("files changed:\nbefore %v\nafter  %v", before, after)
	}
}

func TestWriteFailureChangesNothing(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a.go"), "old")
	write(t, filepath.Join(root, "b"), "a regular file, so b/c.go cannot be written")
	before := snapshot(t, root)

	// a.go sorts first, so its temp file exists when b/c.go fails.
	ds := Write(root, map[string][]byte{"a.go": []byte("new"), "b/c.go": []byte("x")})

	if !strings.Contains(text(ds), "b/c.go") {
		t.Errorf("want an error naming b/c.go, got:\n%s", text(ds))
	}
	if after := snapshot(t, root); !maps.Equal(before, after) {
		t.Errorf("files changed or temp files left:\nbefore %v\nafter  %v", before, after)
	}
}
