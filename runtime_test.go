package maago

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// The runtime needs no driver (D-90); without one, OpenDB says what to import.
func TestOpenDBWithoutDriver(t *testing.T) {
	_, err := OpenDB(context.Background(), DBConfig{URL: "sqlite:app.db"})
	if err == nil || !strings.Contains(err.Error(), `import _ "modernc.org/sqlite" in package main`) {
		t.Errorf("got %v", err)
	}
	_, err = OpenDB(context.Background(), DBConfig{URL: "mysql://x"})
	if err == nil || !strings.Contains(err.Error(), `DBConfig.URL starts with "mysql:"`) {
		t.Errorf("got %v", err)
	}
}

// The runtime depends on the standard library and, of this module, only
// on what reads the address the way the CLI does, and on the migrations
// Migrate shares with it (D-153). A package added to internal/project
// must not bring more (D-09).
func TestRuntimeDependencies(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(string(out))
	want := []string{"github.com/maago-web/maago/internal/diag", "github.com/maago-web/maago/internal/ref", "github.com/maago-web/maago/internal/model", "github.com/maago-web/maago/internal/schema",
		"github.com/maago-web/maago/internal/dialect", "github.com/maago-web/maago/internal/project", "github.com/maago-web/maago/internal/practicedb",
		"github.com/maago-web/maago/internal/migrate", "github.com/maago-web/maago/internal/rtcore", "github.com/maago-web/maago"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("the runtime depends on %q, want %q", got, want)
	}
	// rt is in every app's binary too: the same, and itself (D-152).
	out, err = exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", "./rt").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Fields(string(out)), append(want, "github.com/maago-web/maago/rt"); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("rt depends on %q, want %q", got, want)
	}

	// The library the copied auth app calls is the standard library only
	// (D-123).
	out, err = exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", "./contrib/auth").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "github.com/maago-web/maago/contrib/auth" {
		t.Errorf("contrib/auth depends on %q", got)
	}
}

func TestOpt(t *testing.T) {
	var unset Opt[string]
	if got := unset.Or("draft"); got != "draft" {
		t.Errorf("unset: %q", got)
	}
	if got := Set("").Or("draft"); got != "" {
		t.Errorf("set to the zero value: %q", got)
	}
	var none Opt[*float64]
	if got := none.Or(Ptr(1.5)); got == nil || *got != 1.5 {
		t.Errorf("pointer default: %v", got)
	}
	if got := Set[*float64](nil).Or(Ptr(1.5)); got != nil {
		t.Errorf("set to nil: %v", got)
	}
}
