package dialect

import (
	"context"
	"database/sql"
	"fmt"
	"go/token"

	"github.com/maago-web/maago/internal/diag"
)

// Querier runs a query; a practice database is one.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Reader is what the dialects' schema readers share, so that their
// diagnostics read alike.
type Reader struct {
	Ctx   context.Context
	Q     Querier
	Diags []diag.Diagnostic
}

func (r *Reader) Fail(msg, fix string) {
	r.Diags = append(r.Diags, diag.New(token.Position{}, msg, fix))
}

// Unsupported reports something table has that a schema cannot hold,
// and so a comparison would not see (PHILOSOPHY 7).
func (r *Reader) Unsupported(table, what string) {
	r.Fail(fmt.Sprintf("table %q has %s, which maago does not support", table, what), "remove it from the migrations")
}

// Rows calls f with the values of each row. f must not query: the
// practice database has one connection, busy until the rows are read.
func (r *Reader) Rows(query string, f func([]any), args ...any) error {
	rows, err := r.Q.QueryContext(r.Ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	for rows.Next() {
		v := make([]any, len(cols))
		p := make([]any, len(cols))
		for i := range v {
			p[i] = &v[i]
		}
		if err := rows.Scan(p...); err != nil {
			return err
		}
		f(v)
	}
	return rows.Err()
}

// Bug reports a failed query, which is maago's fault.
func Bug(err error) []diag.Diagnostic {
	return []diag.Diagnostic{diag.New(token.Position{}, "cannot read the practice database: "+diag.OneLine(err.Error()), "report this as a maago bug")}
}

// Str is a text value of a row, or "" for NULL.
func Str(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	}
	return ""
}

// Num is an integer value of a row, or 0 for NULL.
func Num(v any) int64 {
	switch v := v.(type) {
	case int64:
		return v
	case int32:
		return int64(v)
	case int16:
		return int64(v)
	case uint32:
		return int64(v)
	}
	return 0
}

// NormalizeSQL is Normalize for SQL text of one expression, such as a
// default; text that does not split into one statement is kept as is.
func NormalizeSQL(sql string) string {
	stmts, _ := Split("", sql)
	if len(stmts) != 1 {
		return sql
	}
	return Normalize(stmts[0].Tokens)
}
