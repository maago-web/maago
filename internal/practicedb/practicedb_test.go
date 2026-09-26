package practicedb

import (
	"context"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/project"
)

// The root module registers no driver (D-90), so Open fails before it
// tries to connect, and says how to get a build that has one.
func TestOpenWithoutDriver(t *testing.T) {
	for d, name := range map[project.Dialect]string{project.SQLite: "sqlite", project.Postgres: "pgx"} {
		db, ds := Open(context.Background(), d, "postgres://maago:s3cret@localhost/app")
		var b strings.Builder
		diag.Write(&b, ds)
		want := `no "` + name + `" database driver in this maago build` +
			"\n  fix: build maago from its cmd/maago module, which registers the drivers\n"
		if db != nil || b.String() != want {
			t.Errorf("%s: got %v\n%s\nwant nil\n%s", d, db, b.String(), want)
		}
	}
}
