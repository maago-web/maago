package model

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/maago-web/maago/internal/diag"
)

// Type is a column type maago supports (D-77).
type Type int

const (
	Int64 Type = iota + 1
	Int32
	Bool
	Float64
	String
	Time
	Bytes
)

func (t Type) String() string {
	return [...]string{"", "int64", "int32", "bool", "float64", "string", "time.Time", "[]byte"}[t]
}

// App is a package holding models, checked.
type App struct {
	Dir, Name string
	Models    []*Model
}

// Model is a checked model: everything the DDL needs, and nothing left to
// interpret.
type Model struct {
	Name        string
	Pos         token.Pos
	Table       string
	Ordering    []Order
	Uniques     [][]string // column names; from unique on fields and models
	Indexes     [][]string // column names; from index on fields and models, and FKs (D-85)
	Fields      []*Field   // columns, in declaration order; ID is one of them
	Unsupported []Unsupported

	// The admin's options (D-132), as written: Go field names. An empty
	// AdminList leaves the admin its default columns; empty AdminSearch
	// and AdminFilter mean no search box and no filters.
	AdminHidden                         bool
	AdminList, AdminSearch, AdminFilter []string
}

type Order struct {
	Field string
	Desc  bool
}

// Field is a column.
type Field struct {
	Name        string
	Column      string // snake_case of Name (D-76)
	Pos         token.Pos
	Type        Type
	Pointer     bool    // NULL allowed (D-75)
	Max         int     // 0 if none
	Default     *string // as written, quotes removed
	Choices     []string
	Label, Help string
	FK          string // the table of the model it refers to (D-124)
	Auto        string // "create" or "update": generated CRUD fills it with the time (D-102)
	Admin       string // "hidden" or "readonly" in the admin (D-132), or ""
}

// Unsupported is a field that is not a column, and why (D-18, D-82).
type Unsupported struct {
	Name   string
	Pos    token.Pos
	Reason string
}

var (
	modelKeys = []string{"table", "ordering", "unique", "index", "admin", "admin_list", "admin_search", "admin_filter"}
	fieldKeys = []string{"max", "unique", "index", "default", "choices", "label", "help", "fk", "auto", "admin"}
	tableName = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
)

// Check gives the options of each model their meaning and reports every
// independent error.
func Check(fset *token.FileSet, pkgs []*Package) ([]*App, []diag.Diagnostic) {
	c := &checker{fset: fset}
	var apps []*App
	for _, pkg := range pkgs {
		app := &App{Dir: pkg.Dir, Name: pkg.Name}
		for _, rm := range pkg.Models {
			app.Models = append(app.Models, c.model(app, rm))
		}
		apps = append(apps, app)
	}
	c.resolve(apps)
	c.tables(apps)
	return apps, c.ds
}

type checker struct {
	fset      *token.FileSet
	ds        []diag.Diagnostic
	uniq, idx []decl // of the model being checked
	refs      []ref  // every fk=, resolved once all apps are known
}

// ref is an fk= as written: Model, or app.Model (D-124).
type ref struct {
	f      *Field // nil when the field is not int64: only the name is checked
	app    *App   // the app the field is in
	target string
	where  string
	pos    token.Pos
}

// decl is one unique or index as written, on a field or a model.
type decl struct {
	fields []string
	pos    token.Pos
}

func (c *checker) errf(pos token.Pos, fix, format string, args ...any) {
	c.ds = append(c.ds, diag.New(c.fset.Position(pos), fmt.Sprintf(format, args...), fix))
}

func (c *checker) model(app *App, rm *RawModel) *Model {
	m := &Model{Name: rm.Name, Pos: rm.Pos, Table: Snake(app.Name) + "_" + Snake(rm.Name)}
	c.uniq, c.idx = nil, nil
	hasID := false
	for _, rf := range rm.Fields {
		where := rm.Name + "." + rf.Name
		typ, ptr, why := fieldType(rf.Type, rf.TimeName)
		switch {
		case rf.Name == "ID" && !rf.Embedded:
			hasID = true
			if typ != Int64 || ptr {
				c.errf(rf.Pos, "declare it as ID int64 (D-74)", "%s is the primary key and must be int64, not %s", where, types.ExprString(rf.Type))
				c.fieldOptions(&Field{Name: rf.Name}, rf.Options, where, app) // its options are wrong or right regardless
				continue
			}
		case rf.Embedded:
			m.Unsupported = append(m.Unsupported, c.unsupported(rf, where, "embedded field"))
			continue
		case !ast.IsExported(rf.Name):
			m.Unsupported = append(m.Unsupported, c.unsupported(rf, where, "unexported field"))
			continue
		case why != "":
			m.Unsupported = append(m.Unsupported, c.unsupported(rf, where, why))
			continue
		}
		f := &Field{Name: rf.Name, Column: Snake(rf.Name), Pos: rf.Pos, Type: typ, Pointer: ptr}
		c.fieldOptions(f, rf.Options, where, app)
		m.Fields = append(m.Fields, f)
	}
	if !hasID {
		c.errf(rm.Pos, "add the field ID int64 (D-74)", "model %s has no primary key", rm.Name)
	}
	c.modelOptions(m, rm.Options)
	c.constraints(m)
	return m
}

