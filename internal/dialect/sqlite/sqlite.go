// Package sqlite writes SQLite DDL for checked models (D-77, D-84..D-87).
package sqlite

import (
	"fmt"
	"go/token"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/model"
)

var types = map[model.Type]string{
	model.Int64: "INTEGER", model.Int32: "INTEGER", model.Bool: "INTEGER",
	model.Float64: "REAL", model.String: "TEXT", model.Time: "TEXT", model.Bytes: "BLOB",
}

// DDL returns CREATE statements for every model: each table, then its
// indexes. Tables are STRICT, so a value of the wrong type is an error
// rather than stored as is (D-87). SQLite allows a foreign key to name a
// table created later, so tables keep the order of the models.
func DDL(fset *token.FileSet, apps []*model.App) (string, []diag.Diagnostic) {
	if ds := dialect.CheckNames(fset, apps, 0); len(ds) > 0 {
		return "", ds
	}
	var stmts []string
	for _, app := range apps {
		tables := dialect.Tables(app)
		for _, m := range app.Models {
			stmts = append(stmts, table(m, tables))
			for _, cols := range m.Uniques {
				stmts = append(stmts, fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s (%s);",
					dialect.Quote(dialect.UniqueName(m.Table, cols)), dialect.Quote(m.Table), dialect.QuoteAll(cols)))
			}
			for _, cols := range m.Indexes {
				stmts = append(stmts, fmt.Sprintf("CREATE INDEX %s ON %s (%s);",
					dialect.Quote(dialect.IndexName(m.Table, cols)), dialect.Quote(m.Table), dialect.QuoteAll(cols)))
			}
		}
	}
	return strings.Join(stmts, "\n\n") + "\n", nil
}

func table(m *model.Model, tables map[string]string) string {
	var lines []string
	for _, f := range m.Fields {
		lines = append(lines, column(f))
	}
	for _, f := range m.Fields {
		if f.FK != "" {
			lines = append(lines, fmt.Sprintf("CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s) ON DELETE NO ACTION",
				dialect.Quote(dialect.ForeignKeyName(m.Table, f.Column)), dialect.Quote(f.Column), dialect.Quote(tables[f.FK]), dialect.Quote("id")))
		}
	}
	return fmt.Sprintf("CREATE TABLE %s (\n    %s\n) STRICT;", dialect.Quote(m.Table), strings.Join(lines, ",\n    "))
}

func column(f *model.Field) string {
	c := dialect.Quote(f.Column)
	if f.Name == "ID" {
		return c + " INTEGER PRIMARY KEY AUTOINCREMENT" // ids are never reused (D-87)
	}
	s := c + " " + types[f.Type]
	if !f.Pointer {
		s += " NOT NULL"
	}
	if f.Default != nil {
		s += " DEFAULT " + literal(f.Type, *f.Default)
	}
	switch {
	case f.Max > 0:
		s += fmt.Sprintf(" CHECK (length(%s) <= %d)", c, f.Max) // characters, not bytes
	case f.Type == model.Bool:
		s += fmt.Sprintf(" CHECK (%s IN (0, 1))", c)
	}
	if len(f.Choices) > 0 {
		quoted := make([]string, len(f.Choices))
		for i, v := range f.Choices {
			quoted[i] = dialect.String(v)
		}
		s += fmt.Sprintf(" CHECK (%s IN (%s))", c, strings.Join(quoted, ", "))
	}
	return s
}

func literal(t model.Type, v string) string {
	switch t {
	case model.String:
		return dialect.String(v)
	case model.Bool:
		if v == "true" {
			return "1"
		}
		return "0"
	}
	return dialect.Number(t, v)
}
