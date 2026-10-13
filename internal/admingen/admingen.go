// Package admingen writes the admin_gen.go of each app with models when
// the project uses the admin (D-131): what contrib/admin needs to know
// of the models, as typed Go.
package admingen

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/maago-web/maago/contrib/admin"
	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/mapper"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/pipeline"
	"github.com/maago-web/maago/internal/project"
	"github.com/maago-web/maago/internal/web"
)

// Path is the import path of the admin.
const Path = "github.com/maago-web/maago/contrib/admin"

// Imports are the packages admin_gen.go may import (D-66).
var Imports = []string{"context", "net/url", "time", "github.com/maago-web/maago", Path}

//go:embed templates/admin.go.tmpl
var text string

var tmpl = template.Must(template.New("admin").Parse(text))

// SQL is the SQL of a model's list in the admin: its rows, a page at a
// time, and how many there are (D-134).
type SQL struct {
	List, Count string   // List takes the limit and the offset
	Update      string   // the form's, or "" when it changes nothing (D-132)
	Exists      []string // the check of each fk field the form changes
	Row         string   // whether the row of an id is there
	Refs        []Ref    // the counts of the rows that refer to a row (D-135)
}

// Ref is a field of a model, of any app, that refers to the model, and
// the count of the rows whose field holds an id. Self: the field is of
// the same table, and the count leaves out the row itself, which the
// database lets go (NO ACTION is checked at the statement's end).
type Ref struct {
	Model, Field, SQL string // Model as "app.Model"
	Self              bool
}

// Statements is the SQL of m's admin; apps are all the project's, for the
// fields that refer to m. The rows are in m's ordering, or
// newest first, and rows that order the same by the ID, newest first.
// NULL is last either way, as it is not in both dialects alone.
func Statements(m *model.Model, d project.Dialect, apps []*model.App) SQL {
	var order []string
	byID := false
	for _, o := range m.Ordering {
		f := m.Field(o.Field)
		s := dialect.Quote(f.Column)
		if o.Desc {
			s += " DESC"
		}
		if f.Pointer {
			s += " NULLS LAST"
		}
		order = append(order, s)
		byID = byID || f.Name == "ID"
	}
	if !byID {
		order = append(order, dialect.Quote(m.Field("ID").Column)+" DESC")
	}
	var cols []string
	for _, f := range m.Fields {
		cols = append(cols, dialect.Quote(f.Column))
	}
	where, n := whereSQL(m, d)
	limit, offset := mark(d, n+1), mark(d, n+2)
	table := dialect.Quote(m.Table)
	update, _ := updateSQL(m, d)
	var exists []string
	for _, f := range m.Fields {
		if editable(f) && f.FK != "" {
			exists = append(exists, existsSQL(f, d))
		}
	}
	var refs []Ref
	for _, a := range apps {
		for _, x := range a.Models {
			for _, f := range x.Fields {
				if f.FK != m.Table {
					continue
				}
				ref := Ref{Model: a.Name + "." + x.Name, Field: f.Name, Self: x.Table == m.Table,
					SQL: "SELECT COUNT(*) FROM " + dialect.Quote(x.Table) + " WHERE " + dialect.Quote(f.Column) + " = " + mark(d, 1)}
				if ref.Self {
					ref.SQL += ` AND "id" <> ` + mark(d, 2)
				}
				refs = append(refs, ref)
			}
		}
	}
	return SQL{
		List:   "SELECT " + strings.Join(cols, ", ") + " FROM " + table + where + " ORDER BY " + strings.Join(order, ", ") + " LIMIT " + limit + " OFFSET " + offset,
		Count:  "SELECT COUNT(*) FROM " + table + where,
		Update: update,
		Exists: exists,
		Row:    "SELECT 1 FROM " + table + ` WHERE "id" = ` + mark(d, 1),
		Refs:   refs,
	}
}

