package main

import (
	"crypto/rand"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/build"
	"github.com/maago-web/maago/internal/migrate"
	"github.com/maago-web/maago/internal/practicedb"
	"github.com/maago-web/maago/internal/project"
)

// realDB is where migrate goes: a SQLite file in the project, or a new
// PostgreSQL database for the test, dropped after it. The server's user
// needs CREATEDB.
type realDB struct {
	d         project.Dialect
	url, root string
}

func newRealDB(t *testing.T, d project.Dialect) realDB {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	if d == project.SQLite {
		t.Setenv("MAAGO_DATABASE_URL", "sqlite:app.db")
		return realDB{d, "sqlite:app.db", root}
	}
	server := pgURL(t)
	admin, err := sql.Open("pgx", server)
	if err != nil {
		t.Fatal(err)
	}
	name := "maago_test_" + strings.ToLower(rand.Text())
	// Backslashes escape by default in this database, so that a test can
	// see migrate make them plain, as dialect.Split reads them.
	for _, stmt := range []string{"CREATE DATABASE " + name, "ALTER DATABASE " + name + " SET standard_conforming_strings = off"} {
		if _, err := admin.ExecContext(ctx(t), stmt); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		admin.Exec("DROP DATABASE " + name + " WITH (FORCE)")
		admin.Close()
	})
	u, err := url.Parse(server)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	t.Setenv("MAAGO_DATABASE_URL", u.String())
	return realDB{d, u.String(), root}
}

// open opens the database for the test to look at; it closes with the test.
func (r realDB) open(t *testing.T) *practicedb.DB {
	t.Helper()
	db, ds := practicedb.OpenDatabase(ctx(t), r.d, r.url, r.root)
	if len(ds) > 0 {
		t.Fatal(text(ds))
	}
	t.Cleanup(db.Close)
	return db
}

func (r realDB) write(t *testing.T, files map[string]string) {
	t.Helper()
	for name, src := range files {
		writeFile(t, filepath.Join(r.root, "migrations", name), src)
	}
}

func (r realDB) migrate(t *testing.T, want ...string) {
	t.Helper()
	applied, ds := build.Migrate(r.root)
	if len(ds) > 0 || !slices.Equal(applied, want) {
		t.Fatalf("migrate applied %v, want %v\n%s", applied, want, text(ds))
	}
}

func (r realDB) show(t *testing.T) string {
	t.Helper()
	list, ds := build.ShowMigrations(r.root)
	if len(ds) > 0 {
		t.Fatalf("showmigrations:\n%s", text(ds))
	}
	return list
}

var threeFiles = map[string]string{
	"0001_a.sql":     "CREATE TABLE a (id INTEGER PRIMARY KEY, name TEXT NOT NULL);\n",
	"0002_b.sql":     "-- b points at a\nCREATE TABLE b (id INTEGER PRIMARY KEY, a_id INTEGER REFERENCES a (id));\n",
	"0003_add_y.sql": "ALTER TABLE a ADD COLUMN y TEXT;\n",
}

func eachDialect(t *testing.T, f func(t *testing.T, d project.Dialect)) {
	for _, d := range []project.Dialect{project.SQLite, project.Postgres} {
		t.Run(string(d), func(t *testing.T) { f(t, d) })
	}
}

