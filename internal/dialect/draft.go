package dialect

import (
	"fmt"

	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/schema"
)

// Model returns the model of table. A draft asks only for tables the
// models make, so a missing one is a bug.
func Model(apps []*model.App, table string) *model.Model {
	for _, app := range apps {
		for _, m := range app.Models {
			if m.Table == table {
				return m
			}
		}
	}
	panic("dialect.Model: no model has table " + table)
}

// Field returns the field of table's model with column; like the model,
// it must exist.
func Field(apps []*model.App, table, column string) *model.Field {
	m := Model(apps, table)
	for _, f := range m.Fields {
		if f.Column == column {
			return f
		}
	}
	panic("dialect.Field: " + m.Table + " has no column " + column)
}

// Dropping is the warning before a draft's DROP. The draft never writes
// the marker itself, or the marker would stop nothing (D-37).
func Dropping(what string) string {
	return "-- ⚠ This drops " + what + `. If that is intended, put "-- maago: allow-drop" on the line right before the DROP statement.` + "\n"
}

// DropTable and DropColumn are the same in both dialects, warning and all.
func DropTable(table string) string {
	return Dropping(fmt.Sprintf("table %q and all its rows", table)) + fmt.Sprintf("DROP TABLE %s;", Quote(table))
}

func DropColumn(table, column string) string {
	return Dropping(fmt.Sprintf("column %q of %q and its values", column, table)) +
		fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", Quote(table), Quote(column))
}

// CreateIndex writes an index that is not a UNIQUE constraint.
func CreateIndex(table string, ix *schema.Index) string {
	unique := ""
	if ix.Unique {
		unique = "UNIQUE "
	}
	return fmt.Sprintf("CREATE %sINDEX %s ON %s (%s);", unique, Quote(ix.Name), Quote(table), QuoteAll(ix.Columns))
}

// NoDefault is the warning before adding a NOT NULL column without a
// default, which fails once the table has rows. The draft does not guess
// a value (PHILOSOPHY 6).
func NoDefault(table, column string) string {
	return fmt.Sprintf("-- ⚠ Column %q of %q is NOT NULL without a default, so this fails if %q has rows. Give the field a default, or add it as a pointer, fill it, then make it NOT NULL.\n", column, table, table)
}