// constraints turns the declared uniques and indexes into column lists,
// adds an index for each FK (D-85), and reports what is declared twice and
// fields that end up with the same column name.
func (c *checker) constraints(m *Model) {
	column := map[string]string{}
	owner := map[string]string{}
	for _, f := range m.Fields {
		column[f.Name] = f.Column
		if prev, ok := owner[f.Column]; ok {
			c.errf(f.Pos, "rename one of them", "fields %s and %s of %s both become column %q", prev, f.Name, m.Name, f.Column)
			continue
		}
		owner[f.Column] = f.Name
	}
	seen := map[string]bool{}
	collect := func(decls []decl, what string) [][]string {
		var out [][]string
		for _, d := range decls {
			var cols []string
			for _, f := range d.fields {
				cols = append(cols, column[f])
			}
			// A unique is a set of columns; an index is ordered, since the
			// order changes what it speeds up.
			keyCols := cols
			if what == "unique" {
				keyCols = slices.Sorted(slices.Values(cols))
			}
			key := what + ":" + strings.Join(keyCols, ",")
			if seen[key] {
				c.errf(d.pos, "remove one of them", "%s on %s is declared twice in %s", what, strings.Join(d.fields, ", "), m.Name)
				continue
			}
			seen[key] = true
			out = append(out, cols)
		}
		return out
	}
	m.Uniques = collect(c.uniq, "unique")
	m.Indexes = collect(c.idx, "index")
	for _, f := range m.Fields {
		if f.FK != "" && !seen["unique:"+f.Column] && !seen["index:"+f.Column] {
			m.Indexes = append(m.Indexes, []string{f.Column})
			seen["index:"+f.Column] = true
		}
	}
}

// unsupported records a field that is not a column. Options on it would be
// silently ignored, so they are an error.
func (c *checker) unsupported(rf *RawField, where, why string) Unsupported {
	if len(rf.Options) > 0 {
		c.errf(rf.Options[0].KeyPos, "remove the maago tag, or make the field a column: exported, of type "+supportedTypes,
			"%s is not a column (%s), so its maago options would be ignored", where, why)
	}
	return Unsupported{Name: rf.Name, Pos: rf.Pos, Reason: why}
}

// fieldType maps a field's type to a column type, or says why it has none.
func fieldType(e ast.Expr, timeName string) (t Type, ptr bool, why string) {
	e = ast.Unparen(e)
	if star, ok := e.(*ast.StarExpr); ok {
		ptr, e = true, ast.Unparen(star.X)
	}
	switch x := e.(type) {
	case *ast.Ident:
		if i := slices.Index([]string{"int64", "int32", "bool", "float64", "string"}, x.Name); i >= 0 {
			return Type(i + 1), ptr, ""
		}
	case *ast.SelectorExpr:
		if pkg, ok := x.X.(*ast.Ident); ok && timeName != "" && pkg.Name == timeName && x.Sel.Name == "Time" {
			return Time, ptr, ""
		}
	case *ast.ArrayType:
		if elt, ok := x.Elt.(*ast.Ident); ok && x.Len == nil && elt.Name == "byte" {
			return Bytes, ptr, ""
		}
	}
	return 0, false, "type " + types.ExprString(e) + " is not supported"
}

const supportedTypes = "int64, int32, bool, float64, string, time.Time, []byte, or a pointer to one (D-77)"

