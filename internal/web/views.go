package web

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/model"
)

// view is a route's view as Views reads it from the syntax: what the
// generated adapter and URL function need before the types are known.
type view struct {
	escape    bool      // func(http.ResponseWriter, *http.Request)
	in        string    // the name of In's type, "" for struct{}
	fields    []inField // In's fields
	wildcards []param   // the path's wildcards in order: the URL function's parameters

	// Types fills these.
	result   bool   // returns maago.Result[page]
	page     string // the page type of an HTML route
	template string // its template file
}

// inField is a field of In and where its value comes from.
type inField struct {
	name, source, key string // source is "path", "query" or "form"
	typ               string // string, int64, int32, bool or float64; a form's type name
	pointer           bool   // optional: nil when the query lacks it
	pos               token.Pos
}

type param struct{ name, typ string }

var (
	pathTypes  = []string{"string", "int64", "int32"}
	queryTypes = []string{"string", "int64", "int32", "bool", "float64"}
	inKeys     = []string{"path", "query", "form"}
)

// Views checks the shape of each route's view in the syntax: the forms of
// D-116, a named struct or struct{} for In, and In's fields, each from
// one path wildcard or one query key. The types confirm it later; this is
// what the URL functions need before the types can be checked, since the
// views call them. It runs on tables Check accepted.
func Views(fset *token.FileSet, apps []*App) []diag.Diagnostic {
	var ds []diag.Diagnostic
	for _, app := range apps {
		for i := range app.Routes {
			r := &app.Routes[i]
			v, vds := readView(fset, app, r)
			ds = append(ds, vds...)
			r.view = v
		}
	}
	return ds
}

func readView(fset *token.FileSet, app *App, r *Route) (*view, []diag.Diagnostic) {
	var ds []diag.Diagnostic
	errAt := func(pos token.Pos, msg, fix string) { ds = append(ds, diag.New(fset.Position(pos), msg, fix)) }
	fd := app.funcs[r.View]
	if fd == nil {
		return nil, nil // Check reported it
	}
	const shapes = "write func(ctx context.Context, in In) (Page, error), func(ctx context.Context, in In) (maago.Result[Page], error), or func(w http.ResponseWriter, r *http.Request)"
	params, results := fields(fd.Type.Params), fields(fd.Type.Results)
	wildcards, wds := pathWildcards(r)
	for _, w := range wds {
		errAt(r.PathPos, w, "split the route, or read the rest of the path with a func(w, r) view")
	}
	switch {
	case fd.Type.TypeParams != nil:
		errAt(fd.Name.Pos(), "view "+r.View+" has type parameters", shapes)
		return nil, ds
	case len(params) == 2 && len(results) == 0:
		v := &view{escape: true}
		for _, w := range wildcards {
			v.wildcards = append(v.wildcards, param{w, "string"})
		}
		return v, ds
	case len(params) != 2 || len(results) != 2:
		errAt(fd.Name.Pos(), fmt.Sprintf("view %s takes %d values and returns %d", r.View, len(params), len(results)), shapes)
		return nil, ds
	}
	v := &view{}
	var st *ast.StructType
	switch t := params[1].(type) {
	case *ast.StructType:
		st = t
	case *ast.Ident:
		if ts := app.types[t.Name]; ts != nil && ts.TypeParams == nil {
			st, _ = ts.Type.(*ast.StructType)
			v.in = t.Name
		}
	}
	if st == nil || v.in == "" && len(st.Fields.List) > 0 {
		errAt(params[1].Pos(), "the In of view "+r.View+" is not struct{} or a struct type named in package "+app.Name,
			"declare type "+r.View+"In struct{…} in package "+app.Name+" with path= and query= fields, or take struct{}")
		return nil, ds
	}
	var bad []inField // path fields of a type Views rejected
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			errAt(f.Pos(), "In of view "+r.View+" embeds a field", "name each field, tagged maago:\"path=…\" or maago:\"query=…\"")
			continue
		}
		for _, name := range f.Names {
			field, ok := readField(fset, f, name, &ds)
			switch {
			case ok:
				v.fields = append(v.fields, field)
			case field.source == "path": // a wrong type: the wildcard is still read
				bad = append(bad, field)
			}
		}
	}
	if forms := slices.DeleteFunc(slices.Clone(v.fields), func(f inField) bool { return f.source != "form" }); len(forms) > 1 {
		errAt(forms[1].pos, fmt.Sprintf("%s reads a second form, %s", v.in, forms[1].name), "keep one form field in In; a page with two forms posts each to its own route")
	}
	paths := map[string]inField{}
	for _, f := range append(v.fields, bad...) {
		if f.source != "path" {
			continue
		}
		if _, twice := paths[f.key]; twice {
			errAt(f.pos, fmt.Sprintf("two fields of %s read path value %q", v.in, f.key), "keep one")
		}
		paths[f.key] = f
		if !slices.Contains(wildcards, f.key) {
			errAt(r.PathPos, fmt.Sprintf("route %s has no {%s}, which %s.%s reads", r.Name, f.key, v.in, f.name),
				"add {"+f.key+"} to the path, or remove the field")
		}
	}
	for _, w := range wildcards {
		f, ok := paths[w]
		if ok && slices.Contains(bad, f) {
			continue // reported with its type
		}
		if !ok {
			errAt(r.PathPos, fmt.Sprintf("view %s reads no {%s} of route %s", r.View, w, r.Name),
				fmt.Sprintf("add a field tagged maago:\"path=%s\" to the In of %s", w, r.View))
			continue
		}
		v.wildcards = append(v.wildcards, param{w, f.typ})
	}
	return v, ds
}

