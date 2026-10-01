// Package web reads and checks the HTTP side of an app: its route table
// (D-115), and later its views, templates and forms.
package web

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/pipeline"
)

const maagoPath = "github.com/maago-web/maago"

// App is a package with a route table.
type App struct {
	Dir        string // slash path relative to the project root
	Name       string // the package clause
	ImportPath string
	Pos        token.Pos // the name of var Routes
	Routes     []Route
	Funcs      []string // the package's top-level functions
	Used       bool     // another package's code uses its App

	funcs map[string]*ast.FuncDecl // the top-level functions, for Views
	types map[string]*ast.TypeSpec // the top-level types
}

// Route is one line of a route table, as written: Check judges it.
type Route struct {
	Method, Path, Name, View       string
	Pos, PathPos, NamePos, ViewPos token.Pos
	Options                        []Option

	view *view // filled by Views
}

// Option is a route option as written: maago.LoginRequired.
type Option struct {
	Name string
	Pos  token.Pos
}

// methods are the route constructors of the runtime, and options its
// route options.
var (
	methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
	options = []string{"LoginRequired", "API"}
)

// Read finds the route tables of the project and checks their shape: a
// top-level var Routes = maago.Routes{…} whose lines are calls with
// literal paths and names, and views and options named plainly. What the
// names mean, Check judges. Tests and maago's generated files are not
// read. files are the project's Go files; the models' reading has
// reported any syntax error before.
func Read(fset *token.FileSet, root string, files []string, modulePath string) ([]*App, []diag.Diagnostic) {
	byDir := map[string][]*ast.File{} // the parsed files of each package
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil || pipeline.IsGenerated(src) {
			continue // an unreadable file was reported with the models
		}
		af, err := parser.ParseFile(fset, f, src, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		byDir[path.Dir(f)] = append(byDir[path.Dir(f)], af)
	}
	var apps []*App
	var ds []diag.Diagnostic
	for _, dir := range slices.Sorted(maps.Keys(byDir)) {
		app, ads := readApp(fset, dir, byDir[dir], importPath(modulePath, dir))
		ds = append(ds, ads...)
		if app != nil {
			apps = append(apps, app)
		}
	}
	ds = append(ds, markUsed(fset, byDir, apps)...)
	return apps, ds
}

func importPath(modulePath, dir string) string { return path.Join(modulePath, dir) }

// readApp returns the app of p, or nil if p has no route table.
func readApp(fset *token.FileSet, dir string, files []*ast.File, importPath string) (*App, []diag.Diagnostic) {
	app := &App{Dir: dir, Name: files[0].Name.Name, ImportPath: importPath,
		funcs: map[string]*ast.FuncDecl{}, types: map[string]*ast.TypeSpec{}}
	var ds []diag.Diagnostic
	errAt := func(pos token.Pos, msg, fix string) { ds = append(ds, diag.New(fset.Position(pos), msg, fix)) }
	const form = "declare the table at the top level as var Routes = maago.Routes{…}"
	var table *ast.CompositeLit
	var tableNames []string // the names the table's file imports maago under
	for _, f := range files {
		names, dot := maagoNames(f)
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					app.Funcs = append(app.Funcs, d.Name.Name)
					app.funcs[d.Name.Name] = d
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					if ts, ok := s.(*ast.TypeSpec); ok {
						app.types[ts.Name.Name] = ts
					}
				}
			}
		}
		accepted := map[*ast.CompositeLit]bool{}
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR || len(names) == 0 && !dot {
				continue // a var Routes in a file without maago is not a route table
			}
			for _, s := range gd.Specs {
				vs := s.(*ast.ValueSpec)
				i := slices.IndexFunc(vs.Names, func(id *ast.Ident) bool { return id.Name == "Routes" })
				if i < 0 {
					continue
				}
				if dot {
					errAt(vs.Names[i].Pos(), "route table in a file that dot-imports maago", `import maago by name: import "github.com/maago-web/maago"`)
					continue
				}
				var lit *ast.CompositeLit
				if i < len(vs.Values) {
					lit, _ = vs.Values[i].(*ast.CompositeLit)
				}
				if lit != nil {
					accepted[lit] = true
				}
				switch {
				case len(vs.Names) > 1 || vs.Type != nil:
					errAt(vs.Names[i].Pos(), "route table declared in another form", form)
				case lit == nil || !isMaago(lit.Type, "Routes", names):
					errAt(vs.Names[i].Pos(), "var Routes is not a maago.Routes literal", form+"; maago gen reads the table as written")
				case table != nil:
					errAt(vs.Names[i].Pos(), "second route table in package "+app.Name, "keep one var Routes per package")
				default:
					table, tableNames, app.Pos = lit, names, vs.Names[i].Pos()
				}
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.CompositeLit); ok && !accepted[lit] && isMaago(lit.Type, "Routes", names) {
				errAt(lit.Pos(), "maago.Routes literal outside var Routes", form)
			}
			return true
		})
	}
	if table == nil {
		return nil, ds
	}
	if app.Name == "main" {
		errAt(app.Pos, "route table in package main", "move the routes and their views to an app package, and add its App to maago.Config.Apps in main (D-114)")
	}
	app.Routes = []Route{} // an empty table is an app, which Check rejects
	for _, e := range table.Elts {
		r, ok := readRoute(e, tableNames)
		if !ok {
			errAt(e.Pos(), "route is not written as maago.METHOD(\"/path\", \"name\", View, options…)",
				"write the path and the name as string literals, the view as the name of a function of this package, and options as maago.LoginRequired or maago.API")
			continue
		}
		app.Routes = append(app.Routes, r)
	}
	return app, ds
}

