package web

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/model"
)

// view is a route's view as Views reads it from the syntax: what the
// generated adapter and URL function need before the types are known.
type view struct {
	escape    bool      // func(http.ResponseWriter, *http.Request)
	user      bool      // takes the user, between ctx and In (D-149)
	userPtr   bool      // as *U: optional, nil without one
	userExpr  string    // the user's type as the view writes it, for fixes
	db        bool      // takes maago.DB
	tx        bool      // takes maago.Tx: runs in a transaction (D-150)
	in        string    // the name of In's type, "" for struct{}
	fields    []inField // In's fields
	wildcards []param   // the path's wildcards in order: the URL function's parameters

	// Types fills these.
	userType   string // the user's type as the adapter writes it: User or maagouser.User
	userImport string // the import path maagouser names, "" for the view's own package
	result     bool   // returns maago.Result[page]
	page       string // the page type of an HTML route
	template   string // its template file
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
		ds = append(ds, loginOptions(fset, app)...)
		ds = append(ds, noTxOptions(fset, app)...)
	}
	return ds
}

// loginOptions checks maago.LoginRequired against the views that take the
// user, whose signature says it already (D-149).
func loginOptions(fset *token.FileSet, app *App) []diag.Diagnostic {
	var ds []diag.Diagnostic
	routes := map[string][]*Route{} // the routes of each view that takes the user
	for i := range app.Routes {
		if r := &app.Routes[i]; r.view != nil && r.view.user {
			routes[r.View] = append(routes[r.View], r)
		}
	}
	for _, view := range slices.Sorted(maps.Keys(routes)) {
		rs := routes[view]
		with := slices.DeleteFunc(slices.Clone(rs), func(r *Route) bool { return !hasOption(r.Options, "LoginRequired") })
		switch {
		case len(with) == 0:
		case len(with) < len(rs) && rs[0].view.userPtr:
			ds = append(ds, diag.New(fset.Position(optionPos(with[0], "LoginRequired")), fmt.Sprintf("route %s requires login, but view %s serves %s too, which does not", with[0].Name, view, rs[slices.IndexFunc(rs, func(r *Route) bool { return !hasOption(r.Options, "LoginRequired") })].Name),
				"split the view: the one without login takes u "+rs[0].view.userExpr+", the one with login u "+strings.TrimPrefix(rs[0].view.userExpr, "*")+", and neither route has maago.LoginRequired"))
		case rs[0].view.userPtr:
			ds = append(ds, diag.New(fset.Position(optionPos(with[0], "LoginRequired")), fmt.Sprintf("route %s requires login, but view %s takes the user as a pointer, which is for a user that may be missing", with[0].Name, view),
				"take u "+strings.TrimPrefix(rs[0].view.userExpr, "*")+", not "+rs[0].view.userExpr+", and remove maago.LoginRequired: the view's user requires login"))
		default:
			for _, r := range with {
				ds = append(ds, diag.New(fset.Position(optionPos(r, "LoginRequired")), fmt.Sprintf("route %s has maago.LoginRequired, but view %s takes the user, which requires login already", r.Name, view),
					"remove maago.LoginRequired"))
			}
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
	const shapes = "write func(ctx context.Context, [u User,] [db maago.DB | tx maago.Tx,] in In) (Page or maago.Result[Page], error), or func(w http.ResponseWriter, r *http.Request)"
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
	case len(params) < 2 || len(params) > 5 || len(results) != 2:
		errAt(fd.Name.Pos(), fmt.Sprintf("view %s takes %d values and returns %d", r.View, len(params), len(results)), shapes)
		return nil, ds
	}
	v := &view{}
	last := params[len(params)-1]
	in := last
	if star, ok := in.(*ast.StarExpr); ok {
		in = star.X
	}
	if id, ok := in.(*ast.Ident); ok && isForm(app, id.Name) {
		errAt(last.Pos(), "view "+r.View+" takes form "+id.Name+" as its In",
			"take it as a field: type "+r.View+"In struct{ Form "+id.Name+" `maago:\"form\"` }")
		return nil, ds
	}
	// Between ctx and In, the user, then the database (D-149).
	names := app.maago[r.View]
	var user ast.Expr
	order, wrong, both := false, "", false
	for _, p := range params[1 : len(params)-1] {
		kind, how := store(p, names)
		switch {
		case kind != "" && !v.db && !v.tx:
			v.db, v.tx, wrong = kind == "db", kind == "tx", how
		case kind != "":
			if v.db && kind == "tx" || v.tx && kind == "db" {
				both = true
			} else {
				order = true // a second one
			}
		case user != nil:
			order = true
		default:
			order = order || v.db || v.tx // the user comes before the database
			user = p
		}
	}
	// A user or the database last, or a model: the In is missing. Read as
	// an In, its fix would drop the user, and the login with it.
	if kind, _ := store(last, names); in != last || isModel(app, in) || kind != "" {
		u, db, tx := user, v.db, v.tx
		if kind != "" {
			db, tx = db || kind == "db", tx || kind == "tx"
		} else if user == nil {
			u = last
		}
		errAt(last.Pos(), "view "+r.View+" takes no In: In comes last, even when empty", "write "+viewSignature(r.View, u, db, tx, "struct{}"))
		return nil, ds
	}
	switch {
	case both:
		errAt(params[1].Pos(), "view "+r.View+" takes both maago.Tx and maago.DB", "write "+viewSignature(r.View, user, false, true, types.ExprString(last))+": the transaction reads and writes (D-150)")
		return nil, ds
	case order:
		errAt(params[1].Pos(), "view "+r.View+" takes its values out of order", "write "+viewSignature(r.View, user, v.db, v.tx, types.ExprString(last)))
		return nil, ds
	case wrong != "" && v.db:
		errAt(params[1].Pos(), "view "+r.View+" takes the database as "+wrong+", not maago.DB", "write "+viewSignature(r.View, user, true, false, types.ExprString(last)))
		return nil, ds
	case wrong != "":
		errAt(params[1].Pos(), "view "+r.View+" takes the transaction as "+wrong+", not maago.Tx", "write "+viewSignature(r.View, user, false, true, types.ExprString(last)))
		return nil, ds
	case v.tx && (r.Method == "GET" || r.Method == "HEAD"):
		errAt(params[1].Pos(), "view "+r.View+" takes maago.Tx, but route "+r.Name+" is a "+r.Method+", which does not write", "take db maago.DB; a view that writes serves a POST")
		return nil, ds
	}
	if user != nil {
		v.user, v.userExpr = true, types.ExprString(user)
		_, v.userPtr = user.(*ast.StarExpr)
	}
	var st *ast.StructType
	switch t := last.(type) {
	case *ast.StructType:
		st = t
	case *ast.Ident:
		if ts := app.types[t.Name]; ts != nil && ts.TypeParams == nil {
			st, _ = ts.Type.(*ast.StructType)
			v.in = t.Name
		}
	}
	if st == nil || v.in == "" && len(st.Fields.List) > 0 {
		errAt(last.Pos(), "the In of view "+r.View+" is not struct{} or a struct type named in package "+app.Name,
			"declare type "+r.View+"In struct{…} in package "+app.Name+" with fields tagged maago:\"path=name\", maago:\"query=name\" or maago:\"form\", or take struct{}; if it is the user, In comes after it: "+viewSignature(r.View, last, false, false, "struct{}"))
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

// viewSignature is a typed view's signature in the order D-149 gives, for a
// fix.
func viewSignature(view string, user ast.Expr, db, tx bool, in string) string {
	s := "func " + view + "(ctx context.Context, "
	if user != nil {
		s += "u " + types.ExprString(user) + ", "
	}
	if tx {
		s += "tx maago.Tx, "
	} else if db {
		s += "db maago.DB, "
	}
	return s + "in " + in + ") …"
}

// store is what e names of the database: "db" for a DB, "tx" for a
// transaction, "" for neither. how is e as written when it is not maago.DB
// or maago.Tx, such as *sql.DB or *maago.Tx: the database all the same.
func store(e ast.Expr, names []string) (kind, how string) {
	t := e
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	sel, ok := t.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}
	x, ok := sel.X.(*ast.Ident)
	switch {
	case !ok:
		return "", ""
	case slices.Contains(names, x.Name) && (sel.Sel.Name == "DB" || sel.Sel.Name == "Tx"):
		kind = strings.ToLower(sel.Sel.Name)
	case x.Name == "sql" && (sel.Sel.Name == "DB" || sel.Sel.Name == "Conn"):
		kind = "db"
	case x.Name == "sql" && sel.Sel.Name == "Tx":
		kind = "tx"
	default:
		return "", ""
	}
	if t != e || x.Name == "sql" {
		how = types.ExprString(e)
	}
	return kind, how
}

// isModel reports whether e names a struct of app that embeds maago.Model:
// a user, say, but no In.
func isModel(app *App, e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	if !ok || app.types[id.Name] == nil {
		return false
	}
	st, ok := app.types[id.Name].Type.(*ast.StructType)
	return ok && slices.ContainsFunc(st.Fields.List, func(f *ast.Field) bool { return len(f.Names) == 0 && isMaago(f.Type, "Model", app.maago[id.Name]) })
}

// isForm reports whether app declares name as a struct that embeds
// maago.Form, as Forms will find.
func isForm(app *App, name string) bool {
	ts := app.types[name]
	if ts == nil {
		return false
	}
	st, ok := ts.Type.(*ast.StructType)
	if !ok {
		return false
	}
	return slices.ContainsFunc(st.Fields.List, func(f *ast.Field) bool { return len(f.Names) == 0 && isMaago(f.Type, "Form", app.maago[name]) })
}

// noTxOptions checks maago.NoTx against the routes and their views: it is
// for a view that writes and takes maago.DB, on every route it serves
// (D-150).
func noTxOptions(fset *token.FileSet, app *App) []diag.Diagnostic {
	var ds []diag.Diagnostic
	routes := map[string][]*Route{} // the routes of each view, in order
	for i := range app.Routes {
		r := &app.Routes[i]
		routes[r.View] = append(routes[r.View], r)
		if !hasOption(r.Options, "NoTx") || r.view == nil {
			continue
		}
		at, fix := fset.Position(optionPos(r, "NoTx")), "remove maago.NoTx"
		switch {
		case r.Method == "GET" || r.Method == "HEAD":
			ds = append(ds, diag.New(at, "route "+r.Name+" has maago.NoTx, but a "+r.Method+" writes nothing", fix))
		case r.view.escape:
			ds = append(ds, diag.New(at, "route "+r.Name+" has maago.NoTx, but view "+r.View+" is a func(w, r), which has no transaction", fix))
		case r.view.tx:
			ds = append(ds, diag.New(at, "route "+r.Name+" has maago.NoTx, but view "+r.View+" takes maago.Tx", "remove maago.NoTx, or take db maago.DB"))
		case !r.view.db:
			ds = append(ds, diag.New(at, "route "+r.Name+" has maago.NoTx, but view "+r.View+" takes no maago.DB", "take db maago.DB, or remove maago.NoTx"))
		}
	}
	for _, view := range slices.Sorted(maps.Keys(routes)) {
		var with, without *Route
		for _, r := range routes[view] {
			if r.Method == "GET" || r.Method == "HEAD" {
				continue
			}
			if hasOption(r.Options, "NoTx") {
				with = cmp.Or(with, r)
			} else {
				without = cmp.Or(without, r)
			}
		}
		if with != nil && without != nil {
			ds = append(ds, diag.New(fset.Position(optionPos(with, "NoTx")), fmt.Sprintf("route %s has maago.NoTx, but route %s of the same view %s does not", with.Name, without.Name, view),
				"give both maago.NoTx, or split the view"))
		}
	}
	return ds
}

// optionPos is where route r names option name.
func optionPos(r *Route, name string) token.Pos {
	for _, o := range r.Options {
		if o.Name == name {
			return o.Pos
		}
	}
	return r.Pos
}