func TestMigrate(t *testing.T) {
	eachDialect(t, func(t *testing.T, d project.Dialect) {
		r := newRealDB(t, d)
		if got := r.show(t); got != "no migrations\n" {
			t.Errorf("show without migrations: %q", got)
		}
		// Nothing to apply leaves nothing in the database.
		r.migrate(t)
		if d == project.SQLite {
			if got := rowsText(t, r.open(t), "SELECT name FROM sqlite_schema"); got != "" {
				t.Errorf("tables after migrating nothing: %q", got)
			}
			os.Remove(filepath.Join(r.root, "app.db"))
		}

		r.write(t, threeFiles)
		if got, want := r.show(t), "[ ] 0001_a.sql\n[ ] 0002_b.sql\n[ ] 0003_add_y.sql\n0 of 3 applied\n"; got != want {
			t.Errorf("show before:\n%s", got)
		}
		if _, err := os.Stat(filepath.Join(r.root, "app.db")); d == project.SQLite && err == nil {
			t.Error("showmigrations created the SQLite file")
		}
		// From a subdirectory, the file is still the root's.
		writeFile(t, filepath.Join(r.root, "sub", "x.go"), "package sub\n")
		applied, ds := build.Migrate(filepath.Join(r.root, "sub"))
		if len(ds) > 0 || len(applied) != 3 {
			t.Fatalf("migrate: %v\n%s", applied, text(ds))
		}
		if _, err := os.Stat(filepath.Join(r.root, "sub", "app.db")); err == nil {
			t.Error("migrate made its SQLite file in the subdirectory")
		}
		if got, want := r.show(t), "[x] 0001_a.sql\n[x] 0002_b.sql\n[x] 0003_add_y.sql\n3 of 3 applied\n"; got != want {
			t.Errorf("show after:\n%s", got)
		}
		var before []byte
		if d == project.SQLite {
			before, _ = os.ReadFile(filepath.Join(r.root, "app.db"))
		}
		r.migrate(t)
		if after, _ := os.ReadFile(filepath.Join(r.root, "app.db")); d == project.SQLite && string(after) != string(before) {
			t.Error("migrating again changed the database file")
		}
	})
}

// A file changed after it was applied stops both commands (D-95); line
// endings alone are no change.
func TestMigrateChanged(t *testing.T) {
	eachDialect(t, func(t *testing.T, d project.Dialect) {
		r := newRealDB(t, d)
		r.write(t, threeFiles)
		r.migrate(t, "0001_a.sql", "0002_b.sql", "0003_add_y.sql")
		r.write(t, map[string]string{
			"0001_a.sql": strings.ReplaceAll(threeFiles["0001_a.sql"], "\n", "\r\n"),
			"0002_b.sql": strings.Replace(threeFiles["0002_b.sql"], "points", "Points", 1),
			"0004_z.sql": "CREATE TABLE z (id INTEGER);\n",
		})
		_, mds := build.Migrate(r.root)
		list, sds := build.ShowMigrations(r.root)
		if list != "" {
			t.Errorf("showmigrations listed despite the error:\n%s", list)
		}
		for _, out := range []string{text(mds), text(sds)} {
			if !strings.HasPrefix(out, "migrations/0002_b.sql: migration was changed after it was applied") || strings.Count(out, "fix:") != 1 {
				t.Errorf("got:\n%s", out)
			}
		}
		if got := rowsText(t, r.open(t), "SELECT name FROM "+migrate.Table+" ORDER BY name"); got != "0001_a.sql\n0002_b.sql\n0003_add_y.sql\n" {
			t.Errorf("migrate applied despite the change: %q", got)
		}
	})
}

// A renumbered file, as merging two branches makes, is found by its hash;
// its fix, run on the database, lets migrate go on, and the other
// branch's file is applied behind it (D-99).
func TestMigrateRenumbered(t *testing.T) {
	eachDialect(t, func(t *testing.T, d project.Dialect) {
		r := newRealDB(t, d)
		r.write(t, threeFiles)
		r.migrate(t, "0001_a.sql", "0002_b.sql", "0003_add_y.sql")
		os.Rename(filepath.Join(r.root, "migrations", "0003_add_y.sql"), filepath.Join(r.root, "migrations", "0004_add_y.sql"))
		r.write(t, map[string]string{"0003_x.sql": "CREATE TABLE x (id INTEGER);\n"})

		_, ds := build.Migrate(r.root)
		out := text(ds)
		fix := "UPDATE maago_migrations SET name = '0004_add_y.sql' WHERE name = '0003_add_y.sql';"
		if len(ds) != 1 || !strings.HasPrefix(out, "migrations/0004_add_y.sql: migration was applied as 0003_add_y.sql") || !strings.Contains(out, fix) {
			t.Fatalf("got:\n%s", out)
		}
		if list, _ := build.ShowMigrations(r.root); strings.Contains(list, "[x] 0003_x.sql") {
			t.Error("0003_x was applied despite the renumbered file")
		}
		mustExec(t, r.open(t), fix)
		r.migrate(t, "0003_x.sql")
		if got := r.show(t); got != "[x] 0001_a.sql\n[x] 0002_b.sql\n[x] 0003_x.sql\n[x] 0004_add_y.sql\n4 of 4 applied\n" {
			t.Errorf("show:\n%s", got)
		}

		// A record whose file is gone, and one whose hash two files share.
		same := "SELECT 1;\n"
		r.write(t, map[string]string{"0005_p.sql": same, "0006_q.sql": same})
		files, _ := migrate.ReadFiles(r.root)
		hash := files[len(files)-1].Hash
		mustExec(t, r.open(t),
			"INSERT INTO maago_migrations VALUES ('0009_gone.sql', 'abc', 'now')",
			"INSERT INTO maago_migrations VALUES ('0010_twin.sql', '"+hash+"', 'now')")
		_, ds = build.Migrate(r.root)
		out = text(ds)
		if len(ds) != 2 || !strings.Contains(out, "applied migration 0009_gone.sql has no file") ||
			!strings.Contains(out, "applied migration 0010_twin.sql matches several files: migrations/0005_p.sql, migrations/0006_q.sql") {
			t.Errorf("got:\n%s", out)
		}
	})
}

