// Package gentest runs the code maago generates. Its subpackages,
// sqlite/APP and postgres/APP, each hold a fixture app's models.go, the
// files generated for it, which are the goldens of TestGenerated, and a
// test that calls them on a real database. The test of an app is the same
// file in both dialects: Open tells the dialect by the directory.
package gentest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/dialect/postgres"
	"github.com/maago-web/maago/internal/dialect/sqlite"
	"github.com/maago-web/maago/internal/model/modeltest"
)

// Open returns a connection to a new, empty database holding the tables
// of the fixture at root, a directory of the repository, made by maago's
// DDL: SQLite in memory, or a PostgreSQL schema dropped after the test.
// Without MAAGO_TEST_PG_URL, a PostgreSQL test is skipped, or fails in CI.
func Open(t *testing.T, root string) (context.Context, *sql.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	apps, fset := modeltest.Example(t, filepath.Join("../../../../../..", root))
	driver, dsn, ddl, setup := "sqlite", ":memory:", sqlite.DDL, []string{"PRAGMA foreign_keys = ON"}
	if filepath.Base(filepath.Dir(wd)) == "postgres" {
		dsn = os.Getenv("MAAGO_TEST_PG_URL")
		switch {
		case dsn != "":
		case os.Getenv("CI") != "":
			t.Fatal("MAAGO_TEST_PG_URL is not set; CI must run the PostgreSQL tests")
		default:
			t.Skip("MAAGO_TEST_PG_URL is not set; set it to a PostgreSQL server to run this test")
		}
		schema := "maago_gentest_" + strings.ToLower(rand.Text())
		driver, ddl, setup = "pgx", postgres.DDL, []string{"CREATE SCHEMA " + schema, "SET search_path TO " + schema}
		t.Cleanup(func() {
			db, err := sql.Open("pgx", dsn)
			if err == nil {
				db.Exec("DROP SCHEMA " + schema + " CASCADE")
				db.Close()
			}
		})
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	src, ds := ddl(fset, apps)
	if len(ds) > 0 {
		t.Fatalf("DDL: %v", ds)
	}
	stmts, _ := dialect.Split("", src)
	for _, s := range stmts {
		setup = append(setup, s.SQL)
	}
	for _, s := range setup {
		if _, err := conn.ExecContext(ctx, s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return ctx, conn
}
