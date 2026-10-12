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
}

// Statements is the SQL of m's list. The rows are in m's ordering, or
// newest first, and rows that order the same by the ID, newest first.
// NULL is last either way, as it is not in both dialects alone.
func Statements(m *model.Model, d project.Dialect) SQL {
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
	limit, offset := "$1", "$2"
	if d == project.SQLite {
		limit, offset = "?", "?"
	}
	table := dialect.Quote(m.Table)
	update, _ := updateSQL(m, d)
	var exists []string
	for _, f := range m.Fields {
		if editable(f) && f.FK != "" {
			exists = append(exists, existsSQL(f, d))
		}
	}
	return SQL{
		List:   "SELECT " + strings.Join(cols, ", ") + " FROM " + table + " ORDER BY " + strings.Join(order, ", ") + " LIMIT " + limit + " OFFSET " + offset,
		Count:  "SELECT COUNT(*) FROM " + table,
		Update: update,
		Exists: exists,
	}
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
		sql := Statements(m, d)
		why := blocks(m)
		if why != "" {
			noAdd = append(noAdd, why)
		}
		v := modelView{Name: m.Name, List: m.AdminList, ListSQL: mapper.GoString(sql.List), CountSQL: mapper.GoString(sql.Count),
			Form: formCode(m, d, models, imports), Add: why == "", Edit: slices.ContainsFunc(m.Fields, editable)}
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
