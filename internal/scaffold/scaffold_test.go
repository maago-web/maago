package scaffold

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
)

func newProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func text(ds []diag.Diagnostic) string {
	var b strings.Builder
	diag.Write(&b, ds)
	return b.String()
}

func TestAdd(t *testing.T) {
	root := newProject(t)
	sub := filepath.Join(root, "cmd")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	written, ds := Add(sub, "auth") // from below the root, as gen finds it
	if ds != nil || !slices.Equal(written, []string{"auth/models.go"}) {
		t.Fatalf("%v\n%s", written, text(ds))
	}
	got, err := os.ReadFile(filepath.Join(root, "auth", "models.go"))
	want, _ := files.ReadFile("testdata/auth/models.go")
	if err != nil || string(got) != string(want) {
		t.Errorf("auth/models.go is not the scaffold: %v", err)
	}
	if strings.HasPrefix(string(got), "// Code generated") {
		t.Error("the copy is the project's, not generated (D-67)")
	}
}

// Anything at a path Add would write stops it before it writes anything.
func TestAddRefuses(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup func(root string) error
	}{
		{"a file", func(root string) error {
			os.Mkdir(filepath.Join(root, "auth"), 0o755)
			return os.WriteFile(filepath.Join(root, "auth", "models.go"), []byte("mine"), 0o644)
		}},
		{"a directory", func(root string) error { return os.MkdirAll(filepath.Join(root, "auth", "models.go"), 0o755) }},
		{"a dangling link", func(root string) error {
			os.Mkdir(filepath.Join(root, "auth"), 0o755)
			return os.Symlink("nowhere", filepath.Join(root, "auth", "models.go"))
		}},
	} {
		root := newProject(t)
		if err := c.setup(root); err != nil {
			t.Fatal(err)
		}
		written, ds := Add(root, "auth")
		want := "auth/models.go: something is in the way of maago add auth\n  fix: remove it, or run maago add in an empty directory and compare the two with git diff --no-index\n"
		if written != nil || text(ds) != want {
			t.Errorf("%s: %v\n%s", c.name, written, text(ds))
		}
		if c.name == "a file" {
			if got, _ := os.ReadFile(filepath.Join(root, "auth", "models.go")); string(got) != "mine" {
				t.Errorf("the file in the way became %q", got)
			}
		}
	}
}

func TestAddUnknownApp(t *testing.T) {
	_, ds := Add(newProject(t), "autth")
	if got := text(ds); got != "unknown app \"autth\"\n  fix: did you mean \"auth\"? apps: auth\n" {
		t.Errorf("%s", got)
	}
}

// A write that fails leaves no file of this run behind.
func TestAddCannotWrite(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anyway")
	}
	root := newProject(t)
	dir := filepath.Join(root, "auth")
	if err := os.Mkdir(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	written, ds := Add(root, "auth")
	if written != nil || !strings.HasPrefix(text(ds), "auth/models.go: cannot write the file: ") {
		t.Errorf("%v\n%s", written, text(ds))
	}
	if entries, _ := os.ReadDir(dir); len(entries) > 0 {
		t.Errorf("left %v", entries)
	}
}
