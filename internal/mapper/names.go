package mapper

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/pipeline"
)

// Imports are the packages the generated files may import: no top-level
// name of the package may be one of their names.
var Imports = []string{"context", "database/sql", "fmt", "time", "github.com/maago-web/maago"}

// Names reports the names the generated files of the app at dir would
// declare twice: in two places of their own, or where the package's own
// files, its tests included, already declare them. app is nil for an app
// without models, and qs for one without queries. It needs no database:
// a query's result is a new type when it names no model of the app, as
// Results decides.
func Names(fset *token.FileSet, root, dir string, app *model.App, qs []*Query) []diag.Diagnostic {
	type decl struct {
		by  string // what generates the name
		pos token.Position
	}
	var ds []diag.Diagnostic
	user := declared(root, dir)
	seen := map[string]decl{}
	clashed := map[[2]string]bool{} // reported once for all the names two generators share
	add := func(name, by string, pos token.Position, fix string) {
		if prev, ok := seen[name]; ok {
			if !clashed[[2]string{by, prev.by}] {
				clashed[[2]string{by, prev.by}] = true
				ds = append(ds, diag.New(pos, fmt.Sprintf("%s would declare %s, which %s declares too, at %s:%d", by, name, prev.by, prev.pos.Filename, prev.pos.Line), fix))
			}
			return
		}
		seen[name] = decl{by, pos}
		if at, ok := user[name]; ok {
			ds = append(ds, diag.New(pos, fmt.Sprintf("%s would declare %s, which %s declares already", by, name, fmt.Sprintf("%s:%d", at.Filename, at.Line)), fix))
		}
	}
	if app != nil {
		for _, m := range app.Models {
			by, pos := "the CRUD of model "+m.Name, fset.Position(m.Pos)
			fix := "rename what your code declares under that name; the CRUD's names follow the model's (D-21)"
			for _, name := range []string{"New" + m.Name + "Input", "Get" + m.Name, "Insert" + m.Name, "Delete" + m.Name, "scan" + m.Name} {
				add(name, by, pos, fix)
			}
			if slices.ContainsFunc(m.Fields, func(f *model.Field) bool { return f.Name != "ID" && f.Auto != "create" }) {
				add("Update"+m.Name, by, pos, fix)
			}
		}
	}
	results := map[string]bool{} // new result types, declared once
	for _, q := range qs {
		by, fix := "query "+q.Name, "rename the query, or what else declares that name"
		add(q.Name, by, q.NamePos, fix)
		if q.Kind != Exec {
			add("scan"+q.Name, by, q.NamePos, fix)
		}
		if len(q.Params) > 0 {
			add(q.Name+"Params", by, q.NamePos, fix)
		}
		for _, p := range q.Params {
			if p.Type.Choices == nil {
				continue
			}
			typ := q.Name + fieldName(p.Name)
			by := "parameter " + p.Name + " of query " + q.Name
			fix := "rename the query or the parameter, or what else declares that name"
			add(typ, by, p.Pos, fix)
			for _, v := range p.Type.Choices {
				add(typ+fieldName(v), by, p.Pos, fix)
			}
		}
		if q.Kind != Exec && modelNamed(app, q.Result) == nil && !results[q.Result] {
			results[q.Result] = true
			add(q.Result, "the result type of query "+q.Name, q.NamePos, "rename the result type, or what else declares that name")
		}
	}
	for _, imp := range Imports {
		name := path.Base(imp)
		if at, ok := user[name]; ok {
			ds = append(ds, diag.New(at, fmt.Sprintf("%s is declared at the top level, but maago's generated files in this package may import a package %s", name, name),
				"rename it"))
		}
	}
	return ds
}

// PackageName is the package of the app at dir: the package clause of a
// Go file there that is not a test and that maago did not generate. An app
// with queries but no such file has no package to generate into.
func PackageName(root, dir string) (string, []diag.Diagnostic) {
	for _, f := range goFiles(root, dir) {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil || pipeline.IsGenerated(src) {
			continue
		}
		if af, err := parser.ParseFile(token.NewFileSet(), f, src, parser.PackageClauseOnly); err == nil {
			return af.Name.Name, nil
		}
	}
	return "", []diag.Diagnostic{diag.New(token.Position{Filename: path.Join(dir, "queries")},
		"directory "+dir+" has queries but no Go file saying which package they belong to",
		"add a Go file with a package clause to "+dir+", such as a doc.go")}
}

// goFiles are the Go files in dir that Go does not ignore, sorted.
func goFiles(root, dir string) []string {
	files, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(dir), "*.go"))
	return slices.DeleteFunc(files, func(f string) bool {
		return strings.HasPrefix(filepath.Base(f), ".") || strings.HasPrefix(filepath.Base(f), "_")
	})
}

// declared are the top-level names of the Go files in dir that maago did
// not generate, and where they are. A test file counts unless it is an
// external test package, since the tests compile with the package.
func declared(root, dir string) map[string]token.Position {
	fset := token.NewFileSet()
	names := map[string]token.Position{}
	add := func(id *ast.Ident) {
		if id.Name != "_" && id.Name != "init" {
			names[id.Name] = fset.Position(id.Pos())
		}
	}
	for _, f := range goFiles(root, dir) {
		src, err := os.ReadFile(f)
		if err != nil || pipeline.IsGenerated(src) {
			continue
		}
		af, err := parser.ParseFile(fset, path.Join(dir, filepath.Base(f)), src, parser.SkipObjectResolution)
		if err != nil || strings.HasSuffix(af.Name.Name, "_test") {
			continue // the models' reading reports a syntax error
		}
		for _, d := range af.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					add(d.Name)
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						add(s.Name)
					case *ast.ValueSpec:
						for _, n := range s.Names {
							add(n)
						}
					}
				}
			}
		}
	}
	return names
}
