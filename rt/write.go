package rt

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/internal/rtcore"
)

// UniqueKey is a unique constraint of a model: its fields, and Key, how
// the database names it in its error: PostgreSQL by the constraint's name,
// SQLite by its columns, "table.a, table.b".
type UniqueKey struct {
	Fields []string
	Key    string
}

// Write runs write, a generated Insert's or Update's statement, and makes
// a unique constraint's refusal a *maago.UniqueError, of the one of keys
// the database names. In a maago.Tx, write runs in a savepoint it rolls
// back to when write fails, so the transaction goes on: PostgreSQL would
// refuse every later statement. Only generated code calls it.
func Write(ctx context.Context, db maago.DB, keys []UniqueKey, write func() error) error {
	tx, inTx := db.(maago.Tx)
	if inTx {
		if _, err := tx.ExecContext(ctx, "SAVEPOINT maago_write"); err != nil {
			return err
		}
	}
	err := write()
	if inTx {
		var e error
		if err != nil {
			_, e = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT maago_write")
		}
		if e == nil {
			_, e = tx.ExecContext(ctx, "RELEASE SAVEPOINT maago_write")
		}
		if e != nil {
			if err != nil { // err as %v: no form may take it, as the transaction is lost
				return fmt.Errorf("maago: ending the savepoint of a failed write: %w; the write: %v", e, err)
			}
			return e
		}
	}
	if err == nil || !isUnique(err) {
		return err
	}
	return &maago.UniqueError{Fields: uniqueFields(err, keys), Err: err}
}

// isUnique reports whether err is a unique constraint's: PostgreSQL's
// SQLSTATE 23505, SQLite's SQLITE_CONSTRAINT_UNIQUE. The drivers' errors
// are seen through these methods, so maago imports neither.
func isUnique(err error) bool {
	var pg interface{ SQLState() string }
	var lite interface{ Code() int }
	return errors.As(err, &pg) && pg.SQLState() == "23505" ||
		errors.As(err, &lite) && lite.Code() == 2067
}

// uniqueFields are the fields of the key err names: the only key there is,
// or the one the database's message names; nil when none fits.
func uniqueFields(err error, keys []UniqueKey) []string {
	if len(keys) == 1 {
		return keys[0].Fields
	}
	msg := err.Error()
	_, cols, lite := strings.Cut(msg, "UNIQUE constraint failed: ")
	cols, _, _ = strings.Cut(cols, " (") // modernc's " (2067)"
	for _, k := range keys {
		if lite && cols == k.Key || !lite && strings.Contains(msg, `"`+k.Key+`"`) {
			return k.Fields
		}
	}
	return nil
}

// Fail is a form's Fail: it shows err on f when err is a
// *maago.UniqueError, on the one field of fields, which maps each field's
// Go name to its Error, that the constraint is over, or else as an error
// of the form; it reports whether it did. Only generated code calls it.
func Fail(f *maago.Form, err error, fields map[string]*string) bool {
	var u *maago.UniqueError
	if !errors.As(err, &u) {
		return false
	}
	s := cmp.Or(rtcore.FormStateOf(f).Settings, rtcore.Default) // nil: a form no request bound
	var in []*string
	for _, name := range u.Fields {
		if e, ok := fields[name]; ok {
			in = append(in, e)
		}
	}
	if len(in) == 1 {
		*in[0] = s.Msgs.Taken
	} else {
		f.AddError(s.Msgs.Taken)
	}
	return true
}
