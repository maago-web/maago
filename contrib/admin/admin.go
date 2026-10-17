// Package admin is maago's admin site: the types of the
// admin_gen.go that maago gen writes into each app with models when a
// package of the project imports this one.
//
// What the admin does not do (v0):
//   - It saves a row through the rules of its model form only: the
//     work of the app's views, such as storing an email in lowercase or
//     deleting what goes with a row, does not happen.
//   - The form has its fields in declaration order, with no groups; an fk
//     field takes the number of the row, with no picker; a []byte field is
//     not edited. A row changed by someone else since the form was drawn is
//     a 409 when the model has an auto=update field, else the later save
//     wins.
//   - What the database refuses, such as a unique value twice or a
//     referenced row deleted meanwhile, is a 500, not a form error.
//   - A row that rows refer to is not deleted, even when those rows are of
//     a hidden model; nor is a chosen row that another chosen row
//     refers to, though a row that refers to itself is.
//   - The list is in the model's ordering, or newest first by ID, with
//     no other sort. A filter of a pointer field cannot choose NULL. The
//     search lowercases, which SQLite does to ASCII letters only,
//     and LOWER(column) LIKE on PostgreSQL may use no index.
//   - Groups of permissions, changing a password, and pages of your own
//     or templates in place of the admin's are not in v0.
package admin

import (
	"context"
	"net/url"
	"time"

	"github.com/maago-web/maago"
)

// Prefix is the path the admin serves: no route of the project's own may
// be under it.
const Prefix = "/admin/"

// Model is one model as the admin sees it, generated from its tags.
type Model struct {
	App, Name string   // the app's package name, the model's Go name
	Fields    []Field  // the fields not admin=hidden, in declaration order
	List      []string // the Go names of the list's columns
	Search    []string // the Go names of the fields the search box looks in (admin_search); none: no box
	Filters   []Filter
	Rows      func(ctx context.Context, db maago.DB, q Query) ([]Row, error)
	Count     func(ctx context.Context, db maago.DB, q Query) (int64, error)

	// Form reads the form of the row with id, or of a new row when id is
	// 0, from vals, and saves it when it is valid; for a GET, vals is nil
	// and it only describes the inputs. A row that is not there
	// is sql.ErrNoRows.
	Form func(ctx context.Context, db maago.DB, id int64, vals url.Values) (Form, error)
	Add  bool // a new row can be made: no field the form leaves out would be a zero value
	Edit bool // the form has a field to change

	// Delete deletes the row with id: sql.ErrNoRows if there is none. Refs
	// counts the rows that refer to it, which keep it from being deleted:
	// sql.ErrNoRows if there is none.
	Delete func(ctx context.Context, db maago.DB, id int64) error
	Refs   func(ctx context.Context, db maago.DB, id int64) ([]Ref, error)
}

// Ref is a field of a model, of any app, that refers to a model, and how
// many rows refer to one row of it.
type Ref struct {
	Model, Field string // Model as "app.Model"
	SQL          string // the count: SELECT COUNT(*) … WHERE "field" = ?, and AND "id" <> ? when Self
	Self         bool   // the field is of the same table: the row itself does not count
	N            int64
}

// Form is a model's form, read or to draw.
type Form struct {
	Inputs  []Input
	Saved   bool      // the form was valid, and its row written
	Version time.Time // the row's version the form carries, if its model has one
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

// Query is which rows of a model's list to read: the page (the zero
// Query reads none, Limit is the page's size), and the search and the
// filters as the URL has them, "" for none.
type Query struct {
	Limit, Offset int
	Search        string
	Filters       map[string]string // by Filter.Key
}

// Filter is a field the list filters by (admin_filter).
type Filter struct {
	Key, Field string   // the URL's key, the field's name in snake_case; its Go name
	Kind       string   // choice, bool or fk
	Choices    []string // of a choice
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
