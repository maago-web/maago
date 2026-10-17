package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/rt"
)

// sqliteFile makes an empty SQLite database app.db in a new working
// directory, as maago migrate would.
func sqliteFile(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	db, err := sql.Open("sqlite", "app.db")
	if err == nil {
		_, err = db.Exec("CREATE TABLE t (x INTEGER)")
		db.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
}

func openDB(t *testing.T, cfg maago.DBConfig) *sql.DB {
	t.Helper()
	db, err := maago.OpenDB(ctx(t), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// The address: the environment variable beats .env, and DBConfig.URL
// beats both without reading .env at all (D-101).
func TestOpenDBAddress(t *testing.T) {
	sqliteFile(t)
	os.WriteFile(".env", []byte("MAAGO_DATABASE_URL=sqlite:app.db\n"), 0o644)
	t.Setenv("MAAGO_DATABASE_URL", "")
	os.Unsetenv("MAAGO_DATABASE_URL")
	openDB(t, maago.DBConfig{}) // from .env

	t.Setenv("MAAGO_DATABASE_URL", "sqlite:other.db")
	wd, _ := os.Getwd()
	if _, err := maago.OpenDB(ctx(t), maago.DBConfig{}); err == nil || !strings.Contains(err.Error(), `cannot open SQLite file "other.db": it is not in the working directory `+wd) ||
		!strings.Contains(err.Error(), "call maago.Migrate before OpenDB") {
		t.Errorf("the environment should win over .env and other.db not exist; got %v", err)
	}

	os.WriteFile(".env", []byte("broken line\n"), 0o644)
	openDB(t, maago.DBConfig{URL: "sqlite:app.db"})
	// DBConfig.URL does not even read .env.
	os.Rename(".env", "env.txt")
	os.Mkdir(".env", 0o755)
	openDB(t, maago.DBConfig{URL: "sqlite:app.db"})
	os.Remove(".env")
	os.Rename("env.txt", ".env")
	if _, err := maago.OpenDB(ctx(t), maago.DBConfig{}); err == nil || !strings.Contains(err.Error(), ".env:1: line without =") {
		t.Errorf("got %v, want the .env error", err)
	}
	os.Remove(".env")
	os.Unsetenv("MAAGO_DATABASE_URL")
	if _, err := maago.OpenDB(ctx(t), maago.DBConfig{}); err == nil || !strings.Contains(err.Error(), "MAAGO_DATABASE_URL is not set") {
		t.Errorf("got %v, want not set", err)
	}
}

// Every connection of the pool has the settings, not just the first (TODO: PRAGMA).
func TestOpenDBSQLiteConnections(t *testing.T) {
	sqliteFile(t)
	if err := os.Rename("app.db", "a#b%.db"); err != nil {
		t.Fatal(err)
	}
	db := openDB(t, maago.DBConfig{URL: "sqlite:a#b%.db"}) // # and % are escaped in the driver's address
	var conns []*sql.Conn
	for range 2 {
		c, err := db.Conn(ctx(t))
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		conns = append(conns, c)
	}
	for i, c := range conns {
		for pragma, want := range map[string]string{"foreign_keys": "1", "busy_timeout": "5000", "journal_mode": "wal"} {
			var got string
			if err := c.QueryRowContext(ctx(t), "PRAGMA "+pragma).Scan(&got); err != nil || got != want {
				t.Errorf("connection %d: %s = %q %v, want %s", i, pragma, got, err, want)
			}
		}
	}
}

func TestOpenDBPool(t *testing.T) {
	sqliteFile(t)
	pg := "postgres"
	for _, c := range []struct {
		url              string
		cfg              maago.DBConfig
		maxOpen, idle, n int // n connections are taken, then put back
	}{
		{"sqlite:app.db", maago.DBConfig{}, 4, 4, 4},
		{pg, maago.DBConfig{}, 20, 5, 6},
		{pg, maago.DBConfig{MaxOpenConns: 3, MaxIdleConns: 2}, 3, 2, 3},
		{pg, maago.DBConfig{MaxIdleConns: -1}, 20, 0, 2},
		{pg, maago.DBConfig{MaxOpenConns: -1}, 0, 2, 2}, // 0: no limit
	} {
		if c.url == pg {
			c.url = pgURL(t)
		}
		c.cfg.URL = c.url
		db := openDB(t, c.cfg)
		var conns []*sql.Conn
		for range c.n {
			conn, err := db.Conn(ctx(t))
			if err != nil {
				t.Fatal(err)
			}
			conns = append(conns, conn)
		}
		for _, conn := range conns {
			conn.Close()
		}
		if s := db.Stats(); s.MaxOpenConnections != c.maxOpen || s.Idle != c.idle {
			t.Errorf("%s %+v: max open %d, idle %d; want %d, %d", c.url[:6], c.cfg, s.MaxOpenConnections, s.Idle, c.maxOpen, c.idle)
		}
	}
	// A lifetime closes the connection when it is next handed out. The
	// idle time is not tested: database/sql's cleaner enforces it, once a
	// second at most.
	for _, cfg := range []maago.DBConfig{{ConnMaxLifetime: time.Nanosecond}} {
		cfg.URL = "sqlite:app.db"
		db := openDB(t, cfg)
		for range 2 {
			c, err := db.Conn(ctx(t))
			if err != nil {
				t.Fatal(err)
			}
			time.Sleep(time.Millisecond)
			c.Close()
		}
		if s := db.Stats(); s.MaxLifetimeClosed == 0 {
			t.Errorf("%+v: no connection closed for its age: %+v", cfg, s)
		}
	}
}

func TestOneMany(t *testing.T) {
	sqliteFile(t)
	db := openDB(t, maago.DBConfig{URL: "sqlite:app.db"})
	if _, err := db.Exec("INSERT INTO t VALUES (1), (2)"); err != nil {
		t.Fatal(err)
	}
	scan := func(r *sql.Rows) (int64, error) {
		var x int64
		return x, r.Scan(&x)
	}
	c := context.Background()
	if v, err := rt.One(c, db, scan, "SELECT x FROM t WHERE x = ?1", 2); v != 2 || err != nil {
		t.Errorf("one row: %d %v", v, err)
	}
	if _, err := rt.One(c, db, scan, "SELECT x FROM t WHERE x = ?1", 3); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("no row: %v", err)
	}
	if _, err := rt.One(c, db, scan, "SELECT x FROM t"); !errors.Is(err, maago.ErrManyRows) {
		t.Errorf("two rows: %v", err)
	}
	if _, err := rt.One(c, db, scan, "SELECT nope FROM t"); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("query error: %v", err)
	}
	if vs, err := rt.Many(c, db, scan, "SELECT x FROM t ORDER BY x"); len(vs) != 2 || vs[1] != 2 || err != nil {
		t.Errorf("many: %v %v", vs, err)
	}
	if vs, err := rt.Many(c, db, scan, "SELECT x FROM t WHERE false"); vs == nil || len(vs) != 0 || err != nil {
		t.Errorf("no rows: %#v %v, want an empty slice", vs, err)
	}
	// A Scan error stops the query.
	bad := func(r *sql.Rows) (string, error) {
		var s time.Time
		return "", r.Scan(rt.ScanSQLiteTime(&s))
	}
	if _, err := rt.Many(c, db, bad, "SELECT x FROM t"); err == nil {
		t.Error("scan error: none")
	}
}

// SQLite times as D-105 stores them.
func TestSQLiteTimes(t *testing.T) {
	sqliteFile(t)
	db := openDB(t, maago.DBConfig{URL: "sqlite:app.db"})
	if _, err := db.Exec("CREATE TABLE times (id INTEGER PRIMARY KEY, at TEXT) STRICT"); err != nil {
		t.Fatal(err)
	}
	kst := time.FixedZone("KST", 9*3600)
	later := time.Date(2026, 9, 27, 12, 0, 5, 500_000_000, kst)       // 03:00:05.5 UTC
	earlier := time.Date(2026, 9, 27, 3, 0, 5, 250_000_000, time.UTC) // .25: fewer digits, earlier
	past, future := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	for i, v := range []any{rt.SQLiteTime(later), rt.SQLiteTime(earlier), rt.SQLiteTime(past), rt.SQLiteTime(future),
		nil, "2026-09-27T12:00:00+09:00", "yesterday", later} {
		if _, err := db.Exec("INSERT INTO times VALUES (?1, ?2)", i+1, v); err != nil {
			t.Fatal(err)
		}
	}
	var text string
	if err := db.QueryRow("SELECT at FROM times WHERE id = 1").Scan(&text); err != nil {
		t.Fatal(err)
	}
	if text != "2026-09-27 03:00:05.500000" {
		t.Errorf("stored %q, want UTC with six digits", text)
	}
	var ids []int
	rows, err := db.Query("SELECT id FROM times WHERE id IN (1, 2) ORDER BY at")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) != 2 || ids[0] != 2 {
		t.Errorf("ORDER BY at gave %v, want the earlier time (2) first", ids)
	}
	var before, after bool
	if err := db.QueryRow("SELECT (SELECT at FROM times WHERE id = 3) < datetime('now'), (SELECT at FROM times WHERE id = 4) < datetime('now')").Scan(&before, &after); err != nil {
		t.Fatal(err)
	}
	if !before || after {
		t.Errorf("against datetime('now'): past %v, future %v; want true, false", before, after)
	}
	scan := func(id int) (time.Time, *time.Time, error, error) {
		var v time.Time
		var p *time.Time
		err1 := db.QueryRow("SELECT at FROM times WHERE id = ?1", id).Scan(rt.ScanSQLiteTime(&v))
		err2 := db.QueryRow("SELECT at FROM times WHERE id = ?1", id).Scan(rt.ScanNullSQLiteTime(&p))
		return v, p, err1, err2
	}
	if v, _, err, _ := scan(1); err != nil || !v.Equal(later) {
		t.Errorf("read back %v %v, want %v", v, err, later)
	}
	if _, p, err1, err2 := scan(5); err1 == nil || err2 != nil || p != nil {
		t.Errorf("NULL: time.Time err %v, *time.Time %v %v", err1, p, err2)
	}
	if v, _, err, _ := scan(6); err != nil || !v.Equal(time.Date(2026, 9, 27, 3, 0, 0, 0, time.UTC)) {
		t.Errorf("RFC 3339 by hand: %v %v", v, err)
	}
	if _, _, err, _ := scan(7); err == nil {
		t.Error("bad text: no error")
	}
	// A time.Time passed as is, not through SQLiteTime, is Go's String()
	// form, which does not read back: generated code must convert.
	if _, _, err, _ := scan(8); err == nil {
		t.Error("unconverted time.Time: no error")
	}
}

