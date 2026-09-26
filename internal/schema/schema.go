// Package schema is a database schema as read back from a database, for
// comparing the migrations with the models (D-87, D-92). Text such as types
// and defaults stays in its dialect: a schema is only compared with one
// read from the same dialect.
//
// Names of indexes and constraints are kept for writing DROP statements,
// but never used to match one with another: those match by columns (D-87).
package schema

import (
	"cmp"
	"slices"
)

// Schema is the tables of one database, sorted by name.
type Schema struct {
	Tables []*Table
}

type Table struct {
	Name    string
	Strict  bool     // SQLite
	Columns []Column // in definition order, which a comparison ignores
	PK      []string // in key order
	PKAuto  string   // "AUTOINCREMENT"; PostgreSQL "BY DEFAULT" or "ALWAYS"; "" if none
	FKs     []FK
	Indexes []Index // unique ones included
	Checks  []Check
}

type Column struct {
	Name    string
	Type    string // upper case
	NotNull bool
	Default string // dialect.Normalize'd; "" if none
}

type Index struct {
	Name       string
	Columns    []string // in index order
	Unique     bool
	Constraint bool // a UNIQUE constraint rather than CREATE INDEX
}

type FK struct {
	Name       string // "" if unnamed
	Columns    []string
	RefTable   string
	RefColumns []string
	OnDelete   string
	OnUpdate   string
}

type Check struct {
	Name   string // "" if unnamed
	Column string // "" for a table constraint; PostgreSQL cannot tell them apart
	Expr   string // inside the parentheses, dialect.Normalize'd
}

// Sort puts tables in name order and what each holds in an order by what
// it covers, the same for every dialect.
func (s *Schema) Sort() {
	slices.SortFunc(s.Tables, func(a, b *Table) int { return cmp.Compare(a.Name, b.Name) })
	for _, t := range s.Tables {
		slices.SortFunc(t.FKs, func(a, b FK) int {
			return cmp.Or(slices.Compare(a.Columns, b.Columns), cmp.Compare(a.RefTable, b.RefTable),
				slices.Compare(a.RefColumns, b.RefColumns), cmp.Compare(a.OnDelete, b.OnDelete),
				cmp.Compare(a.OnUpdate, b.OnUpdate), cmp.Compare(a.Name, b.Name))
		})
		slices.SortFunc(t.Indexes, func(a, b Index) int {
			return cmp.Or(compareBool(a.Unique, b.Unique), slices.Compare(a.Columns, b.Columns), cmp.Compare(a.Name, b.Name))
		})
		slices.SortFunc(t.Checks, func(a, b Check) int {
			return cmp.Or(cmp.Compare(a.Expr, b.Expr), cmp.Compare(a.Column, b.Column))
		})
	}
}

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}
