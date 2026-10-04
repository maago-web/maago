package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/maago-web/maago/maagotest"
)

// enterProject makes the module example.com/app, its .env holding env and
// a migration, and goes into its package board/, as go test would.
func enterProject(t *testing.T, env string) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.25\n")
	writeFile(t, filepath.Join(root, ".env"), "MAAGO_DATABASE_URL="+env+"\n")
	writeFile(t, filepath.Join(root, "migrations", "0001_initial.sql"), "CREATE TABLE note (id INTEGER PRIMARY KEY, text TEXT NOT NULL);\n")
	if err := os.Mkdir(filepath.Join(root, "board"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(root, "board"))
}

func notes(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM note").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestMaagotest runs maagotest.DB (D-113) on both dialects: the root's
// .env found from the package's directory, the migrations applied, a
// database for each call that outlives none of its tests.
func TestMaagotest(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		noEnv(t)
		enterProject(t, "sqlite:app.db")
		checkFresh(t)
	})
	t.Run("postgres", func(t *testing.T) {
		pg := pgURL(t)
		noEnv(t)
		enterProject(t, pg+"?application_name=maagotest_check")
		checkFresh(t)
		checkSchemas(t, pg)
	})
	t.Run("the environment wins", func(t *testing.T) {
		enterProject(t, "postgres://127.0.0.1:1/nowhere")
		t.Setenv("MAAGO_DATABASE_URL", "sqlite:app.db")
		if notes(t, maagotest.DB(t)) != 0 {
			t.Error("not a new database")
		}
	})
}

// checkFresh: a database lives while its test does, and no other call
// sees it, the same test's or a subtest's.
func checkFresh(t *testing.T) {
	db := maagotest.DB(t)
	if _, err := db.Exec("INSERT INTO note (id, text) VALUES (1, 'parent')"); err != nil {
		t.Fatal(err)
	}
	t.Run("child", func(t *testing.T) {
		if notes(t, maagotest.DB(t)) != 0 {
			t.Error("the child sees the parent's row")
		}
	})
	if notes(t, maagotest.DB(t)) != 0 {
		t.Error("a second call in a test sees the first's row")
	}
	if notes(t, db) != 1 {
		t.Error("the parent's row went with the child")
	}
}

// checkSchemas: every connection of the pool searches the test's schema,
// the schema is gone when the test ends, and one a killed run left is
// dropped by name.
func checkSchemas(t *testing.T, pg string) {
	admin, err := sql.Open("pgx", pg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	exists := func(name string) bool {
		var ok bool
		if err := admin.QueryRow("SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)", name).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}

	var name string
	t.Run("pool", func(t *testing.T) {
		db := maagotest.DB(t)
		var conns []*sql.Conn
		for range 4 { // held at once: four connections, not one reused
			c, err := db.Conn(ctx(t))
			if err != nil {
				t.Fatal(err)
			}
			conns = append(conns, c)
		}
		for i, c := range conns {
			var s string
			if err := c.QueryRowContext(ctx(t), "SELECT current_schema()").Scan(&s); err != nil {
				t.Fatal(err)
			}
			if i == 0 {
				name = s
			}
			if s != name || s == "public" {
				t.Errorf("connection %d searches %q, connection 0 %q", i, s, name)
			}
			var app string
			if err := c.QueryRowContext(ctx(t), "SELECT current_setting('application_name')").Scan(&app); err != nil || app != "maagotest_check" {
				t.Errorf("connection %d lost the address's query: application_name %q %v", i, app, err)
			}
			c.Close()
		}
		if !exists(name) {
			t.Errorf("no schema %s while the test runs", name)
		}
	})
	if name == "" || exists(name) {
		t.Errorf("schema %q is left after its test", name)
	}

	left := maagotest.SchemaName("example.com/app/board", t.Name()+"/left_over", 1)
	if _, err := admin.Exec("CREATE SCHEMA " + left + "; CREATE TABLE " + left + ".stale (x int)"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec("DROP SCHEMA IF EXISTS " + left + " CASCADE") })
	t.Run("left over", func(t *testing.T) {
		var stale sql.NullString
		var schema string
		if err := maagotest.DB(t).QueryRow("SELECT to_regclass('stale')::text, current_schema()").Scan(&stale, &schema); err != nil {
			t.Fatal(err)
		}
		if schema != left {
			t.Fatalf("the schema is %s, not %s: the name is not the package's and the test's", schema, left)
		}
		if stale.Valid {
			t.Error("the schema a killed run left was kept")
		}
	})
}
