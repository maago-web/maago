// Package dialect holds what the SQL dialects share: constraint names
// (D-86), identifier quoting, literals, and splitting SQL into statements.
package dialect

import (
	"fmt"
	"go/token"
	"strconv"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/model"
)

// Quote quotes an identifier, so that reserved words such as "order" work.
func Quote(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// QuoteAll quotes each name and joins them with ", ".
func QuoteAll(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = Quote(n)
	}
	return strings.Join(q, ", ")
}

// String is a SQL string literal.
func String(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// Number is the canonical form of a checked int64, int32 or float64
// default, so that 007 and 7 are written alike.
func Number(t model.Type, v string) string {
	switch t {
	case model.Int64, model.Int32:
		n, _ := strconv.ParseInt(v, 10, 64)
		return strconv.FormatInt(n, 10)
	case model.Float64:
		f, _ := strconv.ParseFloat(v, 64)
		if f == 0 {
			f = 0 // -0 would lose its sign in PostgreSQL anyway
		}
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
	panic("dialect.Number: not a number type: " + t.String())
}

// Tables maps each model of app to its table, for foreign keys.
func Tables(app *model.App) map[string]string {
	t := map[string]string{}
	for _, m := range app.Models {
		t[m.Name] = m.Table
	}
	return t
}

func UniqueName(table string, cols []string) string {
	return table + "_" + strings.Join(cols, "_") + "_key"
}

func IndexName(table string, cols []string) string {
	return table + "_" + strings.Join(cols, "_") + "_idx"
}

func ForeignKeyName(table, col string) string {
	return table + "_" + col + "_fkey"
}

// CheckNames reports constraint names that two constraints share, since a
// database keeps one namespace for them, and, when maxLen > 0, names
// longer than maxLen bytes.
func CheckNames(fset *token.FileSet, apps []*model.App, maxLen int) []diag.Diagnostic {
	var ds []diag.Diagnostic
	owner := map[string]string{}
	for _, app := range apps {
		for _, m := range app.Models {
			var names []string
			for _, cols := range m.Uniques {
				names = append(names, UniqueName(m.Table, cols))
			}
			for _, cols := range m.Indexes {
				names = append(names, IndexName(m.Table, cols))
			}
			for _, f := range m.Fields {
				if f.FK != "" {
					names = append(names, ForeignKeyName(m.Table, f.Column))
				}
			}
			for _, n := range names {
				pos := fset.Position(m.Pos)
				if maxLen > 0 && len(n) > maxLen {
					ds = append(ds, diag.New(pos, fmt.Sprintf("constraint name %q of %s is longer than %d bytes", n, m.Name, maxLen),
						"shorten the table name with table=…"))
				}
				if prev, ok := owner[n]; ok {
					ds = append(ds, diag.New(pos, fmt.Sprintf("constraint name %q of %s is also used by %s", n, m.Name, prev),
						"rename a table with table=… or a field, so the names differ"))
					continue
				}
				owner[n] = m.Name
			}
		}
	}
	return ds
}