// readField reads one In field: its tag and its type.
func readField(fset *token.FileSet, f *ast.Field, name *ast.Ident, ds *[]diag.Diagnostic) (inField, bool) {
	errAt := func(pos token.Pos, msg, fix string) { *ds = append(*ds, diag.New(fset.Position(pos), msg, fix)) }
	opts, tds := model.ParseTag(fset, f.Tag)
	*ds = append(*ds, tds...)
	if len(tds) > 0 {
		return inField{}, false
	}
	if len(opts) > 1 {
		errAt(opts[1].KeyPos, "field "+name.Name+" names more than one source",
			`keep one: maago:"path=name", maago:"query=name" or maago:"form"`)
		return inField{}, false
	}
	if len(opts) == 1 && opts[0].Key == "form" {
		return readFormField(fset, f, name, opts[0], ds)
	}
	if len(opts) != 1 || opts[0].Kind != model.Bare || len(opts[0].Values) != 1 {
		errAt(name.Pos(), "field "+name.Name+" says not where its value comes from",
			`tag it with one source: maago:"path=name", maago:"query=name" or maago:"form"`)
		return inField{}, false
	}
	o := opts[0]
	if !slices.Contains(inKeys, o.Key) {
		fix := "allowed: " + strings.Join(inKeys, ", ")
		if m := diag.Suggest(o.Key, inKeys); m != "" {
			fix = fmt.Sprintf("did you mean %q? %s", m, fix)
		}
		errAt(o.KeyPos, fmt.Sprintf("unknown key %q in the tag of %s", o.Key, name.Name), fix)
		return inField{}, false
	}
	field := inField{name: name.Name, source: o.Key, key: o.Values[0], pos: name.Pos()}
	t := f.Type
	if s, ok := t.(*ast.StarExpr); ok && o.Key == "query" {
		field.pointer, t = true, s.X
	}
	id, _ := t.(*ast.Ident)
	allowed := map[string][]string{"path": pathTypes, "query": queryTypes}[o.Key]
	if id == nil || !slices.Contains(allowed, id.Name) {
		fix := "use one of " + strings.Join(allowed, ", ")
		if o.Key == "query" {
			fix += ", or a pointer to one when the value may be missing"
		}
		errAt(f.Type.Pos(), fmt.Sprintf("%s value %s cannot be read into %s", o.Key, name.Name, types.ExprString(f.Type)), fix)
		return field, false
	}
	field.typ = id.Name
	return field, true
}

// pathWildcards are the names of the {name} segments of r's path; {$}
// is none. A {name...} wildcard is an error.
func pathWildcards(r *Route) (names, errs []string) {
	for _, seg := range strings.Split(r.Path, "/") {
		switch {
		case seg == "{$}" || !strings.HasPrefix(seg, "{"):
		case strings.HasSuffix(seg, "...}"):
			errs = append(errs, fmt.Sprintf("route %s reads the rest of the path with %s, which v0 does not support", r.Name, seg))
			names = append(names, strings.TrimSuffix(strings.Trim(seg, "{}"), "...")) // its field is not an error too
		default:
			names = append(names, strings.Trim(seg, "{}"))
		}
	}
	return names, errs
}

// fields lists the types of a parameter or result list, one per name.
func fields(fl *ast.FieldList) []ast.Expr {
	var ts []ast.Expr
	if fl != nil {
		for _, f := range fl.List {
			for range max(len(f.Names), 1) {
				ts = append(ts, f.Type)
			}
		}
	}
	return ts
}
