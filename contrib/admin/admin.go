// Package admin is maago's admin site (D-131): the types of the
// admin_gen.go that maago gen writes into each app with models when a
// package of the project imports this one.
package admin

import (
	"context"
	"net/url"

	"github.com/maago-web/maago"
)

// Prefix is the path the admin serves: no route of the project's own may
// be under it (D-131).
const Prefix = "/admin/"

// Model is one model as the admin sees it, generated from its tags
// (D-132).
type Model struct {
	App, Name string   // the app's package name, the model's Go name
	Fields    []Field  // the fields not admin=hidden, in declaration order
	List      []string // the Go names of the list's columns
	Rows      func(ctx context.Context, db maago.DB, q Query) ([]Row, error)
	Count     func(ctx context.Context, db maago.DB, q Query) (int64, error)

	// Form reads the form of the row with id, or of a new row when id is
	// 0, from vals, and saves it when it is valid; for a GET, vals is nil
	// and it only describes the inputs (D-119). A row that is not there
	// is sql.ErrNoRows.
	Form func(ctx context.Context, db maago.DB, id int64, vals url.Values) (Form, error)
	Add  bool // a new row can be made: no field the form leaves out would be a zero value (D-132)
	Edit bool // the form has a field to change
}

// Form is a model's form, read or to draw.
type Form struct {
	Inputs []Input
	Saved  bool // the form was valid, and its row written
}

// Input is one field of a Form, for drawing.
type Input struct {
	Name, Label, Help string
	Kind              string // text, integer, number, checkbox, select, datetime (a text input, to the microsecond), or readonly
	Raw, Error        string // the text to draw, and what is wrong with it
	Choices           []string
	Required          bool
	FK                string // the model an fk field refers to, as "app.Model"
}

// Query is which rows of a model's list to read. The zero Query reads
// none: Limit is the page's size.
type Query struct {
	Limit, Offset int
}

// Field is a field of a model in the admin.
type Field struct {
	Name, Label, Help string
	ReadOnly          bool   // admin=readonly, the ID, and auto= fields
	FK                string // the model it refers to, as "app.Model", or ""
}

// Row is one row of a list: its ID, and the values of the list's columns
// in order, as the model's Go types. A nil pointer is a typed nil, so a
// cell holding one is not == nil.
type Row struct {
	ID    int64
	Cells []any
}