func (c *checker) fieldOptions(f *Field, opts []Option, where string, app *App) {
	seen := map[string]bool{}
	var defaultOpt *Option
	for i := range opts {
		o := &opts[i]
		if !slices.Contains(fieldKeys, o.Key) {
			c.unknown(o.KeyPos, "tag key", o.Key, where, fieldKeys)
			continue
		}
		if seen[o.Key] {
			c.errf(o.KeyPos, "keep one of them", "tag key %q given twice in %s", o.Key, where)
			continue
		}
		seen[o.Key] = true
		if f.Name == "ID" && o.Key != "label" && o.Key != "help" {
			c.errf(o.KeyPos, "remove it; ID may only have label and help", "%s is the primary key and cannot have %q", where, o.Key)
			continue
		}
		switch o.Key {
		case "max":
			n, err := strconv.Atoi(first(o))
			switch {
			case !c.shape(o, where, Bare, 1, "write a number, like max=200"):
			case err != nil || n <= 0:
				c.errf(o.ValuePos[0], "use a positive whole number, like max=200", "max in %s is %q, not a positive whole number", where, first(o))
			case f.Type != String:
				c.errf(o.KeyPos, "remove max, or make the field a string", "max applies to strings, but %s is %s", where, f.Type)
			default:
				f.Max = n
			}
		case "unique", "index":
			if c.shape(o, where, NoValue, 0, "write it alone, like "+o.Key) {
				d := decl{[]string{f.Name}, o.KeyPos}
				if o.Key == "unique" {
					c.uniq = append(c.uniq, d)
				} else {
					c.idx = append(c.idx, d)
				}
			}
		case "label", "help":
			if c.shape(o, where, Text, 1, fmt.Sprintf("write text in quotes, like %s='Title'", o.Key)) {
				if o.Key == "label" {
					f.Label = o.Values[0]
				} else {
					f.Help = o.Values[0]
				}
			}
		case "choices":
			switch {
			case !c.shape(o, where, Bare, -1, "list them with /, like choices=draft/published"):
			case f.Type != String:
				c.errf(o.KeyPos, "remove choices, or make the field a string", "choices apply to strings, but %s is %s", where, f.Type)
			default:
				for j, v := range o.Values {
					if slices.Contains(o.Values[:j], v) {
						c.errf(o.ValuePos[j], "remove the repeated choice", "choice %q appears twice in %s", v, where)
					}
				}
				f.Choices = o.Values
			}
		case "fk":
			const fix = "name a model, like fk=Post or fk=auth.User"
			if !c.shape(o, where, Bare, 1, fix) {
				break
			}
			if a, name, dotted := strings.Cut(first(o), "."); dotted && (a == "" || name == "" || strings.Contains(name, ".")) {
				c.errf(o.KeyPos, fix, "fk in %s is not written as expected", where)
				break
			}
			r := ref{app: app, target: first(o), where: where, pos: o.ValuePos[0]}
			if f.Type != Int64 {
				c.errf(o.KeyPos, "declare the field as int64, or *int64 to allow NULL", "%s refers to %s by ID, so it must be int64, not %s", where, first(o), f.Type)
			} else {
				r.f, f.FK = f, first(o) // resolve sets the table; constraints needs it set now
			}
			c.refs = append(c.refs, r)
		case "default":
			defaultOpt = o
		case "auto":
			switch {
			case !c.shape(o, where, Bare, 1, "write auto=create or auto=update"):
			case first(o) != "create" && first(o) != "update":
				c.unknown(o.ValuePos[0], "auto value", first(o), where, []string{"create", "update"})
			case f.Type != Time || f.Pointer:
				typ := f.Type.String()
				if f.Pointer {
					typ = "*" + typ
				}
				c.errf(o.KeyPos, "declare the field as time.Time, or remove auto", "auto fills a time.Time, but %s is %s", where, typ)
			default:
				f.Auto = first(o)
			}
		case "admin":
			switch {
			case !c.shape(o, where, Bare, 1, "write admin=hidden or admin=readonly"):
			case first(o) != "hidden" && first(o) != "readonly":
				c.unknown(o.ValuePos[0], "admin value", first(o), where, []string{"hidden", "readonly"})
			default:
				f.Admin = first(o)
			}
		}
	}
	if defaultOpt != nil {
		c.defaultValue(f, defaultOpt, where)
	}
}

// shape reports whether o has the expected kind and number of values
// (n < 0: one or more), and reports an error if not.
func (c *checker) shape(o *Option, where string, k Kind, n int, fix string) bool {
	if o.Kind == k && (n < 0 && len(o.Values) > 0 || len(o.Values) == n) {
		return true
	}
	c.errf(o.KeyPos, fix, "%s in %s is not written as expected", o.Key, where)
	return false
}

func first(o *Option) string {
	if len(o.Values) == 0 {
		return ""
	}
	return o.Values[0]
}