// whereSQL is the WHERE of m's list, and the number of its values: the
// search, then each filter, each off when its value is NULL (D-134). A
// value's typed use comes before its IS NULL, which PostgreSQL could not
// type alone.
func whereSQL(m *model.Model, d project.Dialect) (string, int) {
	var conds []string
	n := 0
	if m.AdminSearch != nil {
		n++
		escape := `'\'`
		if d == project.Postgres {
			escape = `E'\\'` // a backslash, whatever standard_conforming_strings is
		}
		var or []string
		for _, name := range m.AdminSearch {
			or = append(or, "LOWER("+dialect.Quote(m.Field(name).Column)+") LIKE LOWER("+mark(d, n)+") ESCAPE "+escape)
		}
		conds = append(conds, "("+strings.Join(or, " OR ")+" OR "+mark(d, n)+" IS NULL)")
	}
	for _, name := range m.AdminFilter {
		n++
		conds = append(conds, "("+dialect.Quote(m.Field(name).Column)+" = "+mark(d, n)+" OR "+mark(d, n)+" IS NULL)")
	}
	if conds == nil {
		return "", 0
	}
	return " WHERE " + strings.Join(conds, " AND "), n
}

// filterKind is how the list filters by f: by its choices, as a bool, or
// by the id it refers to.
func filterKind(f *model.Field) string {
	switch {
	case f.Choices != nil:
		return "choice"
	case f.Type == model.Bool:
		return "bool"
	}
	return "fk"
}

// argsCode is the Go that reads the values of m's WHERE from q, and the
// list of them; fail is what returns on an error.
func argsCode(m *model.Model, fail string) (code, list string) {
	var b strings.Builder
	var args []string
	if m.AdminSearch != nil {
		b.WriteString("vSearch := admin.Like(q.Search)\n")
		args = append(args, "vSearch")
	}
	for _, name := range m.AdminFilter {
		f := m.Field(name)
		v, key := "v"+f.Name, strconv.Quote(model.Snake(f.Name))
		switch filterKind(f) {
		case "choice":
			var qs []string
			for _, c := range f.Choices {
				qs = append(qs, strconv.Quote(c))
			}
			fmt.Fprintf(&b, "%s, err := admin.Choice(q.Filters[%s], %s)\n", v, key, strings.Join(qs, ", "))
		case "bool":
			fmt.Fprintf(&b, "%s, err := admin.YesNo(q.Filters[%s])\n", v, key)
		default:
			fmt.Fprintf(&b, "%s, err := admin.ID(q.Filters[%s])\n", v, key)
		}
		fmt.Fprintf(&b, "if err != nil {\nreturn %s, err\n}\n", fail)
		args = append(args, v)
	}
	if args == nil {
		return "", ""
	}
	return b.String(), strings.Join(args, ", ") + ", "
}

