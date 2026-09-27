// Package migrate compares the schema the migrations make with the one
// the models make, and turns the difference into drafts and diagnostics
// (D-32, D-34).
package migrate

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/schema"
)

// Kind is what a Change does. Changes come in the order of their kinds:
// drops first, so that no foreign key holds up another drop, and foreign
// keys last, once what they point at exists.
type Kind int

const (
	DropFK Kind = iota
	DropIndex
	DropCheck
	DropPK
	DropTable
	DropColumn
	CreateTable
	AddColumn
	AlterType
	AlterNotNull
	AlterDefault
	AlterStrict
	AddPK
	AddIndex
	AddCheck
	AddFK
)

// Change is one difference between the migrations (From) and the models
// (To). A table only on one side is one change; what it holds is not
// listed again. Index, FK and Check come from From for a drop and from To
// for an add.
type Change struct {
	Kind     Kind
	From, To *schema.Table  // nil on the side without the table
	Old, New *schema.Column // column kinds only; nil on the side without it
	Index    *schema.Index
	FK       *schema.FK
	Check    *schema.Check
}

// Diff returns what turns from, read from the migrations, into to, read
// from the models. It sorts both in place, and the changes point into
// them. Renames show as a drop and an add: the
// schemas alone cannot tell them apart (D-37).
func Diff(from, to *schema.Schema) []Change {
	from.Sort()
	to.Sort()
	var cs []Change
	var dropped []*schema.Table
	for _, f := range from.Tables {
		if table(to, f.Name) == nil {
			dropped = append(dropped, f)
		}
	}
	for _, f := range dropOrder(dropped) {
		cs = append(cs, Change{Kind: DropTable, From: f})
	}
	for _, t := range to.Tables {
		f := table(from, t.Name)
		if f == nil {
			cs = append(cs, Change{Kind: CreateTable, To: t})
			continue
		}
		cs = append(cs, diffTable(f, t)...)
	}
	// Stable, so DropTable keeps its dependency order and the rest their
	// table name order, then the order Sort gave within a table.
	slices.SortStableFunc(cs, func(a, b Change) int {
		return cmp.Compare(a.Kind, b.Kind)
	})
	return cs
}

func diffTable(f, t *schema.Table) []Change {
	var cs []Change
	add := func(c Change) {
		c.From, c.To = f, t
		cs = append(cs, c)
	}
	for i := range f.Columns {
		if column(t, f.Columns[i].Name) == nil {
			add(Change{Kind: DropColumn, Old: &f.Columns[i]})
		}
	}
	for i := range t.Columns {
		n := &t.Columns[i]
		o := column(f, n.Name)
		if o == nil {
			add(Change{Kind: AddColumn, New: n})
			continue
		}
		if o.Type != n.Type {
			add(Change{Kind: AlterType, Old: o, New: n})
		}
		if o.NotNull != n.NotNull {
			add(Change{Kind: AlterNotNull, Old: o, New: n})
		}
		if o.Default != n.Default {
			add(Change{Kind: AlterDefault, Old: o, New: n})
		}
	}
	if f.Strict != t.Strict {
		add(Change{Kind: AlterStrict})
	}
	if !slices.Equal(f.PK, t.PK) || f.PKAuto != t.PKAuto {
		if len(f.PK) > 0 {
			add(Change{Kind: DropPK})
		}
		if len(t.PK) > 0 {
			add(Change{Kind: AddPK})
		}
	}
	for _, i := range unmatched(f.Indexes, t.Indexes, indexKey) {
		add(Change{Kind: DropIndex, Index: &f.Indexes[i]})
	}
	for _, i := range unmatched(t.Indexes, f.Indexes, indexKey) {
		add(Change{Kind: AddIndex, Index: &t.Indexes[i]})
	}
	for _, i := range unmatched(f.FKs, t.FKs, fkKey) {
		add(Change{Kind: DropFK, FK: &f.FKs[i]})
	}
	for _, i := range unmatched(t.FKs, f.FKs, fkKey) {
		add(Change{Kind: AddFK, FK: &t.FKs[i]})
	}
	checkKey := func(c schema.Check) string { return c.Expr }
	for _, i := range unmatched(f.Checks, t.Checks, checkKey) {
		add(Change{Kind: DropCheck, Check: &f.Checks[i]})
	}
	for _, i := range unmatched(t.Checks, f.Checks, checkKey) {
		add(Change{Kind: AddCheck, Check: &t.Checks[i]})
	}
	return cs
}