// PostgreSQL keeps the instant to the microsecond, in another location:
// compare with Equal.
func TestPostgresTime(t *testing.T) {
	db := openDB(t, maago.DBConfig{URL: pgURL(t)})
	in := time.Date(2026, 9, 27, 12, 0, 5, 123_456_789, time.FixedZone("KST", 9*3600))
	var out time.Time
	if err := db.QueryRow("SELECT $1::timestamptz", in).Scan(&out); err != nil {
		t.Fatal(err)
	}
	if !out.Equal(in.Truncate(time.Microsecond)) {
		t.Errorf("got %v, want %v", out, in.Truncate(time.Microsecond))
	}
}

// A failed PostgreSQL connection does not show the password.
func TestOpenDBHidesPassword(t *testing.T) {
	_, err := maago.OpenDB(ctx(t), maago.DBConfig{URL: "postgres://maago:s3cretpw@127.0.0.1:1/x?connect_timeout=1"})
	if err == nil || strings.Contains(err.Error(), "s3cretpw") || !strings.HasPrefix(err.Error(), "maago: cannot open the database: ") {
		t.Errorf("got %v", err)
	}
}

// A file that exists but cannot be written: WAL writes when it opens.
func TestOpenDBReadOnlyFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write any file")
	}
	sqliteFile(t)
	os.Chmod("app.db", 0o444)
	_, err := maago.OpenDB(ctx(t), maago.DBConfig{URL: "sqlite:app.db"})
	if err == nil || !strings.Contains(err.Error(), "check that the file and its directory can be written") {
		t.Errorf("got %v", err)
	}
}