// File is the admin_gen.go of app, whose list SQL is in d. apps are all
// the project's, for the models an FK refers to.
func File(d project.Dialect, app *model.App, apps []*model.App) []byte {
	type field struct {
		Name, Label, Help, FK string
		ReadOnly              bool
	}
	type modelView struct {
		Name              string
		Fields            []field
		List              []string
		ListSQL, CountSQL string
		Form              string
		Add, Edit         bool
		Row               string
		RowsArgs          string // the Go that reads the WHERE's values, in Rows
		CountArgs         string // and in Count
		Args              string // the values, each followed by ", "
		ArgList           string // the values
		Search            []string
		Filters           []map[string]string
		Refs              []map[string]string
	}
	models := map[string]string{} // each table's model, as "app.Model"
	for _, a := range apps {
		for _, m := range a.Models {
			models[m.Table] = a.Name + "." + m.Name
		}
	}
	var ms []modelView
	var noAdd []string
	imports := map[string]bool{}
	for _, m := range app.Models {
		if m.AdminHidden {
			continue
		}
		sql := Statements(m, d, apps)
		why := blocks(m)
		if why != "" {
			noAdd = append(noAdd, why)
		}
		v := modelView{Name: m.Name, List: m.AdminList, ListSQL: mapper.GoString(sql.List), CountSQL: mapper.GoString(sql.Count),
			Form: formCode(m, d, models, imports), Add: why == "", Edit: slices.ContainsFunc(m.Fields, editable), Row: mapper.GoString(sql.Row), Search: m.AdminSearch}
		v.RowsArgs, v.Args = argsCode(m, "nil")
		v.CountArgs, _ = argsCode(m, "0")
		v.ArgList = strings.TrimSuffix(v.Args, ", ")
		for _, name := range m.AdminFilter {
			f := m.Field(name)
			choices := ""
			if f.Choices != nil {
				var qs []string
				for _, c := range f.Choices {
					qs = append(qs, strconv.Quote(c))
				}
				choices = ", Choices: []string{" + strings.Join(qs, ", ") + "}"
			}
			v.Filters = append(v.Filters, map[string]string{"Key": model.Snake(f.Name), "Field": f.Name, "Kind": filterKind(f), "Choices": choices})
		}
		for _, ref := range sql.Refs {
			self := ""
			if ref.Self {
				self = ", Self: true"
			}
			v.Refs = append(v.Refs, map[string]string{"Model": ref.Model, "Field": ref.Field, "SQL": mapper.GoString(ref.SQL), "Self": self})
		}
		for _, f := range m.Fields {
			if f.Admin == "hidden" {
				continue
			}
			label := f.Label
			if label == "" {
				label = f.Name
			}
			ro := f.Admin == "readonly" || f.Name == "ID" || f.Auto != ""
			v.Fields = append(v.Fields, field{f.Name, label, f.Help, models[f.FK], ro})
			if m.AdminList == nil {
				v.List = append(v.List, f.Name)
			}
		}
		ms = append(ms, v)
	}
	paths := []string{Path}
	if ms != nil {
		paths = []string{"context", "net/url"}
		if imports["time"] {
			paths = append(paths, "time")
		}
		paths = append(paths, "", "github.com/maago-web/maago", Path)
	}
	var b bytes.Buffer
	err := tmpl.Execute(&b, map[string]any{"Marker": pipeline.Marker, "Package": app.Name, "Imports": paths, "Models": ms, "NoAdd": noAdd})
	if err != nil {
		panic(err)
	}
	out, err := format.Source(b.Bytes())
	if err != nil { // a bug of maago: never write what is not Go
		panic(fmt.Sprintf("admingen: admin_gen.go is not Go: %v\n%s", err, b.Bytes()))
	}
	return out
}

// Use reports whether the project uses the admin: a file of it that is
// neither a test nor generated imports it (D-131). If it does, each app
// with models must hand its Admin to the admin, and no route may be
// under the admin's Prefix, where it would hide a screen of the admin or
// make the server panic at start. byDir is what web.Parse returns.
func Use(fset *token.FileSet, byDir map[string][]*ast.File, modulePath string, apps []*model.App, routes []*web.App) (bool, []diag.Diagnostic) {
	used := false
	for _, files := range byDir {
		for _, f := range files {
			for _, imp := range f.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				used = used || p == Path
			}
		}
	}
	if !used {
		return false, nil
	}
	var ds []diag.Diagnostic
	names := map[string]string{}
	for _, a := range apps {
		names[web.ImportPath(modulePath, a.Dir)] = a.Name
	}
	handed := web.Uses(byDir, names, "Admin")
	for _, a := range apps {
		if !handed[web.ImportPath(modulePath, a.Dir)] {
			ds = append(ds, diag.New(fset.Position(a.Models[0].Pos), "app "+a.Name+" has models, but no package uses "+a.Name+".Admin",
				"add "+a.Name+".Admin to admin.Config.Models in main (D-131), then run maago gen: go build works after gen"))
		}
	}
	for _, wa := range routes {
		for _, r := range wa.Routes {
			if r.Path == strings.TrimSuffix(admin.Prefix, "/") || strings.HasPrefix(r.Path, admin.Prefix) {
				ds = append(ds, diag.New(fset.Position(r.PathPos), "route "+r.Path+" is under "+admin.Prefix+", which the admin serves",
					"move the route out of "+admin.Prefix+" (D-131)"))
			}
		}
	}
	return true, ds
}
