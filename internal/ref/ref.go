// Package ref is Maago's reference: the topics of maago doc, and the
// closed lists the checkers accept (D-152). A checker reads its keys from
// here, so what maago doc prints is what maago gen takes. Text is written
// in go doc's comment syntax, for maago doc and the package doc alike.
package ref

import (
	"fmt"
	"io"
	"strings"
)

// The topics, by name.
const (
	Model    = "model"
	Query    = "query"
	View     = "view"
	Form     = "form"
	Template = "template"
	Auth     = "auth"
	Admin    = "admin"
	Main     = "main"
)

// Topic is one page of maago doc.
type Topic struct {
	Name, Title string
	Summary     string // one paragraph
	Text        string
	Lists       []*List
	API         []string // the names of package maago it explains, for its go doc
}

// List is a closed list: each Item is one name a checker accepts.
type List struct {
	Name  string
	Items []Item
}

// Item is one name of a List, what it means, and the values it takes when
// those are closed too.
type Item struct {
	Name, Text string
	Values     []string
}

// Names are the items' names, in order.
func (l *List) Names() []string {
	var names []string
	for _, it := range l.Items {
		names = append(names, it.Name)
	}
	return names
}

// Values are the closed values of item name; it panics on a name not in
// l, which is a bug in the checker.
func (l *List) Values(name string) []string {
	for _, it := range l.Items {
		if it.Name == name {
			return it.Values
		}
	}
	panic("ref: no item " + name + " in " + l.Name)
}

// Find returns the topic called name, or nil.
func Find(name string) *Topic {
	for _, t := range Topics {
		if t.Name == name {
			return t
		}
	}
	return nil
}

// Print writes t as maago doc shows it: the title, the summary, the
// text, then each list with an item to a line, the name first.
func Print(w io.Writer, t *Topic) {
	fmt.Fprintf(w, "%s\n\n%s\n\n%s\n", t.Title, t.Summary, t.Text)
	for _, l := range t.Lists {
		width := 0
		for _, it := range l.Items {
			width = max(width, len(it.Name))
		}
		fmt.Fprintf(w, "\n%s:\n", l.Name)
		for _, it := range l.Items {
			text := it.Text
			if len(it.Values) > 0 {
				text += " Values: " + strings.Join(it.Values, ", ") + "."
			}
			fmt.Fprintf(w, "  %-*s  %s\n", width, it.Name, text)
		}
	}
}

