package maago

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
)

// Tx is the transaction a view that writes takes (D-150): it is a DB, so
// the generated queries take it, but it cannot commit or roll back.
// WithTx does that, for the generated adapter and for tests. On SQLite the
// transaction holds the write lock for the whole view, so slow work (a
// mail, a call to another service) goes in AfterCommit; on PostgreSQL it
// has the server's isolation level, READ COMMITTED unless set otherwise.
// A Tx is for one goroutine.
type Tx struct {
	tx *sql.Tx
	st *txState
}

// txState is what WithTx keeps of its transaction: the work to do after
// the commit, and whether a query failed.
type txState struct {
	after  []func(context.Context)
	failed bool
}

type txKey struct{}

func (t Tx) sqlTx() *sql.Tx {
	if t.tx == nil {
		panic("maago: a Tx that maago.WithTx did not make; a view takes it as a parameter, and a test uses maago.WithTx")
	}
	return t.tx
}

// ExecContext runs query in the transaction.
func (t Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	r, err := t.sqlTx().ExecContext(ctx, query, args...)
	t.st.failed = t.st.failed || err != nil
	return r, err
}

// QueryContext runs query in the transaction.
func (t Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	rows, err := t.sqlTx().QueryContext(ctx, query, args...)
	t.st.failed = t.st.failed || err != nil
	return rows, err
}

// QueryRowContext runs query in the transaction.
func (t Tx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	row := t.sqlTx().QueryRowContext(ctx, query, args...)
	t.st.failed = t.st.failed || row.Err() != nil
	return row
}

// WithTx runs f in a transaction of db (the database a view with maago.NoTx
// takes, or Config.DB) and commits it when f returns nil
// (D-150). Any error f returns, a 422 included, rolls it back and comes
// back as it is; a panic rolls it back and goes on. After the commit,
// the work AfterCommit gave runs, in order. A view that takes Tx runs in
// one; a test that calls such a view makes one with WithTx. WithTx inside
// another panics: a request's second transaction waits on its first. When
// ctx ends (the client went away), database/sql rolls the transaction back.
func WithTx(ctx context.Context, db DB, f func(ctx context.Context, tx Tx) error) error {
	if _, in := db.(Tx); in || ctx.Value(txKey{}) != nil {
		panic("maago: WithTx inside a transaction; use the Tx you have")
	}
	b, ok := db.(interface {
		BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	})
	if !ok {
		panic(fmt.Sprintf("maago: WithTx of a %T, which begins no transaction; pass the db a view with maago.NoTx takes", db))
	}
	tx, err := b.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	st := &txState{}
	done := false
	defer func() {
		if !done { // f panicked: the panic goes on, rolled back
			tx.Rollback()
		}
	}()
	err = f(context.WithValue(ctx, txKey{}, st), Tx{tx, st})
	done = true
	if err != nil {
		tx.Rollback() // its error says less than err
		return err
	}
	if err := tx.Commit(); err != nil {
		if st.failed {
			return fmt.Errorf("maago: commit: %w; a query failed earlier in this transaction, and PostgreSQL rolled it back: check with a query before writing (a title taken, say), or return that query's error from the view", err)
		}
		return err
	}
	runAfter(ctx, st.after)
	return nil
}

// AfterCommit gives f to the transaction of ctx, to run after it commits,
// before the response is written: a mail that tells of a change that might
// still roll back must wait for the commit (D-150). If the transaction
// rolls back, f does not run. Outside a transaction f runs at once. Its
// context is not canceled when the request ends, and its panic is logged.
func AfterCommit(ctx context.Context, f func(ctx context.Context)) {
	if st, ok := ctx.Value(txKey{}).(*txState); ok {
		st.after = append(st.after, f)
		return
	}
	runAfter(ctx, []func(context.Context){f})
}

func runAfter(ctx context.Context, fs []func(context.Context)) {
	ctx = context.WithValue(context.WithoutCancel(ctx), txKey{}, nil) // committed: no transaction now
	for _, f := range fs {
		func() {
			defer func() {
				if p := recover(); p != nil {
					log.Printf("maago: AfterCommit: panic: %v\n%s", p, debug.Stack())
				}
			}()
			f(ctx)
		}()
	}
}

// Buffer holds a response until the transaction commits, so that a page
// that fails to render rolls it back. Only generated code uses it.
type Buffer struct {
	header http.Header
	body   bytes.Buffer
}

// NewBuffer is an empty Buffer.
func NewBuffer() *Buffer { return &Buffer{header: http.Header{}} }

// Header is the header of the response to come.
func (b *Buffer) Header() http.Header { return b.header }

// Write adds p to the body.
func (b *Buffer) Write(p []byte) (int, error) { return b.body.Write(p) }

// WriteHeader is not for a Buffer: the response is a 200.
func (b *Buffer) WriteHeader(int) { panic("maago: WriteHeader on a Buffer") }

// Send writes the response to w.
func (b *Buffer) Send(w http.ResponseWriter) error {
	for k, v := range b.header {
		w.Header()[k] = v
	}
	_, err := w.Write(b.body.Bytes())
	return err
}
