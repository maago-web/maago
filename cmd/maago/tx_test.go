package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/rt"
)

// txDB is a database of each dialect with an empty table, and the
// table's name: PostgreSQL's is the test's own, dropped after.
func txDB(t *testing.T, dialect string) (*sql.DB, string) {
	t.Helper()
	if dialect == "sqlite" {
		sqliteFile(t)
		return openDB(t, maago.DBConfig{URL: "sqlite:app.db"}), "t"
	}
	db := openDB(t, maago.DBConfig{URL: pgURL(t)})
	table := fmt.Sprintf("tx_%d", time.Now().UnixNano())
	if _, err := db.Exec("CREATE TABLE " + table + " (x INTEGER UNIQUE)"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DROP TABLE " + table) })
	return db, table
}

func count(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// WithTx commits what f wrote when f returns nil, and nothing otherwise;
// AfterCommit's work runs only after a commit (D-150).
func TestWithTx(t *testing.T) {
	for _, d := range []string{"sqlite", "postgres"} {
		t.Run(d, func(t *testing.T) {
			db, table := txDB(t, d)
			insert := "INSERT INTO " + table + " (x) VALUES (1)"
			var ran []string
			write := func(ret error) func(context.Context, maago.Tx) error {
				return func(ctx context.Context, tx maago.Tx) error {
					if _, err := tx.ExecContext(ctx, insert); err != nil {
						return err
					}
					maago.AfterCommit(ctx, func(context.Context) { ran = append(ran, "first") })
					maago.AfterCommit(ctx, func(context.Context) { panic("logged, and no more") })
					maago.AfterCommit(ctx, func(context.Context) { ran = append(ran, "second") })
					return ret
				}
			}
			bad := errors.New("bad form")
			if err := maago.WithTx(ctx(t), db, write(bad)); err != bad || count(t, db, table) != 0 || ran != nil {
				t.Errorf("an error: %v, %d rows, ran %v", err, count(t, db, table), ran)
			}
			func() {
				defer func() {
					if p := recover(); p != "boom" {
						t.Errorf("the panic: %v", p)
					}
				}()
				maago.WithTx(ctx(t), db, func(ctx context.Context, tx maago.Tx) error {
					write(nil)(ctx, tx)
					panic("boom")
				})
			}()
			if n := count(t, db, table); n != 0 || ran != nil {
				t.Errorf("a panic: %d rows, ran %v", n, ran)
			}
			if err := maago.WithTx(ctx(t), db, write(nil)); err != nil || count(t, db, table) != 1 || strings.Join(ran, " ") != "first second" {
				t.Errorf("a commit: %v, %d rows, ran %v", err, count(t, db, table), ran)
			}
			// A query that failed and was not returned: PostgreSQL rolled
			// the transaction back, and the commit says why.
			err := maago.WithTx(ctx(t), db, func(ctx context.Context, tx maago.Tx) error {
				tx.ExecContext(ctx, insert) // a unique violation, its error dropped
				return nil
			})
			if d == "postgres" && (err == nil || !strings.Contains(err.Error(), "a query failed earlier in this transaction")) {
				t.Errorf("a dropped query error: %v", err)
			}
			// The same through QueryRowContext, as a generated Insert runs.
			err = maago.WithTx(ctx(t), db, func(ctx context.Context, tx maago.Tx) error {
				var x int
				tx.QueryRowContext(ctx, insert+" RETURNING x").Scan(&x) // its error dropped
				return nil
			})
			if d == "postgres" && (err == nil || !strings.Contains(err.Error(), "a query failed earlier in this transaction")) {
				t.Errorf("a dropped QueryRow error: %v", err)
			}
			// Outside a transaction AfterCommit runs at once; inside one,
			// WithTx panics.
			ran = nil
			maago.AfterCommit(ctx(t), func(context.Context) { ran = append(ran, "now") })
			if strings.Join(ran, " ") != "now" {
				t.Errorf("AfterCommit outside: ran %v", ran)
			}
			func() {
				defer func() {
					if p := recover(); p == nil || !strings.Contains(fmt.Sprint(p), "WithTx inside a transaction") {
						t.Errorf("nested: %v", p)
					}
				}()
				maago.WithTx(ctx(t), db, func(ctx context.Context, tx maago.Tx) error {
					return maago.WithTx(ctx, db, write(nil))
				})
			}()
			// WithTx of a Tx, or of a DB that begins nothing, says so.
			for what, of := range map[string]maago.DB{"inside a transaction": maago.Tx{}, "begins no transaction": noBegin{db}} {
				func() {
					defer func() {
						if p := recover(); p == nil || !strings.Contains(fmt.Sprint(p), what) {
							t.Errorf("WithTx of %T: %v", of, p)
						}
					}()
					maago.WithTx(ctx(t), of, write(nil))
				}()
			}
			// A Tx not from WithTx says so.
			func() {
				defer func() {
					if p := recover(); p == nil || !strings.Contains(fmt.Sprint(p), "maago.WithTx") {
						t.Errorf("a hand-made Tx: %v", p)
					}
				}()
				maago.Tx{}.ExecContext(ctx(t), insert)
			}()
		})
	}
}

// noBegin is a DB without BeginTx.
type noBegin struct{ maago.DB }

// Two transactions that read, then write, both commit on SQLite: they
// take the write lock when they begin, so the second waits (D-150).
func TestWithTxSQLiteConcurrent(t *testing.T) {
	db, table := txDB(t, "sqlite")
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Go(func() {
			errs[i] = maago.WithTx(ctx(t), db, func(ctx context.Context, tx maago.Tx) error {
				var n int
				if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
					return err
				}
				time.Sleep(50 * time.Millisecond) // both read before either writes
				_, err := tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (x) VALUES (%d)", table, 10+n))
				return err
			})
		})
	}
	wg.Wait()
	if errors.Join(errs...) != nil || count(t, db, table) != 2 {
		t.Errorf("%v, %d rows", errs, count(t, db, table))
	}
}

