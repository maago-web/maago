package rt

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/maago-web/maago"
)

// One runs a :one query and reads its one row: sql.ErrNoRows if there is
// none, ErrManyRows if there are more.
func One[T any](ctx context.Context, db maago.DB, scan func(*sql.Rows) (T, error), query string, args ...any) (T, error) {
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
		return zero, maago.ErrManyRows
	}
	if err := rows.Err(); err != nil {
		return zero, err
	}
	return v, rows.Close()
}

// Many runs a :many query and reads every row. No row is an empty
// slice, not nil, so that JSON shows [].
func Many[T any](ctx context.Context, db maago.DB, scan func(*sql.Rows) (T, error), query string, args ...any) ([]T, error) {
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
func Exec(ctx context.Context, db maago.DB, query string, args ...any) (int64, error) {
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
