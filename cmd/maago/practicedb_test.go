package main

import (
	"context"
	"database/sql"
	"go/token"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/practicedb"
	"github.com/maago-web/maago/internal/project"
)

// pgURL returns the PostgreSQL server for tests. Without one, local runs
// skip and CI fails (D-91).
func pgURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("MAAGO_TEST_PG_URL")
	switch {
	case url != "":
	case os.Getenv("CI") != "":
		t.Fatal("MAAGO_TEST_PG_URL is not set; CI must run the PostgreSQL tests")
	default:
		t.Skip("MAAGO_TEST_PG_URL is not set; set it to a PostgreSQL server to run this test")
	}
	return url
}

// ctx fails a test that hangs, such as one blocked on a lock.
func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return c
}

func openPractice(t *testing.T, d project.Dialect, url string) *practicedb.DB {
	t.Helper()
	db, ds := practicedb.Open(ctx(t), d, url)
	if len(ds) > 0 {
		t.Fatalf("Open(%s):\n%s", d, text(ds))
	}
	t.Cleanup(db.Close)
	return db
}

func text(ds []diag.Diagnostic) string {
	var b strings.Builder
	diag.Write(&b, ds)
	return b.String()
}

var pos = token.Position{Filename: "migrations/0001_initial.sql", Line: 3}

// exec runs stmt and returns its diagnostics as text.
func exec(t *testing.T, db *practicedb.DB, stmt string) string {
	t.Helper()
	return text(db.Exec(ctx(t), pos, stmt))
}

func mustExec(t *testing.T, db *practicedb.DB, stmts ...string) {
	t.Helper()
	for _, s := range stmts {
		if out := exec(t, db, s); out != "" {
			t.Fatalf("%s:\n%s", s, out)
		}
	}
}

func TestPracticeSQLite(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir) // where a wrongly opened database file would land
	a := openPractice(t, project.SQLite, "sqlite:app.db")
	b := openPractice(t, project.SQLite, "sqlite:app.db")

	mustExec(t, a,
		"CREATE TABLE author (id INTEGER PRIMARY KEY)",
		"CREATE TABLE post (id INTEGER PRIMARY KEY, author_id INTEGER REFERENCES author (id))")
	if out := exec(t, b, "SELECT * FROM author"); !strings.Contains(out, "no such table") {
		t.Errorf("b sees a's table:\n%s", out)
	}
	out := exec(t, a, "INSERT INTO post VALUES (1, 99)")
	if !strings.HasPrefix(out, "migrations/0001_initial.sql:3: ") || !strings.Contains(out, "FOREIGN KEY") {
		t.Errorf("want a foreign key error at the statement, got:\n%s", out)
	}
	if files, _ := os.ReadDir(dir); len(files) > 0 {
		t.Errorf("the practice database wrote %s", files[0].Name())
	}
}

func TestPracticePostgres(t *testing.T) {
	url := pgURL(t)
	admin, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() }) // after the DROP below: cleanups run last first
	adminExec := func(q string) {
		t.Helper()
		if _, err := admin.ExecContext(ctx(t), q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	count := func(q string) (n int) {
		t.Helper()
		if err := admin.QueryRowContext(ctx(t), q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n
	}
	const schemas = "SELECT count(*) FROM pg_namespace WHERE starts_with(nspname, 'maago_practice_')"
	adminExec("DROP TABLE IF EXISTS public.maago_test_user, public.maago_test_new")
	adminExec("CREATE TABLE public.maago_test_user (id int)")
	adminExec("INSERT INTO public.maago_test_user VALUES (1)")
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP TABLE IF EXISTS public.maago_test_user, public.maago_test_new"); err != nil {
			t.Error(err)
		}
	})

	// One run opens two practice databases at once.
	a := openPractice(t, project.Postgres, url)
	b := openPractice(t, project.Postgres, url)

	// A failed statement aborts the transaction, so this one gets its own.
	if out := exec(t, openPractice(t, project.Postgres, url), "SELECT * FROM maago_test_user"); !strings.Contains(out, "does not exist") {
		t.Errorf("the practice database sees the user's table:\n%s", out)
	}
	mustExec(t, a, "CREATE TABLE maago_test_user (x text)")
	mustExec(t, b, "CREATE TABLE maago_test_user (y text)")
	// Even statements naming the user's schema leave no trace (D-96).
	mustExec(t, a, "CREATE TABLE public.maago_test_new (id int)", "DELETE FROM public.maago_test_user")
	if n := count(schemas); n != 0 {
		t.Errorf("other sessions see %d practice schemas", n)
	}

	a.Close()
	b.Close()
	if n := count(schemas); n != 0 {
		t.Errorf("%d practice schemas left after Close", n)
	}
	if n := count("SELECT count(*) FROM pg_class WHERE oid = to_regclass('public.maago_test_new')"); n != 0 {
		t.Error("public.maago_test_new was created")
	}
	if n := count("SELECT count(*) FROM public.maago_test_user"); n != 1 {
		t.Errorf("public.maago_test_user has %d rows, want 1", n)
	}
}

// A server that is not running is reported with how to start it (D-64),
// and the password stays out of the message.
func TestPracticePostgresRefused(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	for _, url := range []string{
		"postgres://maago:s3cret@" + addr + "/app",
		"postgres://maago@" + addr + "/app?password=s3cret",
		"postgres://maago:s3cret@" + addr + "/app?connect_timeout=x", // malformed
	} {
		db, ds := practicedb.Open(ctx(t), project.Postgres, url)
		out := text(ds)
		if db != nil || len(ds) != 1 || !strings.HasPrefix(out, "cannot open the practice database: ") ||
			!strings.HasSuffix(out, "\n  fix: start the PostgreSQL server, check MAAGO_DATABASE_URL, and make sure its user may create schemas (GRANT CREATE ON DATABASE)\n") {
			t.Errorf("got %v\n%s", db, out)
		}
		if strings.Contains(out, "s3cret") {
			t.Errorf("the message shows the password:\n%s", out)
		}
	}
}