// unmatched returns the indexes of the items of a left without a partner
// in b. Each item of b partners at most one of a, so two equal checks
// against one leave one over.
func unmatched[T any](a, b []T, key func(T) string) []int {
	left := map[string]int{}
	for _, x := range b {
		left[key(x)]++
	}
	var out []int
	for i, x := range a {
		if k := key(x); left[k] > 0 {
			left[k]--
		} else {
			out = append(out, i)
		}
	}
	return out
}

// indexKey matches a unique index by its set of columns and any other by
// their order (D-87): unique (a, b) is unique (b, a), an index is not.
func indexKey(i schema.Index) string {
	cols := i.Columns
	if i.Unique {
		cols = slices.Sorted(slices.Values(cols))
	}
	return fmt.Sprintf("%t %q", i.Unique, cols)
}

func fkKey(k schema.FK) string {
	return fmt.Sprintf("%q %q %q %q %q", k.Columns, k.RefTable, k.RefColumns, k.OnDelete, k.OnUpdate)
}

// dropOrder puts tables that point at others before those, as PostgreSQL
// drops no table another still points at. It takes them in name order
// whenever it can; a cycle is broken at the first name, and PostgreSQL
// then refuses the draft. Quadratic, as a migration drops few tables.
func dropOrder(ts []*schema.Table) []*schema.Table {
	var out []*schema.Table
	for len(ts) > 0 {
		next := 0
		for i, t := range ts {
			if !slices.ContainsFunc(ts, func(o *schema.Table) bool { return o != t && pointsAt(o, t.Name) }) {
				next = i
				break
			}
		}
		out = append(out, ts[next])
		ts = slices.Delete(ts, next, next+1)
	}
	return out
}

func pointsAt(t *schema.Table, name string) bool {
	return slices.ContainsFunc(t.FKs, func(k schema.FK) bool { return k.RefTable == name })
}

func table(s *schema.Schema, name string) *schema.Table {
	for _, t := range s.Tables {
		if t.Name == name {
			return t
		}
	}
	return nil
}

func column(t *schema.Table, name string) *schema.Column {
	for i := range t.Columns {
		if t.Columns[i].Name == name {
			return &t.Columns[i]
		}
	}
	return nil
}

// Table is the name of the table c changes.
func (c Change) Table() string {
	if c.To != nil {
		return c.To.Name
	}
	return c.From.Name
}

// Message says what differs, naming tables and columns as the database
// does. "Missing" is in the models but not the migrations.
func (c Change) Message() string {
	return "migrations do not match models: " + c.what()
}

