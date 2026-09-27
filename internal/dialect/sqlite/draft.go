package sqlite

import (
	"fmt"
	"slices"
	"strings"

	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/migrate"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/schema"
)

// Draft returns the SQL that turns the migrations into the models, for
// the changes cs that migrate.Diff found. What ALTER TABLE can do is done
// with it; any other change rebuilds its table from the model, the way the
// SQLite documentation does (D-93). What the models have is written from
// the models, as DDL writes it.
func Draft(apps []*model.App, cs []migrate.Change) string {
	rebuild := map[string]bool{} // table name → written yet
	for _, c := range cs {
		if c.From != nil && c.To != nil && !direct(c, cs) {
			rebuild[c.Table()] = false
		}
	}
	var stmts []string
	for _, c := range cs {
		written, ok := rebuild[c.Table()]
		switch {
		case !ok:
			stmts = append(stmts, statements(apps, c)...)
		case !written: // at the first change of the table
			stmts = append(stmts, rebuilt(apps, c.From, c.To)...)
			rebuild[c.Table()] = true
		}
	}
	if len(stmts) == 0 {
		return ""
	}
	return strings.Join(stmts, "\n\n") + "\n"
}

// direct reports whether c, a change of a table on both sides, can be
// written without rebuilding the table.
func direct(c migrate.Change, cs []migrate.Change) bool {
	switch c.Kind {
	case migrate.AddIndex, migrate.DropIndex:
		return !c.Index.Constraint
	case migrate.AddColumn:
		return addable(c.To, c.New)
	case migrate.DropColumn:
		return droppable(c.From, c.Old.Name)
	case migrate.AddCheck:
		// Written with its column, if that is added directly.
		return slices.ContainsFunc(cs, func(o migrate.Change) bool {
			return o.Kind == migrate.AddColumn && o.To == c.To && o.New.Name == c.Check.Column && addable(o.To, o.New)
		})
	case migrate.DropCheck:
		// Dropped with its column, if that is dropped directly.
		return slices.ContainsFunc(cs, func(o migrate.Change) bool {
			return o.Kind == migrate.DropColumn && o.From == c.From && o.Old.Name == c.Check.Column && droppable(o.From, o.Old.Name)
		})
	}
	return false
}

// addable reports whether ADD COLUMN can add col: SQLite refuses a NOT
// NULL column without a default even to an empty table, and a foreign key
// column must default to NULL, which is left to a rebuild.
func addable(t *schema.Table, col *schema.Column) bool {
	return (!col.NotNull || col.Default != "") && !slices.Contains(t.PK, col.Name) && !inFK(t, col.Name)
}

// droppable reports whether DROP COLUMN can drop column: SQLite refuses
// one in a key, a foreign key, a UNIQUE constraint or a CHECK of anything
// but the column itself. Plain indexes on it are dropped before it.
func droppable(t *schema.Table, column string) bool {
	if slices.Contains(t.PK, column) || inFK(t, column) {
		return false
	}
	for _, ix := range t.Indexes {
		if ix.Constraint && slices.Contains(ix.Columns, column) {
			return false
		}
	}
	for _, ck := range t.Checks {
		if ck.Column != column && mentions(ck.Expr, column) {
			return false
		}
	}
	return true
}

func inFK(t *schema.Table, column string) bool {
	return slices.ContainsFunc(t.FKs, func(k schema.FK) bool { return slices.Contains(k.Columns, column) })
}

// mentions reports whether the Normalize'd expr names column, quoted or
// not; SQLite names ignore case.
func mentions(expr, column string) bool {
	stmts, _ := dialect.Split("", expr)
	for _, st := range stmts {
		for _, t := range st.Tokens {
			switch t.Kind {
			case dialect.Word:
				if strings.EqualFold(t.Text, column) {
					return true
				}
			case dialect.Quoted:
				if strings.EqualFold(t.Text[1:len(t.Text)-1], column) {
					return true
				}
			}
		}
	}
	return false
}

// statements writes a change that needs no rebuild.
func statements(apps []*model.App, c migrate.Change) []string {
	switch c.Kind {
	case migrate.CreateTable:
		m, tables := dialect.Model(apps, c.To.Name)
		return append([]string{table(m, m.Table, tables)}, indexes(m)...)
	case migrate.DropTable:
		return []string{dialect.DropTable(c.From.Name)}
	case migrate.AddColumn:
		return []string{fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", dialect.Quote(c.To.Name), column(dialect.Field(apps, c.To.Name, c.New.Name)))}
	case migrate.DropColumn:
		return []string{dialect.DropColumn(c.From.Name, c.Old.Name)}
	case migrate.AddIndex:
		return []string{dialect.CreateIndex(c.To.Name, c.Index)}
	case migrate.DropIndex:
		return []string{fmt.Sprintf("DROP INDEX %s;", dialect.Quote(c.Index.Name))}
	case migrate.AddCheck, migrate.DropCheck:
		return nil // written with their column
	}
	panic(fmt.Sprintf("sqlite.Draft: change kind %d needs a rebuild", c.Kind))
}

// rebuilt rebuilds table from into the model's table: a new table under
// another name, the values of the columns both have, the AUTOINCREMENT
// counter so that no dropped id comes back (D-87), then the swap and the
// indexes.
func rebuilt(apps []*model.App, from, to *schema.Table) []string {
	m, tables := dialect.Model(apps, to.Name)
	t, tmp := dialect.Quote(to.Name), "maago_new_"+to.Name
	var kept, lost []string
	var warn string
	for _, f := range m.Fields {
		if slices.ContainsFunc(from.Columns, func(c schema.Column) bool { return c.Name == f.Column }) {
			kept = append(kept, f.Column)
		} else if !f.Pointer && f.Default == nil {
			warn += dialect.NoDefault(to.Name, f.Column)
		}
	}
	for _, c := range from.Columns {
		if !slices.ContainsFunc(m.Fields, func(f *model.Field) bool { return f.Column == c.Name }) {
			lost = append(lost, c.Name)
		}
	}
	drop := fmt.Sprintf("DROP TABLE %s;", t)
	if len(lost) > 0 {
		drop = dialect.Dropping(fmt.Sprintf("the values of %s in %q, which the copy above leaves out", dialect.QuoteAll(lost), to.Name)) + drop
	}
	stmts := []string{table(m, tmp, tables)}
	if len(kept) > 0 {
		stmts = append(stmts, warn+fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s;", dialect.Quote(tmp), dialect.QuoteAll(kept), dialect.QuoteAll(kept), t))
	}
	stmts = append(stmts,
		fmt.Sprintf("DELETE FROM sqlite_sequence WHERE name = %s;", dialect.String(tmp)),
		fmt.Sprintf("INSERT INTO sqlite_sequence (name, seq) SELECT %s, seq FROM sqlite_sequence WHERE name = %s;", dialect.String(tmp), dialect.String(to.Name)),
		drop,
		fmt.Sprintf("ALTER TABLE %s RENAME TO %s;", dialect.Quote(tmp), t),
	)
	return append(stmts, indexes(m)...)
}
