package mapper

import (
	"bytes"
	"embed"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/pipeline"
	"github.com/maago-web/maago/internal/project"
)

//go:embed templates
var templateFiles embed.FS

var templates = template.Must(template.ParseFS(templateFiles, "templates/*.go.tmpl"))

// Checked is one query as the checks left it: its combinations and SQL
// texts (Variants) and its result type (Results), nil for :exec.
type Checked struct {
	Query    *Query
	Axes     []Axis
	Variants []Variant
	Result   *Result
}

// ModelsFile is the models_gen.go of app: the fields that are not columns
// (D-18), and each model's input type and CRUD (D-21, D-106).
func ModelsFile(d project.Dialect, app *model.App) []byte {
	g := &gen{d: d, imports: map[string]bool{"context": true, "database/sql": true}}
	var unsupported []string
	var ms []modelView
	for _, c := range CRUDs(app, d) {
		for _, u := range c.Model.Unsupported {
			unsupported = append(unsupported, c.Model.Name+"."+u.Name+": "+u.Reason)
		}
		ms = append(ms, g.model(c))
	}
	return g.execute("models.go.tmpl", map[string]any{"Package": app.Name, "Unsupported": unsupported, "Models": ms})
}

// QueriesFile is the queries_gen.go of one app's queries, in package pkg.
func QueriesFile(d project.Dialect, pkg string, qs []Checked) []byte {
	g := &gen{d: d, imports: map[string]bool{"context": true}}
	declared := map[*Result]bool{} // new result types, declared once
	var vs []queryView
	for _, c := range qs {
		vs = append(vs, g.query(c, declared))
	}
	return g.execute("queries.go.tmpl", map[string]any{"Package": pkg, "Queries": vs})
}

// gen writes one file: Go fragments for the templates, and the imports
// they need, since go/format does not remove an unused one.
type gen struct {
	d       project.Dialect
	imports map[string]bool // standard library only; maago's are added
}

func (g *gen) execute(name string, data map[string]any) []byte {
	data["Marker"] = pipeline.Marker
	data["Imports"] = append(append(slices.Sorted(maps.Keys(g.imports)), ""), pipeline.Runtime...)
	var b bytes.Buffer
	if err := templates.ExecuteTemplate(&b, name, data); err != nil {
		panic(err)
	}
	return pipeline.Format(name, b.Bytes())
}

type modelView struct {
	Name                string
	Input               []inputField
	Defaults            bool
	Build               []string // the fields of the new model, "Name: expression"
	InsertNow           bool
	Get, Insert, Delete string   // Go string literals of the SQL
	InsertArgs          []string // Go expressions
	Update              string   // "" if there is no Update
	UpdateArgs          []string
	UpdateNow           []string // the auto=update fields, set once the row is written
	Version             string   // the field Update compares, "" for none
	Exists              string   // the SQL that tells a missing row from a changed one
	InsertOnly          []string // the auto=create fields, which Update leaves
	Scan                string
}

type inputField struct{ Name, Type, Default string }

func (g *gen) model(c CRUD) modelView {
	m := c.Model
	v := modelView{Name: m.Name, Get: GoString(c.Get.SQL), Insert: GoString(c.Insert.SQL), Delete: GoString(c.Delete.SQL)}
	fields := map[string]*model.Field{}
	var dests []string
	for _, f := range m.Fields {
		fields[f.Name] = f
		dests = append(dests, g.dest("v."+f.Name, f.Type, f.Pointer))
		switch {
		case f.Name == "ID":
		case f.Auto != "":
			g.imports["time"], v.InsertNow = true, true
			v.Build = append(v.Build, f.Name+": now")
			if f.Auto == "update" {
				v.UpdateNow = append(v.UpdateNow, f.Name)
			} else {
				v.InsertOnly = append(v.InsertOnly, f.Name)
			}
		case f.Default != nil:
			lit := literal(f)
			v.Defaults = true
			v.Input = append(v.Input, inputField{f.Name, "maago.Opt[" + g.typ(f.Type, f.Pointer) + "]", lit})
			if f.Pointer {
				lit = "maago.Ptr[" + f.Type.String() + "](" + lit + ")"
			}
			v.Build = append(v.Build, f.Name+": in."+f.Name+".Or("+lit+")")
		default:
			v.Input = append(v.Input, inputField{f.Name, g.typ(f.Type, f.Pointer), ""})
			v.Build = append(v.Build, f.Name+": in."+f.Name)
		}
	}
	v.Scan = strings.Join(dests, ", ")
	for _, a := range c.Insert.Args {
		v.InsertArgs = append(v.InsertArgs, g.arg("o."+a, fields[a].Type, fields[a].Pointer))
	}
	if c.Update != nil {
		v.Update = GoString(c.Update.SQL)
		for _, a := range c.Update.Args {
			x := "o." + a
			if version, ok := strings.CutPrefix(a, "version:"); ok {
				a, x = version, "o."+version // the version as read
			} else if fields[a].Auto == "update" {
				x = "now"
			}
			v.UpdateArgs = append(v.UpdateArgs, g.arg(x, fields[a].Type, fields[a].Pointer))
		}
		if c.Version != "" {
			v.Version, v.Exists = c.Version, GoString(c.Exists.SQL)
			g.imports["errors"] = true
		}
	}
	return v
}

