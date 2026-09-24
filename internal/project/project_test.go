package project

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func text(ds []diag.Diagnostic) string {
	var b strings.Builder
	diag.Write(&b, ds)
	return b.String()
}

// tempDir returns a temporary directory with symbolic links resolved
// (on macOS, /var is a link to /private/var).
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRoot(t *testing.T) {
	root := tempDir(t)
	write(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	sub := filepath.Join(root, "blog", "templates")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tempDir(t), "link")
	if err := os.Symlink(filepath.Join(root, "blog"), link); err != nil {
		t.Fatal(err)
	}

	t.Chdir(sub)
	for _, dir := range []string{root, sub, link, "."} {
		got, ds := Root(dir)
		if got != root || len(ds) > 0 {
			t.Errorf("Root(%s) = %q, want %q\n%s", dir, got, root, text(ds))
		}
	}
}

func TestRootWithoutModule(t *testing.T) {
	dir := tempDir(t)
	_, ds := Root(dir)
	if out := text(ds); len(ds) != 1 || !strings.Contains(out, dir) || !strings.Contains(out, "go mod init") {
		t.Errorf("want one error naming %s with a go mod init fix, got:\n%s", dir, out)
	}
}

func TestGoFiles(t *testing.T) {
	outside := tempDir(t)
	write(t, filepath.Join(outside, "linked.go"), "package x")
	write(t, filepath.Join(outside, "dir", "in_linked_dir.go"), "package x")

	root := t.TempDir() // not resolved: GoFiles must resolve it
	for _, p := range []string{
		"go.mod", "main.go", "blog.go", "blog/models.go", "blog/models_gen.go",
		"blog/models_test.go", "blog/notes.txt",
		"testdata/a.go", "vendor/b.go", "_hidden/c.go", ".dot/d.go", "_e.go", ".f.go",
		"submodule/go.mod", "submodule/g.go",
	} {
		write(t, filepath.Join(root, filepath.FromSlash(p)), "package x")
	}
	if err := os.Symlink(filepath.Join(outside, "linked.go"), filepath.Join(root, "link.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "dir"), filepath.Join(root, "linkdir")); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "project")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}

	want := []string{"blog.go", "blog/models.go", "blog/models_gen.go", "blog/models_test.go", "main.go"}
	for _, r := range []string{root, link} {
		got, ds := GoFiles(r)
		if len(ds) > 0 || !slices.Equal(got, want) {
			t.Errorf("GoFiles(%s) = %q, want %q\n%s", r, got, want, text(ds))
		}
	}
}

func TestMissingDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, ds := Root(missing); len(ds) != 1 {
		t.Errorf("Root: want one error, got:\n%s", text(ds))
	}
	if _, ds := GoFiles(missing); len(ds) != 1 {
		t.Errorf("GoFiles: want one error, got:\n%s", text(ds))
	}
}

// An unreadable directory is reported with a one-line message, and the
// walk goes on.
func TestGoFilesUnreadableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read any directory")
	}
	root := t.TempDir()
	write(t, filepath.Join(root, "z.go"), "package x")
	locked := filepath.Join(root, "a\nlocked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o755)

	files, ds := GoFiles(root)

	if out := text(ds); len(ds) != 1 || strings.Count(out, "\n") != 2 || !strings.Contains(out, `a\nlocked`) {
		t.Errorf("want one error naming the directory, got:\n%s", out)
	}
	if !slices.Equal(files, []string{"z.go"}) {
		t.Errorf("files = %q, want [z.go]", files)
	}
}