// readRoute reads maago.GET("/path", "name", View, maago.Option…).
func readRoute(e ast.Expr, names []string) (Route, bool) {
	call, ok := e.(*ast.CallExpr)
	if !ok || !isMaago(call.Fun, "", names) || len(call.Args) < 3 || call.Ellipsis.IsValid() {
		return Route{}, false
	}
	r := Route{Method: call.Fun.(*ast.SelectorExpr).Sel.Name, Pos: call.Pos()}
	for i, dst := range []*string{&r.Path, &r.Name} {
		lit, ok := call.Args[i].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return Route{}, false
		}
		*dst, _ = strconv.Unquote(lit.Value)
	}
	r.PathPos, r.NamePos = call.Args[0].Pos(), call.Args[1].Pos()
	view, ok := call.Args[2].(*ast.Ident)
	if !ok || view.Name == "nil" {
		return Route{}, false
	}
	r.View, r.ViewPos = view.Name, view.Pos()
	for _, o := range call.Args[3:] {
		if !isMaago(o, "", names) {
			return Route{}, false
		}
		r.Options = append(r.Options, Option{o.(*ast.SelectorExpr).Sel.Name, o.Pos()})
	}
	return r, true
}

// isMaago reports whether e is maago.sel, or maago.anything if sel is "".
func isMaago(e ast.Expr, sel string, names []string) bool {
	s, ok := e.(*ast.SelectorExpr)
	if !ok || sel != "" && s.Sel.Name != sel {
		return false
	}
	x, ok := s.X.(*ast.Ident)
	return ok && slices.Contains(names, x.Name)
}

// maagoNames returns the names f imports maago under, and whether it
// dot-imports it.
func maagoNames(f *ast.File) (names []string, dot bool) {
	for _, imp := range f.Imports {
		if p, _ := strconv.Unquote(imp.Path.Value); p != maagoPath {
			continue
		}
		switch {
		case imp.Name == nil:
			names = append(names, "maago")
		case imp.Name.Name == ".":
			dot = true
		case imp.Name.Name != "_":
			names = append(names, imp.Name.Name)
		}
	}
	return names, dot
}

// markUsed marks the apps whose App another package uses, and rejects a
// dot import of an app, which would hide that use.
func markUsed(fset *token.FileSet, byDir map[string][]*ast.File, apps []*App) []diag.Diagnostic {
	byPath := map[string]*App{}
	for _, a := range apps {
		byPath[a.ImportPath] = a
	}
	var ds []diag.Diagnostic
	for _, files := range byDir {
		for _, f := range files {
			names := map[string]*App{}
			for _, imp := range f.Imports {
				ip, _ := strconv.Unquote(imp.Path.Value)
				a := byPath[ip]
				switch {
				case a == nil:
				case imp.Name == nil:
					names[a.Name] = a
				case imp.Name.Name == ".":
					ds = append(ds, diag.New(fset.Position(imp.Pos()), "app "+a.Name+" is dot-imported", "import it by name and use "+a.Name+".App"))
				case imp.Name.Name != "_":
					names[imp.Name.Name] = a
				}
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if s, ok := n.(*ast.SelectorExpr); ok && s.Sel.Name == "App" {
					if x, ok := s.X.(*ast.Ident); ok && names[x.Name] != nil {
						names[x.Name].Used = true
					}
				}
				return true
			})
		}
	}
	return ds
}

