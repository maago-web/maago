package mapper

import (
	"strconv"
	"strings"

	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/project"
)

// CRUD is the SQL of a model's primary-key functions (D-21). Each has a
// fixed contract for the generated code: Get scans one row into the
// fields in order, or gives sql.ErrNoRows; Insert reads the one id it
// returns into ID; Update and Delete give sql.ErrNoRows when no row was
// affected.
type CRUD struct {
	Model               *model.Model
	Get, Insert, Delete Statement
	Update              *Statement // nil when the model has nothing but ID to set: no Update function
	Version             string     // the field Update compares, its first auto=update one; "" for none (D-150)
	Exists              Statement  // whether the row of an id is there, for an Update of no row
}

// Statement is SQL and the fields its placeholders take, in order: Args[i]
// is placeholder i+1.
type Statement struct {
	SQL  string
	Args []string
}

// CRUDs is the CRUD of each model of app, in declaration order. Every
// field is written: the Go value, zero or not, wins over the column's
// default (D-83), which is for migrations and hand-written SQL.
func CRUDs(app *model.App, d project.Dialect) []CRUD {
	mark := "$"
	if d == project.SQLite {
		mark = "?"
	}
	ph := func(n int) string { return mark + strconv.Itoa(n) }
	var out []CRUD
	for _, m := range app.Models {
		table := dialect.Quote(m.Table)
		var all, cols, marks, set, insertArgs, updateArgs []string
		for _, f := range m.Fields {
			all = append(all, dialect.Quote(f.Column))
			if f.Name == "ID" {
				continue
			}
			cols = append(cols, dialect.Quote(f.Column))
			insertArgs = append(insertArgs, f.Name)
			marks = append(marks, ph(len(insertArgs)))
			if f.Auto != "create" {
				updateArgs = append(updateArgs, f.Name)
				set = append(set, dialect.Quote(f.Column)+" = "+ph(len(updateArgs)))
			}
		}
		c := CRUD{
			Model:  m,
			Get:    Statement{"SELECT " + strings.Join(all, ", ") + " FROM " + table + ` WHERE "id" = ` + ph(1), []string{"ID"}},
			Delete: Statement{"DELETE FROM " + table + ` WHERE "id" = ` + ph(1), []string{"ID"}},
		}
		if cols == nil {
			c.Insert = Statement{"INSERT INTO " + table + ` DEFAULT VALUES RETURNING "id"`, nil}
		} else {
			c.Insert = Statement{"INSERT INTO " + table + " (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(marks, ", ") + `) RETURNING "id"`, insertArgs}
		}
		c.Exists = Statement{"SELECT 1 FROM " + table + ` WHERE "id" = ` + ph(1), []string{"ID"}}
		if set != nil {
			where := ` WHERE "id" = ` + ph(len(updateArgs)+1)
			args := append(updateArgs, "ID")
			if f := m.Version(); f != nil { // the row is the one read (D-150)
				c.Version = f.Name
				where += " AND " + dialect.Quote(f.Column) + " = " + ph(len(args)+1)
				args = append(args, "version:"+f.Name)
			}
			c.Update = &Statement{"UPDATE " + table + " SET " + strings.Join(set, ", ") + where, args}
		}
		out = append(out, c)
	}
	return out
}

// PolicyGet is the SQL of GetX of c's model, which applies its policy p:
// the row of the id, if p lets the user read it. Args are ID, then the
// parameters of p in the order of their placeholders.
func PolicyGet(c CRUD, p *Policy, d project.Dialect) Statement {
	sql, args := p.sql(d, []string{"ID"})
	return Statement{strings.Replace(c.Get.SQL, " WHERE ", " "+p.Alias+" WHERE ", 1) + " AND " + sql, args}
}
