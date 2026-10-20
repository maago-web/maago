package web

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/pipeline"
)

const maagotestPath = maagoPath + "/maagotest"

// Permissions reports each app with a view that takes the user whose
// tests call no maagotest.Permissions (D-156): where people differ in
// what they may do, a table says who gets what, and a route added
// without its row fails go test. An app without such a view has one
// role, and needs no table. files are the project's Go files, tests
// among them; a call in any test file of the app's directory counts,
// in the package or its _test package, in a helper too. Views has
// judged the views.
func Permissions(fset *token.FileSet, root string, files []string, apps []*App) []diag.Diagnostic {
	var ds []diag.Diagnostic
	for _, app := range apps {
		if !takesUser(app) {
			continue
		}
		called, dot, broken := false, false, ""
		for _, f := range files {
			if !strings.HasSuffix(f, "_test.go") || path.Dir(f) != app.Dir {
				continue
			}
			src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
			if err != nil || pipeline.IsGenerated(src) {
				continue
			}
			af, err := parser.ParseFile(token.NewFileSet(), f, src, parser.SkipObjectResolution)
			if err != nil {
				broken = f // its call cannot be seen
				continue
			}
			c, d := callsPermissions(af)
			called, dot = called || c, dot || d
		}
		if called {
			continue
		}
		fix := "write " + path.Join(app.Dir, "permissions_test.go") + " calling maagotest.Permissions(t, []maago.App{App, …}, roles, rows...), a row for each route"
		switch {
		case broken != "":
			fix = broken + " does not parse, so its calls cannot be seen; fix it (go vet says where), then run maago gen"
		case dot:
			fix = "import " + maagotestPath + " by name, not with a dot: maago gen looks for maagotest.Permissions"
		}
		ds = append(ds, diag.New(fset.Position(app.Pos), "app "+app.Name+" has routes whose views take the user, but no test calls maagotest.Permissions", fix))
	}
	return ds
}

// takesUser reports whether a view of app takes the user.
func takesUser(app *App) bool {
	for _, r := range app.Routes {
		if r.view != nil && r.view.user {
			return true
		}
	}
	return false
}

// callsPermissions reports whether f calls Permissions of maagotest
// under the name f imports it by, and whether f imports it with a dot.
func callsPermissions(f *ast.File) (called, dot bool) {
	name := ""
	for _, imp := range f.Imports {
		if p, _ := strconv.Unquote(imp.Path.Value); p != maagotestPath {
			continue
		}
		switch {
		case imp.Name == nil:
			name = "maagotest"
		case imp.Name.Name == ".":
			dot = true
		case imp.Name.Name != "_":
			name = imp.Name.Name
		}
	}
	if name == "" {
		return false, dot
	}
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Permissions" {
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == name {
					called = true
				}
			}
		}
		return !called
	})
	return called, dot
}
