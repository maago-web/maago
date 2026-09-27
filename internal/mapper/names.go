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

// imported are the packages the generated files may import: no top-level
// name of the package may be one of them.
var imported = []string{"context", "sql", "time", "fmt", "maago"}

// Names reports the names the generated files of the app at dir would
// declare twice: in two places of their own, or where the package's own
// files, its tests included, already declare them. app is nil for an app
// without models, and qs for one without queries.
func Names(fset *token.FileSet, root, dir string, app *model.App, qs []Checked) []diag.Diagnostic {
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
	results := map[*Result]bool{}
	for _, c := range qs {
		q := c.Query
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
		if r := c.Result; r != nil && r.Model == nil && !results[r] {
			results[r] = true
			add(r.Name, "the result type of query "+q.Name, q.NamePos, "rename the result type, or what else declares that name")
		}
	}
	for _, name := range imported {
		if at, ok := user[name]; ok {
			ds = append(ds, diag.New(at, fmt.Sprintf("%s is declared at the top level, but maago's generated files in this package may import a package %s", name, name),
				"rename it"))
		}
	}
	return ds
}

// declared are the top-level names of the Go files in dir that maago did
// not generate, and where they are. A test file counts unless it is an
// external test package, since the tests compile with the package.
func declared(root, dir string) map[string]token.Position {
	files, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(dir), "*.go"))
	fset := token.NewFileSet()
	names := map[string]token.Position{}
	add := func(id *ast.Ident) {
		if id.Name != "_" && id.Name != "init" {
			names[id.Name] = fset.Position(id.Pos())
		}
	}
	for _, f := range files {
		if strings.HasPrefix(filepath.Base(f), ".") || strings.HasPrefix(filepath.Base(f), "_") {
			continue // Go ignores it
		}
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
