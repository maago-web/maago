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
	"unicode/utf8"

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

// Model is a checked model: everything the schema needs, and nothing left
// to interpret.
type Model struct {
	Name        string
	Pos         token.Pos
	Table       string
	Ordering    []Order
	Uniques     [][]string // field names
	Indexes     [][]string
	Fields      []*Field // columns, in declaration order; ID is one of them
	Unsupported []Unsupported
}

type Order struct {
	Field string
	Desc  bool
}

// Field is a column.
type Field struct {
	Name          string
	Column        string // snake_case of Name (D-76)
	Pos           token.Pos
	Type          Type
	Pointer       bool // NULL allowed (D-75)
	Max           int  // 0 if none
	Unique, Index bool
	Default       *string // as written, quotes removed
	Choices       []string
	Label, Help   string
	FK            string // model name in the same app
}

// Unsupported is a field that is not a column, and why (D-18, D-82).
type Unsupported struct {
	Name   string
	Pos    token.Pos
	Reason string
}

var (
	modelKeys = []string{"table", "ordering", "unique", "index"}
	fieldKeys = []string{"max", "unique", "index", "default", "choices", "label", "help", "fk"}
	tableName = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
)

// Check gives the options of each model their meaning and reports every
// independent error.
func Check(fset *token.FileSet, pkgs []*Package) ([]*App, []diag.Diagnostic) {
	c := &checker{fset: fset}
	var apps []*App
	for _, pkg := range pkgs {
		app := &App{Dir: pkg.Dir, Name: pkg.Name}
		var names []string
		for _, rm := range pkg.Models {
			names = append(names, rm.Name)
		}
		for _, rm := range pkg.Models {
			app.Models = append(app.Models, c.model(pkg.Name, rm, names))
		}
		apps = append(apps, app)
	}
	c.tables(apps)
	return apps, c.ds
}

type checker struct {
	fset *token.FileSet
	ds   []diag.Diagnostic
}

func (c *checker) errf(pos token.Pos, fix, format string, args ...any) {
	c.ds = append(c.ds, diag.New(c.fset.Position(pos), fmt.Sprintf(format, args...), fix))
}

func (c *checker) model(app string, rm *RawModel, models []string) *Model {
	m := &Model{Name: rm.Name, Pos: rm.Pos, Table: snake(app) + "_" + snake(rm.Name)}
	hasID := false
	for _, rf := range rm.Fields {
		where := rm.Name + "." + rf.Name
		typ, ptr, why := fieldType(rf.Type, rf.TimeName)
		switch {
		case rf.Name == "ID" && !rf.Embedded:
			hasID = true
			if typ != Int64 || ptr {
				c.errf(rf.Pos, "declare it as ID int64 (D-74)", "%s is the primary key and must be int64, not %s", where, types.ExprString(rf.Type))
				c.fieldOptions(&Field{Name: rf.Name}, rf.Options, where, models) // its options are wrong or right regardless
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
		f := &Field{Name: rf.Name, Column: snake(rf.Name), Pos: rf.Pos, Type: typ, Pointer: ptr}
		c.fieldOptions(f, rf.Options, where, models)
		m.Fields = append(m.Fields, f)
	}
	if !hasID {
		c.errf(rm.Pos, "add the field ID int64 (D-74)", "model %s has no primary key", rm.Name)
	}
	c.modelOptions(m, rm.Options)
	return m
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

func (c *checker) fieldOptions(f *Field, opts []Option, where string, models []string) {
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
				f.Unique = f.Unique || o.Key == "unique"
				f.Index = f.Index || o.Key == "index"
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
			switch {
			case !c.shape(o, where, Bare, 1, "name a model, like fk=Post"):
			case !slices.Contains(models, first(o)):
				c.unknown(o.ValuePos[0], "model", first(o), where, models)
			case f.Type != Int64:
				c.errf(o.KeyPos, "declare the field as int64, or *int64 to allow NULL", "%s refers to %s by ID, so it must be int64, not %s", where, first(o), f.Type)
			default:
				f.FK = first(o)
			}
		case "default":
			defaultOpt = o
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
		if (o.Key == "table" || o.Key == "ordering") && seen[o.Key] {
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
				m.Uniques = append(m.Uniques, o.Values)
			} else {
				m.Indexes = append(m.Indexes, o.Values)
			}
		}
	}
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
	if s := suggest(got, allowed); s != "" {
		fix = fmt.Sprintf("did you mean %q? %s", s, fix)
	}
	c.errf(pos, fix, "unknown %s %q in %s", what, got, where)
}

// suggest returns the allowed name got most likely meant, or "". A name
// that got starts with, or that starts with got, wins (maxlen → max);
// otherwise the one within edit distance of a third of got's length,
// rounded up and counted in runes (tabel → table, but not Foo → On).
// Ties go to the earlier name in allowed.
func suggest(got string, allowed []string) string {
	lower := strings.ToLower(got)
	for _, a := range allowed {
		la := strings.ToLower(a)
		if strings.HasPrefix(lower, la) || strings.HasPrefix(la, lower) {
			return a
		}
	}
	best, bestDist := "", (utf8.RuneCountInString(got)+2)/3+1
	for _, a := range allowed {
		if d := distance(lower, strings.ToLower(a)); d < bestDist {
			best, bestDist = a, d
		}
	}
	return best
}

// distance is the Levenshtein distance between a and b, in runes.
func distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
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
