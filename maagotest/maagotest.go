// Package maagotest gives an app's tests a database of their own (D-113)
// and a mailbox (D-126): an exception to the runtime being the root
// package alone, since it imports testing, which the app's binary must not
// carry.
package maagotest

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/migrate"
	"github.com/maago-web/maago/internal/practicedb"
	"github.com/maago-web/maago/internal/project"
)

// DB returns a new database for t with the project's migrations applied,
// and removes it when t ends. The project is the module the working
// directory is in, which go test makes the package's directory; its
// address is MAAGO_DATABASE_URL, from the environment or the root's .env,
// as for maago itself. Only the dialect and the server of the address
// are used: SQLite gets a new file in t.TempDir(), PostgreSQL a schema of
// its own on that server, named after the package and t.Name(), which
// every connection of the pool searches. Each call is a database apart.
// The test imports the driver: _ "modernc.org/sqlite" or
// _ "github.com/jackc/pgx/v5/stdlib". What goes wrong is t.Fatal, so call
// DB from the test's own goroutine, as the testing package asks of Fatal.
func DB(t testing.TB) *sql.DB {
	t.Helper()
	fatal := func(ds []diag.Diagnostic) {
		t.Helper()
		var b strings.Builder
		diag.Write(&b, ds)
		t.Fatal("maagotest: " + strings.TrimSpace(b.String()))
	}
	wd, err := os.Getwd()
	if err == nil {
		wd, err = filepath.EvalSymlinks(wd) // as project.Root resolves the root: the package is below it
	}
	if err != nil {
		t.Fatal("maagotest: " + err.Error())
	}
	root, ds := project.Root(wd)
	if ds != nil {
		fatal(ds)
	}
	d, addr, ds := project.Database(root, os.LookupEnv)
	if ds != nil {
		fatal(ds)
	}
	files, ds := migrate.ReadFiles(root)
	if ds != nil {
		fatal(ds)
	}
	driver := map[project.Dialect][2]string{project.SQLite: {"sqlite", "modernc.org/sqlite"}, project.Postgres: {"pgx", "github.com/jackc/pgx/v5/stdlib"}}[d]
	if !slices.Contains(sql.Drivers(), driver[0]) {
		t.Fatalf("maagotest: no %q database driver\n  fix: import _ %q in a _test.go file of this package; tests do not go through main (D-90)", driver[0], driver[1])
	}
	if d == project.SQLite {
		addr = "sqlite:" + filepath.Join(t.TempDir(), "test.db") // the migrations make the file
	} else {
		addr = schema(t, root, wd, addr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// The migrations run on a connection of their own, closed before the
	// pool opens: two settings on one file (for SQLite) or one schema
	// never meet.
	mdb, ds := practicedb.OpenDatabase(ctx, d, addr, root)
	if ds != nil {
		fatal(ds)
	}
	_, ds = migrate.Run(ctx, mdb, d, files)
	mdb.Close()
	if ds != nil {
		fatal(ds)
	}
	db, err := maago.OpenDB(ctx, maago.DBConfig{URL: addr})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() }) // before the schema is dropped: cleanups run last first
	return db
}

var (
	callsMu sync.Mutex
	calls   = map[testing.TB]int{}
)

// schema makes t's PostgreSQL schema anew and returns addr searching it.
// The name hashes the package (the module path and the directory below
// the root, the same on every run and machine), t.Name() and which call
// of DB in t this is, so a schema a killed run left is dropped by the next.
func schema(t testing.TB, root, wd, addr string) string {
	t.Helper()
	mod, ds := project.ModulePath(root)
	if ds != nil {
		var b strings.Builder
		diag.Write(&b, ds)
		t.Fatal("maagotest: " + strings.TrimSpace(b.String()))
	}
	rel, _ := filepath.Rel(root, wd)
	callsMu.Lock()
	calls[t]++
	n := calls[t]
	callsMu.Unlock()
	t.Cleanup(func() { callsMu.Lock(); delete(calls, t); callsMu.Unlock() })
	name := SchemaName(path.Join(mod, filepath.ToSlash(rel)), t.Name(), n)

	admin := func(stmts ...string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		db, err := maago.OpenDB(ctx, maago.DBConfig{URL: addr, MaxOpenConns: 1})
		if err != nil {
			return err
		}
		defer db.Close()
		for _, s := range stmts {
			if _, err := db.ExecContext(ctx, s); err != nil {
				return err
			}
		}
		return nil
	}
	if err := admin("DROP SCHEMA IF EXISTS "+name+" CASCADE", "CREATE SCHEMA "+name); err != nil {
		t.Fatal("maagotest: cannot make schema " + name + ": " + project.HidePassword(err.Error(), addr))
	}
	t.Cleanup(func() {
		// One connection: the lock timeout holds for the drop.
		if err := admin("SET lock_timeout = '5s'", "DROP SCHEMA IF EXISTS "+name+" CASCADE"); err != nil {
			t.Errorf("maagotest: cannot drop schema %s; did the test leave a transaction or rows open? %s", name, project.HidePassword(err.Error(), addr))
		}
	})
	u, err := url.Parse(addr)
	if err != nil {
		t.Fatal("maagotest: the PostgreSQL address does not parse")
	}
	q := u.Query()
	q.Set("search_path", name) // pgx sends it as each connection starts
	u.RawQuery = q.Encode()
	return u.String()
}

// SchemaName is the schema DB makes for the n-th call in test name of
// package pkg, an import path: maagotest_ and 16 hex digits.
func SchemaName(pkg, name string, n int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", pkg, name, n)))
	return "maagotest_" + hex.EncodeToString(h[:8])
}