func (c Change) what() string {
	switch c.Kind {
	case CreateTable:
		return fmt.Sprintf("table %q is missing", c.To.Name)
	case DropTable:
		return fmt.Sprintf("table %q is not in the models", c.From.Name)
	case AddColumn:
		return fmt.Sprintf("column %q of %q is missing", c.New.Name, c.To.Name)
	case DropColumn:
		return fmt.Sprintf("column %q of %q is not in the models", c.Old.Name, c.From.Name)
	case AlterType:
		return fmt.Sprintf("column %q of %q is %s, the models have %s", c.New.Name, c.To.Name, c.Old.Type, c.New.Type)
	case AlterNotNull:
		return fmt.Sprintf("column %q of %q is %s, the models have %s", c.New.Name, c.To.Name, null(c.Old), null(c.New))
	case AlterDefault:
		return fmt.Sprintf("column %q of %q has %s, the models have %s", c.New.Name, c.To.Name, dflt(c.Old), dflt(c.New))
	case AlterStrict:
		return fmt.Sprintf("table %q is %s, the models have %s", c.To.Name, strict(c.From), strict(c.To))
	case DropPK:
		return fmt.Sprintf("primary key %s of %q is not in the models", pk(c.From), c.From.Name)
	case AddPK:
		return fmt.Sprintf("primary key %s of %q is missing", pk(c.To), c.To.Name)
	case DropIndex:
		return fmt.Sprintf("%s of %q is not in the models", index(c.Index), c.From.Name)
	case AddIndex:
		return fmt.Sprintf("%s of %q is missing", index(c.Index), c.To.Name)
	case DropFK:
		return fmt.Sprintf("%s of %q is not in the models", fk(c.FK), c.From.Name)
	case AddFK:
		return fmt.Sprintf("%s of %q is missing", fk(c.FK), c.To.Name)
	case DropCheck:
		return fmt.Sprintf("check (%s) of %q is not in the models", display(c.Check.Expr), c.From.Name)
	case AddCheck:
		return fmt.Sprintf("check (%s) of %q is missing", display(c.Check.Expr), c.To.Name)
	}
	panic(fmt.Sprintf("migrate: unknown change kind %d", c.Kind))
}

func null(c *schema.Column) string {
	if c.NotNull {
		return "NOT NULL"
	}
	return "NULL"
}

func dflt(c *schema.Column) string {
	if c.Default == "" {
		return "no default"
	}
	return "DEFAULT " + display(c.Default)
}

func strict(t *schema.Table) string {
	if t.Strict {
		return "STRICT"
	}
	return "not STRICT"
}

func pk(t *schema.Table) string {
	s := "(" + dialect.QuoteAll(t.PK) + ")"
	if t.PKAuto != "" {
		s += " " + t.PKAuto
	}
	return s
}

func index(i *schema.Index) string {
	s := "index (" + dialect.QuoteAll(i.Columns) + ")"
	if i.Unique {
		s = "unique " + s
	}
	return s
}

func fk(k *schema.FK) string {
	s := fmt.Sprintf("foreign key (%s) -> %s (%s) ON DELETE %s", dialect.QuoteAll(k.Columns), dialect.Quote(k.RefTable), dialect.QuoteAll(k.RefColumns), k.OnDelete)
	if k.OnUpdate != "NO ACTION" {
		s += " ON UPDATE " + k.OnUpdate
	}
	return s
}

// display writes Normalize'd SQL the way people write it, for messages
// only: Normalize spaces every token, so 1.5 reads as "1 . 5". It drops
// parentheses around the whole, which PostgreSQL adds to checks.
func display(sql string) string {
	stmts, _ := dialect.Split("", sql)
	if len(stmts) != 1 {
		return sql
	}
	toks := stmts[0].Tokens
	for len(toks) > 2 && toks[0].Text == "(" && dialect.Closing(toks, 0) == len(toks)-1 {
		toks = toks[1 : len(toks)-1]
	}
	var b strings.Builder
	var prev dialect.Token
	for i, t := range toks {
		if i > 0 && spaced(prev, t) {
			b.WriteByte(' ')
		}
		b.WriteString(t.Text)
		prev = t
	}
	return b.String()
}

func spaced(prev, t dialect.Token) bool {
	switch {
	case prev.Text == "(" || prev.Text == "[" || prev.Text == "." || prev.Text == ":":
		return false
	case t.Text == ")" || t.Text == "]" || t.Text == "," || t.Text == "." || t.Text == ":" || t.Text == "[":
		return false
	case prev.Kind == dialect.Punct && t.Kind == dialect.Punct &&
		strings.Contains("<>=!|", prev.Text) && strings.Contains("<>=!|", t.Text):
		return false
	}
	return true
}