// A statement that fails rolls back its file, record and all; the files
// before it stay.
func TestMigrateFails(t *testing.T) {
	eachDialect(t, func(t *testing.T, d project.Dialect) {
		r := newRealDB(t, d)
		r.write(t, map[string]string{
			"0001_a.sql": threeFiles["0001_a.sql"],
			"0002_b.sql": "CREATE TABLE b (id INTEGER);\nINSERT INTO a (id, name) VALUES (1, 'x');\nINSERT INTO nope VALUES (1);\n",
			"0003_c.sql": "CREATE TABLE c (id INTEGER);\n",
		})
		applied, ds := build.Migrate(r.root)
		if out := text(ds); !slices.Equal(applied, []string{"0001_a.sql"}) || len(ds) != 1 || !strings.HasPrefix(out, "migrations/0002_b.sql:3:") {
			t.Fatalf("applied %v:\n%s", applied, out)
		}
		db := r.open(t)
		if got := rowsText(t, db, "SELECT name FROM "+migrate.Table); got != "0001_a.sql\n" {
			t.Errorf("records: %q", got)
		}
		if got := rowsText(t, db, "SELECT name FROM a"); got != "" {
			t.Errorf("rows of a: %q", got)
		}
		if got := r.show(t); !strings.Contains(got, "[ ] 0002_b.sql\n[ ] 0003_c.sql") {
			t.Errorf("show:\n%s", got)
		}
		table := "SELECT name FROM sqlite_schema WHERE name = 'b'"
		if d == project.Postgres {
			table = "SELECT coalesce(to_regclass('b')::text, '')"
		}
		if got := strings.TrimSpace(rowsText(t, db, table)); got != "" {
			t.Errorf("table b is left: %q", got)
		}
	})
}

