// Package schema is a database schema as read back from a database, for
// comparing the migrations with the models (D-87, D-92). Text such as types
// and defaults stays in its dialect: a schema is only compared with one
// read from the same dialect.
//
// Names of indexes and constraints are kept for writing DROP statements,
// but never used to match one with another: those match by columns (D-87).
package schema

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
