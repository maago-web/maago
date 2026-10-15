package maago

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSQLiteTime(t *testing.T) {
	kst := time.FixedZone("KST", 9*3600)
	for _, c := range []struct {
		in   time.Time
		want string
	}{
		{time.Date(2026, 9, 27, 12, 0, 5, 500_000_000, kst), "2026-09-27 03:00:05.500000"},
		{time.Date(2026, 9, 27, 12, 0, 5, 0, time.UTC), "2026-09-27 12:00:05.000000"},
		{time.Date(2026, 9, 27, 12, 0, 5, 123_456_789, time.UTC), "2026-09-27 12:00:05.123456"},
	} {
		if got := SQLiteTime(c.in); got != c.want {
			t.Errorf("SQLiteTime(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestScanSQLiteTime(t *testing.T) {
	want := time.Date(2026, 9, 27, 3, 0, 5, 250_000_000, time.UTC)
	for _, src := range []any{
		"2026-09-27 03:00:05.250000",
		"2026-09-27 03:00:05.25",
		[]byte("2026-09-27 03:00:05.25"),
		"2026-09-27T12:00:05.25+09:00",
		"2026-09-27T03:00:05.25Z",
	} {
		var got time.Time
		if err := ScanSQLiteTime(&got).Scan(src); err != nil || !got.Equal(want) || got.Location() != time.UTC {
			t.Errorf("%v: got %v %v, want %v in UTC", src, got, err, want)
		}
	}
	for _, src := range []any{nil, "2026-09-27 12:00:05.5 +0900 KST", "yesterday", int64(5)} {
		var got time.Time
		if err := ScanSQLiteTime(&got).Scan(src); err == nil {
			t.Errorf("%v: no error", src)
		}
	}
	p := &want
	if err := ScanNullSQLiteTime(&p).Scan(nil); err != nil || p != nil {
		t.Errorf("NULL: got %v %v, want nil", p, err)
	}
	if err := ScanNullSQLiteTime(&p).Scan("2026-09-27 03:00:05.25"); err != nil || p == nil || !p.Equal(want) {
		t.Errorf("got %v %v, want %v", p, err, want)
	}
}

func TestPlaceholders(t *testing.T) {
	if got := Placeholders("$", 3, 2); got != "$3, $4" {
		t.Errorf("got %q", got)
	}
	if got := Placeholders("?", 1, 1); got != "?1" {
		t.Errorf("got %q", got)
	}
	defer func() {
		if recover() == nil {
			t.Error("n=0 did not panic")
		}
	}()
	Placeholders("$", 1, 0)
}

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
	want := []string{"github.com/maago-web/maago/internal/diag", "github.com/maago-web/maago/internal/model", "github.com/maago-web/maago/internal/schema",
		"github.com/maago-web/maago/internal/dialect", "github.com/maago-web/maago/internal/project", "github.com/maago-web/maago/internal/practicedb",
		"github.com/maago-web/maago/internal/migrate", "github.com/maago-web/maago"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("the runtime depends on %q, want %q", got, want)
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

func TestSQLiteNullTime(t *testing.T) {
	if SQLiteNullTime(nil) != nil {
		t.Error("nil is not NULL")
	}
	at := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	if got := SQLiteNullTime(&at); got != "2026-09-27 12:00:00.000000" {
		t.Errorf("got %v", got)
	}
}