func (c *checker) defaultValue(f *Field, o *Option, where string) {
	v := first(o)
	var err error
	switch f.Type {
	case String:
		if o.Kind != Text || len(o.Values) != 1 {
			c.errf(o.KeyPos, "write text in quotes, like default='draft'", "default of %s must be quoted text", where)
			return
		}
	case Int64, Int32, Bool, Float64:
		if !c.shape(o, where, Bare, 1, "write the value without quotes, like default=0") {
			return
		}
		switch f.Type {
		case Int64:
			_, err = strconv.ParseInt(v, 10, 64)
		case Int32:
			_, err = strconv.ParseInt(v, 10, 32)
		case Bool:
			if v != "true" && v != "false" {
				err = strconv.ErrSyntax
			}
		case Float64:
			var x float64
			x, err = strconv.ParseFloat(v, 64)
			if err == nil && (math.IsInf(x, 0) || math.IsNaN(x)) {
				err = strconv.ErrRange
			}
		}
		if err != nil {
			c.errf(o.ValuePos[0], "write a valid "+f.Type.String()+" value", "default %q of %s is not a valid %s", v, where, f.Type)
			return
		}
	default:
		c.errf(o.KeyPos, "remove the default", "%s is %s, which cannot have a default in v0", where, f.Type)
		return
	}
	if len(f.Choices) > 0 && !slices.Contains(f.Choices, v) {
		c.errf(o.ValuePos[0], "use one of: "+strings.Join(f.Choices, ", "), "default %q of %s is not one of its choices", v, where)
		return
	}
	f.Default = &v
}

func (c *checker) modelOptions(m *Model, opts []Option) {
	var columns []string
	for _, f := range m.Fields {
		columns = append(columns, f.Name)
	}
	seen := map[string]bool{}
	for i := range opts {
		o := &opts[i]
		if !slices.Contains(modelKeys, o.Key) {
			c.unknown(o.KeyPos, "tag key", o.Key, m.Name, modelKeys)
			continue
		}
		if o.Key != "unique" && o.Key != "index" && seen[o.Key] {
			c.errf(o.KeyPos, "keep one of them", "tag key %q given twice in %s", o.Key, m.Name)
			continue
		}
		seen[o.Key] = true
		switch o.Key {
		case "table":
			switch {
			case !c.shape(o, m.Name, Bare, 1, "name the table, like table=posts"):
			case !tableName.MatchString(first(o)):
				c.errf(o.ValuePos[0], "use lowercase letters, digits and _, like table=blog_posts", "table name %q of %s is not a plain SQL name", first(o), m.Name)
			default:
				m.Table = first(o)
			}
		case "ordering":
			if (o.Kind != Bare || len(o.Values) != 1) && o.Kind != Group {
				c.errf(o.KeyPos, "write ordering=-PublishedAt, or ordering=(-PublishedAt,Title)", "ordering in %s is not written as expected", m.Name)
				continue
			}
			for j, v := range o.Values {
				name, desc := strings.CutPrefix(v, "-")
				if c.item(o, j, name, m.Name, columns) {
					m.Ordering = append(m.Ordering, Order{Field: name, Desc: desc})
				}
			}
		case "unique", "index":
			if !c.shape(o, m.Name, Group, -1, "list fields in parentheses, like "+o.Key+"=(AuthorID,Title)") {
				continue
			}
			ok := true
			for j, v := range o.Values {
				ok = c.item(o, j, v, m.Name, columns) && ok
			}
			if !ok {
				continue
			}
			if o.Key == "unique" {
				c.uniq = append(c.uniq, decl{o.Values, o.KeyPos})
			} else {
				c.idx = append(c.idx, decl{o.Values, o.KeyPos})
			}
		case "admin":
			switch {
			case !c.shape(o, m.Name, Bare, 1, "write admin=hidden"):
			case first(o) != "hidden":
				c.unknown(o.ValuePos[0], "admin value", first(o), m.Name, []string{"hidden"})
			default:
				m.AdminHidden = true
			}
		case "admin_list", "admin_search", "admin_filter":
			if !c.shape(o, m.Name, Group, -1, "list fields in parentheses, like "+o.Key+"=(Title)") {
				continue
			}
			var names []string
			for j, v := range o.Values {
				if c.item(o, j, v, m.Name, columns) && c.adminItem(o, j, m.Field(v), m.Name) {
					names = append(names, v)
				}
			}
			switch o.Key {
			case "admin_list":
				m.AdminList = names
			case "admin_search":
				m.AdminSearch = names
			default:
				m.AdminFilter = names
			}
		}
	}
}

