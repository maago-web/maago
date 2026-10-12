package scaffold

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
)

func newProject(t *testing.T) string {
	t.Helper()
	t.Setenv("MAAGO_DATABASE_URL", "sqlite:app.db")
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

// Add copies every file as it is, but of the files named for a dialect
// only the project's, under the name without the dialect.
func TestAdd(t *testing.T) {
	for _, c := range []struct{ url, driver string }{
		{"sqlite:app.db", "modernc.org/sqlite"},
		{"postgres://localhost/app", "github.com/jackc/pgx/v5/stdlib"},
	} {
		root := newProject(t)
		t.Setenv("MAAGO_DATABASE_URL", c.url)
		sub := filepath.Join(root, "cmd")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		written, ds := Add(sub, "auth") // from below the root, as gen finds it
		if ds != nil || !slices.Contains(written, "auth/models.go") || !slices.Contains(written, "auth/driver_test.go") {
			t.Fatalf("%v\n%s", written, text(ds))
		}
		for _, n := range written {
			got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(n)))
			want, _ := files.ReadFile("testdata/" + n)
			if n == "auth/driver_test.go" {
				if !strings.Contains(string(got), `import _ "`+c.driver+`"`) {
					t.Errorf("%s: %s", c.url, got)
				}
				continue
			}
			if err != nil || string(got) != string(want) {
				t.Errorf("%s is not the scaffold: %v", n, err)
			}
			if strings.HasPrefix(string(got), "// Code generated") {
				t.Errorf("%s: the copy is the project's, not generated (D-67)", n)
			}
		}
		if strings.Contains(strings.Join(written, " "), ".sqlite") || strings.Contains(strings.Join(written, " "), ".postgres") {
			t.Errorf("wrote %v", written)
		}
	}
}

// Without a database the driver to test with is unknown.
func TestAddNoDatabase(t *testing.T) {
	root := newProject(t)
	os.Unsetenv("MAAGO_DATABASE_URL")
	written, ds := Add(root, "auth")
	if written != nil || !strings.HasPrefix(text(ds), "MAAGO_DATABASE_URL is not set") {
		t.Errorf("%v\n%s", written, text(ds))
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
		{"the driver's file", func(root string) error {
			os.Mkdir(filepath.Join(root, "auth"), 0o755)
			return os.WriteFile(filepath.Join(root, "auth", "driver_test.go"), []byte("mine"), 0o644)
		}},
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
		in := map[bool]string{true: "auth/driver_test.go", false: "auth/models.go"}[c.name == "the driver's file"]
		want := in + ": something is in the way of maago add auth\n  fix: remove it, or run maago add in an empty directory and compare the two with git diff --no-index\n"
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

// A write that fails, here after others succeeded, leaves no file of
// this run behind.
func TestAddCannotWrite(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anyway")
	}
	root := newProject(t)
	dir := filepath.Join(root, "auth", "templates")
	os.Mkdir(filepath.Dir(dir), 0o755)
	if err := os.Mkdir(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	written, ds := Add(root, "auth")
	if got := text(ds); written != nil || !strings.HasPrefix(got, "auth/templates/") || !strings.Contains(got, ".html: cannot write the file: ") {
		t.Errorf("%v\n%s", written, text(ds))
	}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() != "go.mod" {
			t.Errorf("left %s", p)
		}
		return nil
	})
}
