package maago

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
)

// ErrManyRows is what a :one query returns when it finds more than one
// row: it does not pick the first silently.
var ErrManyRows = errors.New("maago: :one query returned more than one row; make it :many, or add LIMIT 1")

// ErrConflict is what a generated Update gives when the row's version
// (its auto=update field) is not the one the model holds: another
// request changed it since it was read (D-150). It answers 409.
var ErrConflict = errors.New("maago: the row changed since it was read; read it again and apply the change, or change one column with a query that sets the version too")

// ErrEmptySlice is what a generated query returns for an empty slice
// parameter where the SQL needs one element or more: IN () is not SQL.
// The generated code wraps it with the query's and the parameter's names.
var ErrEmptySlice = errors.New("maago: empty slice, which IN (…) cannot take; check it before calling the query, or put its line under {name}")

// One runs a :one query and reads its one row: sql.ErrNoRows if there is
// none, ErrManyRows if there are more.
func One[T any](ctx context.Context, db DB, scan func(*sql.Rows) (T, error), query string, args ...any) (T, error) {
	var zero T
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return zero, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return zero, err
		}
		return zero, sql.ErrNoRows
	}
	v, err := scan(rows)
	if err != nil {
		return zero, err
	}
	if rows.Next() {
		return zero, ErrManyRows
	}
	if err := rows.Err(); err != nil {
		return zero, err
	}
	return v, rows.Close()
}

// Many runs a :many query and reads every row. No row is an empty
// slice, not nil, so that JSON shows [].
func Many[T any](ctx context.Context, db DB, scan func(*sql.Rows) (T, error), query string, args ...any) ([]T, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	vs := []T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		vs = append(vs, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return vs, rows.Close()
}

// Exec runs a statement and returns how many rows it touched: an :exec
// query, and the CRUD's Update and Delete, which make 0 sql.ErrNoRows.
func Exec(ctx context.Context, db DB, query string, args ...any) (int64, error) {
	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Placeholders returns n placeholders numbered from first, like "$3, $4"
// for mark "$", for the elements of a slice parameter. n is at least 1:
// the generated code returns ErrEmptySlice before.
func Placeholders(mark string, first, n int) string {
	if n < 1 {
		panic("maago: a slice needs at least one element")
	}
	var b strings.Builder
	for i := range n {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(mark + strconv.Itoa(first+i))
	}
	return b.String()
}
