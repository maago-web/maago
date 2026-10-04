package maagotest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fatalTB records the first Fatal and stops the goroutine there, as the
// testing package does.
type fatalTB struct {
	testing.TB
	msg string
}

func (f *fatalTB) Helper() {}
func (f *fatalTB) Fatal(args ...any) {
	f.msg = fmt.Sprint(args...)
	runtime.Goexit()
}
func (f *fatalTB) Fatalf(format string, args ...any) { f.Fatal(fmt.Sprintf(format, args...)) }

// fatal runs DB in project dir and returns what it failed with.
func fatal(t *testing.T, files map[string]string) string {
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	f := &fatalTB{TB: t}
	done := make(chan struct{})
	go func() { defer close(done); DB(f) }()
	<-done
	return f.msg
}

// This module has no driver: the errors DB gives before one is needed,
// and the one for the missing driver.
func TestDBErrors(t *testing.T) {
	t.Setenv("MAAGO_DATABASE_URL", "")
	os.Unsetenv("MAAGO_DATABASE_URL")
	for _, c := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"no module", map[string]string{}, "no go.mod"},
		{"no address", map[string]string{"go.mod": "module example.com/app\n"}, "MAAGO_DATABASE_URL is not set"},
		{"no driver", map[string]string{"go.mod": "module example.com/app\n", ".env": "MAAGO_DATABASE_URL=sqlite:app.db\n"}, "no \"sqlite\" database driver\n  fix: import _ \"modernc.org/sqlite\" in a _test.go file of this package"},
	} {
		if got := fatal(t, c.files); !strings.Contains(got, c.want) {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSchemaName(t *testing.T) {
	a := SchemaName("example.com/app/board", "TestPost", 1)
	if len(a) != len("maagotest_")+16 || !strings.HasPrefix(a, "maagotest_") || strings.ToLower(a) != a {
		t.Errorf("%q", a)
	}
	for _, b := range []string{SchemaName("example.com/app/blog", "TestPost", 1), SchemaName("example.com/app/board", "TestPosts", 1), SchemaName("example.com/app/board", "TestPost", 2)} {
		if a == b {
			t.Errorf("%q twice", a)
		}
	}
}