// literal is the Go constant of f's default, the value the DDL has (D-83):
// default=010 is 10, not Go's octal 8.
func literal(f *model.Field) string {
	switch f.Type {
	case model.String:
		return strconv.Quote(*f.Default)
	case model.Bool:
		return *f.Default
	}
	return dialect.Number(f.Type, *f.Default)
}

type queryView struct {
	Name, File   string
	Choices      []choiceView
	Params       string   // the parameter type; "" if there are no parameters
	Fields       []string // of Params, "Name Type"
	Result       string   // a new result type first given by this query
	ResultFields []string
	Returns      string
	Body         string
	Row          string // the type the scan function reads; "" for :exec
	Scan         string
}

type choiceView struct {
	Type, Param string
	Consts      []string
}

func (g *gen) query(c Checked, declared map[*Result]bool) queryView {
	q := c.Query
	v := queryView{Name: q.Name, File: q.File}
	if len(q.Params) > 0 {
		v.Params = q.Name + "Params"
	}
	for _, p := range q.Params {
		typ := g.paramType(q, p)
		v.Fields = append(v.Fields, FieldName(p.Name)+" "+typ)
		if p.Type.Choices != nil {
			ch := choiceView{Type: typ, Param: p.Name}
			for _, val := range p.Type.Choices {
				ch.Consts = append(ch.Consts, fmt.Sprintf("%s %s = %q", typ+FieldName(val), typ, val))
			}
			v.Choices = append(v.Choices, ch)
		}
	}
	zero := "0"
	if r := c.Result; r != nil && q.Scalar != 0 {
		g.imports["database/sql"], g.imports["fmt"] = true, true
		v.Row, v.Scan = g.typ(q.Scalar, false), g.dest("v", q.Scalar, false)
	} else if r != nil {
		g.imports["database/sql"], g.imports["fmt"] = true, true
		v.Row = r.Name
		if r.Model == nil && !declared[r] {
			declared[r] = true
			v.Result = r.Name
			for _, f := range r.Fields {
				v.ResultFields = append(v.ResultFields, f.Name+" "+g.typ(f.Type, f.Pointer))
			}
		}
		var dests []string
		for _, f := range r.Fields {
			dests = append(dests, g.dest("v."+f.Name, f.Type, f.Pointer))
		}
		v.Scan = strings.Join(dests, ", ")
	}
	switch q.Kind {
	case One:
		v.Returns, zero = "("+v.Row+", error)", v.Row+"{}"
		if q.Scalar != 0 {
			zero = scalarZero[q.Scalar]
		}
	case Many:
		v.Returns, zero = "([]"+v.Row+", error)", "nil"
	case Exec:
		v.Returns = "(int64, error)"
	}
	v.Body = g.body(c, zero)
	return v
}

// scalarZero is the zero value of a result that is a type (:one bool).
var scalarZero = map[model.Type]string{model.Int64: "0", model.Int32: "0", model.Float64: "0", model.Bool: "false", model.String: `""`, model.Time: "time.Time{}"}

func (g *gen) paramType(q *Query, p Param) string {
	switch {
	case p.Type.Choices != nil:
		return q.Name + FieldName(p.Name)
	case p.Type.Slice:
		return "[]" + g.typ(p.Type.Base, false)
	}
	return g.typ(p.Type.Base, p.Optional)
}