// A write a unique constraint refuses, in a transaction that waited for
// another to commit the same value, leaves its transaction to read, write
// and commit: rt.Write's savepoint, without which PostgreSQL refuses every
// later statement (D-154). B most often waits, on SQLite to begin and on
// PostgreSQL at its insert; it gets the same answer if A was first.
func TestWriteRace(t *testing.T) {
	for _, d := range []string{"sqlite", "postgres"} {
		t.Run(d, func(t *testing.T) {
			db, table := txDB(t, d)
			if d == "sqlite" {
				if _, err := db.Exec("CREATE UNIQUE INDEX t_x_key ON t (x)"); err != nil {
					t.Fatal(err)
				}
			}
			keys := []rt.UniqueKey{{Fields: []string{"X"}, Key: table + "_x_key"}}
			insert := func(ctx context.Context, db maago.DB, x int) error {
				return rt.Write(ctx, db, keys, func() error {
					_, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (x) VALUES (%d)", table, x))
					return err
				})
			}
			a, err := db.BeginTx(ctx(t), nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := a.Exec(fmt.Sprintf("INSERT INTO %s (x) VALUES (1)", table)); err != nil {
				t.Fatal(err)
			}
			b := make(chan error)
			go func() {
				b <- maago.WithTx(ctx(t), db, func(ctx context.Context, tx maago.Tx) error {
					var u *maago.UniqueError
					if err := insert(ctx, tx, 1); !errors.As(err, &u) || len(u.Fields) != 1 || u.Fields[0] != "X" {
						return fmt.Errorf("the taken value: %v", err)
					}
					var n int
					if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil || n != 1 {
						return fmt.Errorf("read after: %d %v", n, err)
					}
					return insert(ctx, tx, 2)
				})
			}()
			time.Sleep(50 * time.Millisecond) // B starts, and waits for A
			if err := a.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := <-b; err != nil || count(t, db, table) != 2 {
				t.Errorf("%v, %d rows", err, count(t, db, table))
			}
		})
	}
}

// A refused write whose savepoint cannot be rolled back to, the request
// gone, is no UniqueError: a form must not take it and draw a page of a
// transaction that is lost.
func TestWriteSavepointLost(t *testing.T) {
	for _, d := range []string{"sqlite", "postgres"} {
		t.Run(d, func(t *testing.T) {
			db, table := txDB(t, d)
			if d == "sqlite" {
				if _, err := db.Exec("CREATE UNIQUE INDEX t_x_key ON t (x)"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec(fmt.Sprintf("INSERT INTO %s (x) VALUES (1)", table)); err != nil {
				t.Fatal(err)
			}
			err := maago.WithTx(ctx(t), db, func(c context.Context, tx maago.Tx) error {
				c, cancel := context.WithCancel(c)
				err := rt.Write(c, tx, nil, func() error {
					_, err := tx.ExecContext(c, fmt.Sprintf("INSERT INTO %s (x) VALUES (1)", table))
					cancel() // the client went away
					return err
				})
				var u *maago.UniqueError
				if err == nil || errors.As(err, &u) || !strings.Contains(err.Error(), "ending the savepoint of a failed write") {
					t.Errorf("%v", err)
				}
				return err
			})
			if err == nil {
				t.Error("committed")
			}
		})
	}
}