// SQLite rebuilds keep rows and the foreign keys pointing at them; one
// that breaks a foreign key is rolled back, and foreign keys are on again.
func TestMigrateSQLiteRebuild(t *testing.T) {
	r := newRealDB(t, project.SQLite)
	schema := `CREATE TABLE p (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL) STRICT;
CREATE TABLE c (id INTEGER PRIMARY KEY AUTOINCREMENT, p_id INTEGER NOT NULL REFERENCES p (id)) STRICT;
INSERT INTO p (id, name) VALUES (1, 'a'), (2, 'b');
INSERT INTO c (p_id) VALUES (1), (2);
`
	rebuild := func(copy, marker string) string {
		return `CREATE TABLE maago_new_p (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL CHECK (length(name) <= 9)) STRICT;
` + copy + "\n" + marker + `DROP TABLE p;
ALTER TABLE maago_new_p RENAME TO p;
`
	}
	r.write(t, map[string]string{
		"0001_p.sql":       schema,
		"0002_rebuild.sql": rebuild("INSERT INTO maago_new_p (id, name) SELECT id, name FROM p;", ""),
	})
	r.migrate(t, "0001_p.sql", "0002_rebuild.sql")
	db := r.open(t)
	if got := rowsText(t, db, "SELECT c.id || ' ' || p.name FROM c JOIN p ON p.id = c.p_id ORDER BY c.id"); got != "1 a\n2 b\n" {
		t.Errorf("rows after the rebuild: %q", got)
	}

	// Without its marker, a rebuild that loses a column fails, even with
	// no check before migrate (D-93); the database is as it was.
	lossy := strings.Replace(rebuild("INSERT INTO maago_new_p (id) SELECT id FROM p;", ""), "name TEXT NOT NULL CHECK (length(name) <= 9)", "name TEXT", 1)
	r.write(t, map[string]string{"0003_lossy.sql": lossy})
	if _, ds := build.Migrate(r.root); !strings.Contains(text(ds), `DROP TABLE "p" loses the values of "name"`) {
		t.Errorf("lossy rebuild:\n%s", text(ds))
	}
	if got := rowsText(t, db, "SELECT name FROM p ORDER BY id"); got != "a\nb\n" {
		t.Errorf("rows after the lossy rebuild: %q", got)
	}
	os.Remove(filepath.Join(r.root, "migrations", "0003_lossy.sql"))

	// A marked rebuild that leaves out the row c points at breaks its
	// foreign key: rolled back, with foreign keys on again after.
	r.write(t, map[string]string{"0003_break.sql": rebuild("INSERT INTO maago_new_p (id, name) SELECT id, name FROM p WHERE id = 1;", "-- maago: allow-drop\n")})
	files, ds := migrate.ReadFiles(r.root)
	if ds := migrate.CheckFiles(files); len(ds) > 0 {
		t.Fatal(text(ds))
	}
	applied, ds := migrate.Run(ctx(t), db, project.SQLite, files[2:])
	if out := text(ds); len(applied) > 0 || !strings.Contains(out, "migrations/0003_break.sql: after this migration, a row of \"c\" has a foreign key that points nowhere") {
		t.Errorf("applied %v:\n%s", applied, out)
	}
	if got := rowsText(t, db, "SELECT CAST(count(*) AS TEXT) FROM p"); got != "2\n" {
		t.Errorf("rows of p after the rollback: %q", got)
	}
	if got := rowsText(t, db, "SELECT CAST(foreign_keys AS TEXT) FROM pragma_foreign_keys"); got != "1\n" {
		t.Errorf("foreign_keys after the rollback: %q", got)
	}
}

// Migrations the static checks refuse never reach the database.
func TestMigrateRefused(t *testing.T) {
	r := newRealDB(t, project.SQLite)
	r.write(t, map[string]string{"0001_a.sql": "CREATE TABLE a (id INTEGER);\nDROP TABLE a;\n"})
	if _, ds := build.Migrate(r.root); !strings.Contains(text(ds), "migrations/0001_a.sql:2:1: DROP TABLE deletes data") {
		t.Errorf("got:\n%s", text(ds))
	}
	if _, err := os.Stat(filepath.Join(r.root, "app.db")); err == nil {
		t.Error("migrate opened the database despite the error")
	}
	t.Setenv("MAAGO_DATABASE_URL", "sqlite:app.db?mode=rwc")
	if _, ds := build.Migrate(r.root); !strings.Contains(text(ds), "has ? or file: in its file name") {
		t.Errorf("got:\n%s", text(ds))
	}
}

// A backslash in a string is plain in both dialects, as dialect.Split
// reads it, even where the server would take it as an escape.
func TestMigrateBackslash(t *testing.T) {
	eachDialect(t, func(t *testing.T, d project.Dialect) {
		r := newRealDB(t, d)
		r.write(t, map[string]string{"0001_a.sql": "CREATE TABLE a (name TEXT);\nINSERT INTO a VALUES ('x\\y');\n"})
		r.migrate(t, "0001_a.sql")
		if got := rowsText(t, r.open(t), "SELECT name FROM a"); got != "x\\y\n" {
			t.Errorf("got %q", got)
		}
	})
}