// body is the code of a query function: it works out which combination
// of condition values the parameters give, and runs that combination's
// fixed SQL text (D-22). Only a slice adds to the text: its placeholders.
func (g *gen) body(c Checked, zero string) string {
	q := c.Query
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	multi := len(c.Variants) > 1
	// The key numbers the combinations as Variants does: the first axis
	// turns slowest.
	weights := make([]int, len(c.Axes))
	for i := range c.Axes {
		weights[i] = 1
		for _, a := range c.Axes[i+1:] {
			weights[i] *= len(a.Values)
		}
	}
	if multi {
		w("key := 0")
	}
	for _, p := range q.Params {
		x := "p." + FieldName(p.Name)
		axis := slices.IndexFunc(c.Axes, func(a Axis) bool { return a.Param == p.Name })
		add := func(k int) {
			if multi && axis >= 0 && k > 0 {
				w("key += %d", k*weights[axis])
			}
		}
		switch {
		case p.Type.Choices != nil: // checked even when no condition looks at it
			g.imports["fmt"] = true
			w("switch %s {", x)
			for k, val := range p.Type.Choices {
				w("case %s:", q.Name+FieldName(p.Name)+FieldName(val))
				add(k)
			}
			w("default:")
			w("return %s, fmt.Errorf(%q, %s)", zero, q.Name+": "+p.Name+" %q is not one of "+strings.Join(p.Type.Choices, ", "), x)
			w("}")
		case !multi || axis < 0:
		case p.Optional:
			w("if %s != nil {", x)
			add(1)
			w("}")
		case p.Type.Slice:
			w("if len(%s) > 0 {", x)
			add(1)
			w("}")
		default: // bool
			w("if %s {", x)
			add(1)
			w("}")
		}
	}
	call := func(query, args string) {
		switch q.Kind {
		case One:
			w("return rt.One(ctx, db, scan%s, %s%s)", q.Name, query, args)
		case Many:
			w("return rt.Many(ctx, db, scan%s, %s%s)", q.Name, query, args)
		case Exec:
			w("return rt.Exec(ctx, db, %s%s)", query, args)
		}
	}
	if v := c.Variants[0]; !multi && v.Slices == nil {
		var args string
		for _, a := range g.scalars(q, v) {
			args += ", " + a
		}
		call(g.sql(v), args)
		return b.String()
	}
	w("var query string")
	w("var args []any")
	if multi {
		w("switch key {")
	}
	for _, v := range c.Variants {
		if multi {
			var keys []string
			for _, combo := range v.Combos {
				k := 0
				for i, x := range combo {
					k += x * weights[i]
				}
				keys = append(keys, strconv.Itoa(k))
			}
			w("case %s:", strings.Join(keys, ", "))
		}
		for _, s := range v.Slices {
			g.imports["fmt"] = true
			w("if len(p.%s) == 0 {", FieldName(s))
			w("return %s, fmt.Errorf(%q, maago.ErrEmptySlice)", zero, q.Name+": "+s+": %w")
			w("}")
		}
		w("query = %s", g.sql(v))
		if args := g.scalars(q, v); args != nil {
			w("args = []any{%s}", strings.Join(args, ", "))
		}
		for _, s := range v.Slices {
			w("for _, x := range p.%s {", FieldName(s))
			w("args = append(args, %s)", g.arg("x", q.param(s).Type.Base, false))
			w("}")
		}
	}
	if multi {
		w("}")
	}
	call("query", ", args...")
	return b.String()
}

// sql is a Go expression of v's text: constant but for the placeholders
// of the slices, numbered after the scalars and the slices before.
func (g *gen) sql(v Variant) string {
	mark := "$"
	if g.d == project.SQLite {
		mark = "?"
	}
	var parts []string
	var s strings.Builder
	for _, p := range v.Pieces {
		switch {
		case p.Slice != "":
			if s.Len() > 0 {
				parts = append(parts, GoString(s.String()))
				s.Reset()
			}
			first := strconv.Itoa(len(v.Args) + 1)
			for _, prev := range v.Slices[:slices.Index(v.Slices, p.Slice)] {
				first += "+len(p." + FieldName(prev) + ")"
			}
			parts = append(parts, fmt.Sprintf("rt.Placeholders(%q, %s, len(p.%s))", mark, first, FieldName(p.Slice)))
		case p.Arg > 0:
			s.WriteString(mark + strconv.Itoa(p.Arg))
		default:
			s.WriteString(p.SQL)
		}
	}
	if s.Len() > 0 {
		parts = append(parts, GoString(s.String()))
	}
	return strings.Join(parts, " + ")
}

// scalars are the Go expressions of v's scalar arguments, in placeholder
// order. An optional parameter is present on every line that uses it.
func (g *gen) scalars(q *Query, v Variant) []string {
	var out []string
	for _, name := range v.Args {
		p := q.param(name)
		x := "p." + FieldName(name)
		switch {
		case p.Type.Choices != nil:
			x = "string(" + x + ")"
		case p.Optional:
			x = "*" + x
		}
		out = append(out, g.arg(x, p.Type.Base, false))
	}
	return out
}

func (g *gen) typ(t model.Type, pointer bool) string {
	if t == model.Time {
		g.imports["time"] = true
	}
	return fieldType(t, pointer)
}

// arg is x as the driver takes it: SQLite keeps a time as text (D-105).
func (g *gen) arg(x string, t model.Type, pointer bool) string {
	switch {
	case g.d != project.SQLite || t != model.Time:
		return x
	case pointer:
		return "rt.SQLiteNullTime(" + x + ")"
	}
	return "rt.SQLiteTime(" + x + ")"
}

// dest is where Scan puts a column into x.
func (g *gen) dest(x string, t model.Type, pointer bool) string {
	switch {
	case g.d != project.SQLite || t != model.Time:
		return "&" + x
	case pointer:
		return "rt.ScanNullSQLiteTime(&" + x + ")"
	}
	return "rt.ScanSQLiteTime(&" + x + ")"
}

// GoString is a Go literal of s: a raw string, which keeps SQL readable,
// unless s has what a raw string cannot hold.
func GoString(s string) string {
	if strings.ContainsAny(s, "`\r") {
		return strconv.Quote(s)
	}
	return "`" + s + "`"
}