// name is the form of a route name: lower-case words joined by single
// underscores, so that two names never make the same URL function.
var name = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// Check judges what the route tables say: methods, options, views, names,
// paths, and whether each app is used. The tables are checked together,
// since route names and paths are one namespace across the project.
func Check(fset *token.FileSet, apps []*App) []diag.Diagnostic {
	var ds []diag.Diagnostic
	errAt := func(pos token.Pos, msg, fix string) { ds = append(ds, diag.New(fset.Position(pos), msg, fix)) }
	didYouMean := func(got string, allowed []string) string {
		s := "allowed: " + strings.Join(allowed, ", ")
		if m := diag.Suggest(got, allowed); m != "" {
			s = fmt.Sprintf("did you mean %q? %s", m, s)
		}
		return s
	}
	type seen struct {
		r   *Route
		pos token.Position
	}
	names := map[string]seen{}
	var valid []seen // routes whose pattern parses, in order
	for _, app := range apps {
		if len(app.Routes) == 0 {
			errAt(app.Pos, "route table of "+app.Name+" is empty", "add a route, or remove the table")
		}
		if !app.Used {
			errAt(app.Pos, "app "+app.Name+" has routes, but no package uses "+app.Name+".App",
				"add "+app.Name+".App to maago.Config.Apps in main (D-114), then run maago gen: go build works after gen")
		}
		for i := range app.Routes {
			r := &app.Routes[i]
			if !slices.Contains(methods, r.Method) {
				errAt(r.Pos, "unknown route method maago."+r.Method, didYouMean(r.Method, methods))
			}
			for j, o := range r.Options {
				switch {
				case !slices.Contains(options, o.Name):
					errAt(o.Pos, "unknown route option maago."+o.Name, didYouMean(o.Name, options))
				case slices.ContainsFunc(r.Options[:j], func(p Option) bool { return p.Name == o.Name }):
					errAt(o.Pos, "route option maago."+o.Name+" given twice", "remove one")
				}
			}
			if !slices.Contains(app.Funcs, r.View) {
				fix := "declare func " + r.View + " in package " + app.Name + ", or name one of its functions"
				if m := diag.Suggest(r.View, app.Funcs); m != "" {
					fix = fmt.Sprintf("did you mean %q?", m)
				}
				errAt(r.ViewPos, "no function "+r.View+" in package "+app.Name, fix)
			}
			switch prev, dup := names[r.Name]; {
			case !name.MatchString(r.Name):
				errAt(r.NamePos, fmt.Sprintf("route name %q is not lower-case words joined by single underscores", r.Name), "name it like post_detail")
			case dup:
				errAt(r.NamePos, fmt.Sprintf("route name %q is taken by %s:%d", r.Name, prev.pos.Filename, prev.pos.Line), "route names are one namespace across the project; rename one")
			default:
				names[r.Name] = seen{r, fset.Position(r.NamePos)}
			}
			if msg, fix := checkPath(r); msg != "" {
				errAt(r.PathPos, msg, fix)
				continue
			}
			for _, v := range valid {
				if conflicts(v.r, r) {
					errAt(r.PathPos, fmt.Sprintf("route %s %s conflicts with %s (%s:%d): a request could match both", r.Method, r.Path, v.r.Name, v.pos.Filename, v.pos.Line),
						"make one pattern more specific than the other, or remove one")
					break
				}
			}
			valid = append(valid, seen{r, fset.Position(r.Pos)})
		}
	}
	return ds
}

// checkPath returns what is wrong with the route's path, if anything.
// net/http's ServeMux judges the pattern: registering it panics if it is
// wrong. The panic's words are no API, so they only add detail.
func checkPath(r *Route) (msg, fix string) {
	switch {
	case !strings.HasPrefix(r.Path, "/"):
		return fmt.Sprintf("route path %q does not start with /", r.Path), "write the path from the root, such as /posts/{id}"
	case strings.HasSuffix(r.Path, "/"):
		return fmt.Sprintf("route path %q ends in /, so it would match every path below it", r.Path),
			"end it in {$} to match that path alone, such as " + r.Path + "{$}"
	}
	defer func() {
		if p := recover(); p != nil {
			msg = fmt.Sprintf("route path %q is not a valid pattern", r.Path)
			fix = "see the patterns of net/http's ServeMux, such as /posts/{id}"
			if s := fmt.Sprint(p); strings.HasPrefix(s, "parsing ") && !strings.Contains(s, "\n") {
				if _, detail, ok := strings.Cut(s, ": "); ok {
					msg += ": " + detail
				}
			}
		}
	}()
	http.NewServeMux().Handle(r.Method+" "+r.Path, http.NotFoundHandler())
	return "", ""
}

// conflicts reports whether ServeMux rejects a and b together: both valid
// alone, the second panics only on a conflict.
func conflicts(a, b *Route) (yes bool) {
	defer func() { yes = recover() != nil }()
	mux := http.NewServeMux()
	mux.Handle(a.Method+" "+a.Path, http.NotFoundHandler())
	mux.Handle(b.Method+" "+b.Path, http.NotFoundHandler())
	return false
}
