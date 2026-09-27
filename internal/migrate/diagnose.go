package migrate

import (
	"go/token"
	"slices"

	"github.com/maago-web/maago/internal/diag"
)

// Diagnose reports each change as a difference between the migrations
// and the models. pos gives where the models have table and column, or
// the table alone for column "", or nothing. An index, foreign key or
// check that differs only because its column was added or dropped is left
// out: fixing the column fixes it too (D-61).
func Diagnose(cs []Change, pos func(table, column string) token.Position) []diag.Diagnostic {
	var ds []diag.Diagnostic
	for _, c := range cs {
		cols := columns(c)
		if follows(c, cols, cs) {
			continue
		}
		col := ""
		if len(cols) > 0 {
			col = cols[0]
		}
		ds = append(ds, diag.New(pos(c.Table(), col), c.Message(), "run maago makemigrations"))
	}
	return ds
}

// columns are the columns c is about, in order.
func columns(c Change) []string {
	switch {
	case c.New != nil:
		return []string{c.New.Name}
	case c.Old != nil:
		return []string{c.Old.Name}
	case c.Index != nil:
		return c.Index.Columns
	case c.FK != nil:
		return c.FK.Columns
	case c.Check != nil && c.Check.Column != "":
		return []string{c.Check.Column}
	}
	return nil
}

// follows reports whether c, an index, foreign key or check, holds a
// column that another change of cs adds or drops along with it.
func follows(c Change, cols []string, cs []Change) bool {
	var with Kind
	switch c.Kind {
	case DropIndex, DropFK, DropCheck:
		with = DropColumn
	case AddIndex, AddFK, AddCheck:
		with = AddColumn
	default:
		return false
	}
	return slices.ContainsFunc(cs, func(o Change) bool {
		return o.Kind == with && o.Table() == c.Table() && slices.Contains(cols, columns(o)[0])
	})
}