// The lists the checkers read.
var (
	ModelTags = &List{"Model tags (on the embedded maago.Model)", []Item{
		{"table", "The table's name, lowercase SQL: table=posts. Without it, app_model in snake_case.", nil},
		{"ordering", "The order of the admin's lists: ordering=-PublishedAt, or ordering=(-PublishedAt,Title); - is descending.", nil},
		{"unique", "A unique constraint over fields: unique=(AuthorID,Slug).", nil},
		{"index", "An index over fields: index=(Status,PublishedAt).", nil},
		{"admin", "admin=hidden leaves the model out of the admin.", []string{"hidden"}},
		{"admin_list", "The admin list's columns: admin_list=(Title,Status).", nil},
		{"admin_search", "The string fields the admin's search box looks in: admin_search=(Title).", nil},
		{"admin_filter", "The fields the admin's list filters by, each with a few values (bool, choices, fk): admin_filter=(Status).", nil},
	}}
	FieldTags = &List{"Field tags", []Item{
		{"max", "The most characters of a string: max=200. VARCHAR(n) or a CHECK in the database, a rule in model forms.", nil},
		{"unique", "A unique constraint on the column.", nil},
		{"index", "An index on the column. An fk gets one without it.", nil},
		{"default", "The column's default: default='draft' for a string, default=0, default=true, default=1.5 without quotes.", nil},
		{"choices", "The values a string may hold, split by /: choices=draft/published. A CHECK, and a select in forms.", nil},
		{"label", "The name people see, in forms and the admin: label='Title'.", nil},
		{"help", "A line under the field in forms and the admin: help='Shown in lists'.", nil},
		{"fk", "The model an int64 ID field refers to: fk=Post, or fk=auth.User for another app's. *int64 for an optional one.", nil},
		{"auto", "The generated CRUD fills a time.Time with the time: create when inserting, update on every UpdateX, which then compares it (409 on a stale copy).", []string{"create", "update"}},
		{"admin", "hidden leaves the field out of the admin, readonly shows it without an input.", []string{"hidden", "readonly"}},
	}}
	QueryKeys = &List{"Header lines (-- key: …)", []Item{
		{"name", "-- name: Name :kind [Result]: the Go function, what it returns, and the result type.", nil},
		{"param", "-- param: name[?] type: a value the SQL uses as :name; ? when it may be missing.", nil},
		{"column", "-- column: name[?] type: the type of a result column the database cannot tell (an expression, an outer join's side); ? when it may be NULL.", nil},
	}}
	QueryKinds = &List{"Kinds", []Item{
		{":one", "One row: sql.ErrNoRows when none, maago.ErrManyRows when more.", nil},
		{":many", "A slice of rows; none is an empty slice.", nil},
		{":exec", "No rows: the number of rows changed (0 is no error).", nil},
	}}
	InTags = &List{"In tags (where each field of In comes from)", []Item{
		{"path", "path=id reads {id} of the route's path: string, int64 or int32. A value that does not parse is a 404.", nil},
		{"query", "query=page reads ?page=: string, int64, int32, bool or float64, a pointer to one when it may be missing. A value that does not parse is a 400.", nil},
		{"form", "form, with no value, reads the request's form (or JSON on an API route) into a form type of the package.", nil},
	}}
	RouteOptions = &List{"Route options (after the view)", []Item{
		{"API", "maago.API answers JSON, and errors as RFC 9457 problems; a form error is a 422.", nil},
		{"Block", `maago.Block("name") renders only that block of the page, for htmx-like partial updates.`, nil},
		{"NoTx", "maago.NoTx lets a writing route run a view that takes maago.DB, outside a transaction: for a write that must stay when the view then fails (a counted login attempt). Give it to every writing route of that view.", nil},
	}}
	FormTypes = &List{"Form field types (maago.Field[T])", []Item{
		{"string", "Text.", nil},
		{"int64", "A whole number.", nil},
		{"int32", "A whole number.", nil},
		{"float64", "A number.", nil},
		{"bool", "A checkbox: on is true, absent is false.", nil},
		{"time.Time", "A date or a time, in Config.TimeZone.", nil},
		{"maago.File", "An uploaded file, in an HTML form only (a multipart one).", nil},
	}}
	FormTags = &List{"Form field tags", []Item{
		{"label", "The name people see: label='Title'. A model form takes the model field's.", nil},
		{"help", "A line under the input: help='…'.", nil},
		{"choices", "The values a string may hold, split by /: choices=draft/published.", nil},
		{"min", "The fewest characters of a string: min=3.", nil},
		{"max", "The most characters of a string: max=200.", nil},
		{"min_value", "The smallest number: min_value=1.", nil},
		{"max_value", "The largest number: max_value=100.", nil},
		{"optional", "An empty value is no error (a bool never is required). A pointer field of a model is optional in its model form.", nil},
		{"accept", "The kinds of file a maago.File field takes, split by /: accept=png/jpeg, as the content's first bytes tell: png, jpeg, gif, webp, pdf, zip (zip includes .docx and .xlsx).", nil},
	}}
)

// The lists whose source is elsewhere; tests hold them to it.
var (
	FieldTypes = &List{"Field types", []Item{
		{"int64", "BIGINT in PostgreSQL, INTEGER in SQLite.", nil},
		{"int32", "INTEGER.", nil},
		{"bool", "BOOLEAN; INTEGER 0 or 1 in SQLite.", nil},
		{"float64", "DOUBLE PRECISION; REAL in SQLite.", nil},
		{"string", "TEXT.", nil},
		{"time.Time", "TIMESTAMPTZ; TEXT in SQLite.", nil},
		{"[]byte", "BYTEA; BLOB in SQLite.", nil},
	}}
	Methods = &List{"Route methods", []Item{
		{"GET", "maago.GET(path, name, View, options…): reads; its view cannot take maago.Tx.", nil},
		{"POST", "maago.POST: writes, so its view takes tx maago.Tx (or maago.DB with maago.NoTx).", nil},
		{"PUT", "maago.PUT: writes, as POST.", nil},
		{"PATCH", "maago.PATCH: writes, as POST.", nil},
		{"DELETE", "maago.DELETE: writes, as POST.", nil},
	}}
	TemplateFuncs = &List{"Template functions (beside text/template's)", []Item{
		{"url", `{{url "route_name" .ID}}: the route's path, with its {wildcards} in order.`, nil},
		{"date", `{{date "2006-01-02" .PublishedAt}}: a time in Go's layout, in Config.TimeZone; "" for a nil one.`, nil},
		{"static", `{{static "app/file.css"}}: the hashed URL of a file of an app's static/.`, nil},
	}}
)