// adminItem checks that f fits the admin option o: not hidden, a string
// to search, and a field with a few values to filter by. Each is
// reported on its own (D-61).
func (c *checker) adminItem(o *Option, j int, f *Field, model string) bool {
	where, ok := model+"."+f.Name, true
	if f.Admin == "hidden" {
		c.errf(o.ValuePos[j], "remove it from "+o.Key+", or remove admin=hidden", "%s is admin=hidden, but %s of %s lists it", where, o.Key, model)
		ok = false
	}
	if o.Key == "admin_search" && f.Type != String {
		c.errf(o.ValuePos[j], "remove it from admin_search, or make the field a string", "admin_search applies to strings, but %s is %s", where, f.Type)
		ok = false
	}
	if o.Key == "admin_filter" && len(f.Choices) == 0 && f.Type != Bool && f.FK == "" {
		c.errf(o.ValuePos[j], "filter by a field with choices, a bool, or an fk field", "%s in admin_filter has no choices, and is neither a bool nor an fk field", where)
		ok = false
	}
	if k := Snake(f.Name); o.Key == "admin_filter" && (k == "page" || k == "q") {
		c.errf(o.ValuePos[j], "rename the field, or filter by another", "%s in admin_filter would be the list's key %s, which the page or the search has", where, k)
		ok = false
	}
	return ok
}

// Field is the column field named name, which exists.
func (m *Model) Field(name string) *Field {
	for _, f := range m.Fields {
		if f.Name == name {
			return f
		}
	}
	panic("model: no field " + name)
}

// item checks one field name in ordering, unique or index.
func (c *checker) item(o *Option, j int, name, model string, columns []string) bool {
	if !slices.Contains(columns, name) {
		c.unknown(o.ValuePos[j], "column field", name, model, columns)
		return false
	}
	for _, prev := range o.Values[:j] {
		if strings.TrimPrefix(prev, "-") == name {
			c.errf(o.ValuePos[j], "remove the repeated field", "field %s appears twice in %s of %s", name, o.Key, model)
			return false
		}
	}
	return true
}

// unknown reports a name that is not among the allowed ones, suggesting
// the closest.
func (c *checker) unknown(pos token.Pos, what, got, where string, allowed []string) {
	fix := "allowed: " + strings.Join(allowed, ", ")
	if s := diag.Suggest(got, allowed); s != "" {
		fix = fmt.Sprintf("did you mean %q? %s", s, fix)
	}
	c.errf(pos, fix, "unknown %s %q in %s", what, got, where)
}

// resolve turns each fk= into the table of the model it names: a model of
// the field's own app, or app.Model with app a package name (D-124). An
// app name two packages share is an error only where an fk= uses it. A
// name that does not resolve leaves the field without an FK.
func (c *checker) resolve(apps []*App) {
	byName := map[string][]*App{}
	var names []string
	for _, a := range apps {
		if byName[a.Name] == nil {
			names = append(names, a.Name)
		}
		byName[a.Name] = append(byName[a.Name], a)
	}
	for _, r := range c.refs {
		table := c.lookup(r, byName, names)
		if r.f != nil {
			r.f.FK = table
		}
	}
}

// lookup is the table r names, or "" after reporting why there is none.
func (c *checker) lookup(r ref, byName map[string][]*App, names []string) string {
	app, name, prefix := r.app, r.target, ""
	if a, n, dotted := strings.Cut(r.target, "."); dotted {
		switch cands := byName[a]; len(cands) {
		case 0:
			c.unknown(r.pos, "app", a, r.where, names)
			return ""
		case 1:
			app, name, prefix = cands[0], n, a+"."
		default:
			var dirs []string
			for _, cand := range cands {
				dirs = append(dirs, cand.Dir+"/")
			}
			c.errf(r.pos, "rename one of the packages", "app %q in %s is ambiguous: %s", a, r.where, strings.Join(dirs, ", "))
			return ""
		}
	}
	var models []string
	for _, m := range app.Models {
		if m.Name == name {
			return m.Table
		}
		models = append(models, prefix+m.Name)
	}
	c.unknown(r.pos, "model", r.target, r.where, models)
	return ""
}

// tables reports table names used by more than one model, across apps.
func (c *checker) tables(apps []*App) {
	owner := map[string]string{}
	for _, app := range apps {
		for _, m := range app.Models {
			where := app.Dir + ": " + m.Name
			if prev, ok := owner[m.Table]; ok {
				c.errf(m.Pos, "give one of them another name with table=…", "table %q of %s is also the table of %s", m.Table, where, prev)
				continue
			}
			owner[m.Table] = where
		}
	}
}
