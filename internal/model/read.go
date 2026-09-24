package model

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/maago-web/maago/internal/diag"
)

// maagoPath is the import path of the runtime package that holds Model.
const maagoPath = "github.com/maago-web/maago"

// Package is a Go package as Read found it: its models, with their tags
// parsed but not yet checked.
type Package struct {
	Dir    string // slash path relative to the project root
	Name   string // from the package clause
	Models []*RawModel
}

// RawModel is a struct that embeds maago.Model.
type RawModel struct {
	Name    string
	Pos     token.Pos
	Options []Option // from the tag on the embedded maago.Model
	Fields  []*RawField
}

// RawField is one field of a model, other than the embedded maago.Model.
type RawField struct {
	Name     string // for an embedded field, the type as written
	Pos      token.Pos
	Embedded bool
	Type     ast.Expr
	TimeName string // the name its file imports "time" under, or ""
	Options  []Option
}

// Read parses the Go files of the project, except tests, and returns the
// packages that hold models. A package with a syntax error in any file is
// left out, since checking what was half read would only add errors that
// follow from it.
func Read(fset *token.FileSet, root string, files []string) ([]*Package, []diag.Diagnostic) {
	byDir := map[string][]string{}
	for _, f := range files {
		if !strings.HasSuffix(f, "_test.go") {
			byDir[path.Dir(f)] = append(byDir[path.Dir(f)], f)
		}
	}
	var pkgs []*Package
	var ds []diag.Diagnostic
	for _, dir := range slices.Sorted(maps.Keys(byDir)) {
		var parsed []*ast.File
		for _, f := range byDir[dir] {
			af, fds := parseFile(fset, root, f)
			ds = append(ds, fds...)
			if af != nil {
				parsed = append(parsed, af)
			}
		}
		if len(parsed) < len(byDir[dir]) {
			continue
		}
		pkg, pds := readPackage(fset, dir, parsed)
		ds = append(ds, pds...)
		if pkg != nil && len(pkg.Models) > 0 {
			pkgs = append(pkgs, pkg)
		}
	}
	return pkgs, ds
}

// parseFile returns nil and the syntax errors if f does not parse.
func parseFile(fset *token.FileSet, root, f string) (*ast.File, []diag.Diagnostic) {
	src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
	if err != nil {
		return nil, []diag.Diagnostic{diag.New(token.Position{Filename: f}, "cannot read file: "+diag.OneLine(err.Error()), "make sure the file can be read")}
	}
	af, err := parser.ParseFile(fset, f, src, parser.AllErrors|parser.SkipObjectResolution)
	if err == nil {
		return af, nil
	}
	var list scanner.ErrorList
	if !errors.As(err, &list) {
		return nil, []diag.Diagnostic{diag.New(token.Position{Filename: f}, diag.OneLine(err.Error()), "fix the Go syntax error")}
	}
	var ds []diag.Diagnostic
	for _, e := range list {
		ds = append(ds, diag.New(e.Pos, diag.OneLine(e.Msg), "fix the Go syntax error"))
	}
	return nil, ds
}

func readPackage(fset *token.FileSet, dir string, files []*ast.File) (*Package, []diag.Diagnostic) {
	pkg := &Package{Dir: dir, Name: files[0].Name.Name}
	for _, af := range files[1:] {
		if af.Name.Name != pkg.Name {
			return nil, []diag.Diagnostic{diag.New(fset.Position(af.Name.Pos()),
				"directory has two packages, "+pkg.Name+" and "+af.Name.Name, "keep one package per directory")}
		}
	}
	var ds []diag.Diagnostic
	for _, af := range files {
		maagoNames, timeName, dotMaago := importNames(af)
		for _, decl := range af.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts := spec.(*ast.TypeSpec)
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				m, mds := readModel(fset, ts, st, maagoNames, timeName, dotMaago)
				ds = append(ds, mds...)
				if m != nil {
					pkg.Models = append(pkg.Models, m)
				}
			}
		}
	}
	return pkg, ds
}

// importNames returns the names a file imports maago under (it may import
// it more than once), the name it imports time under, and whether it
// dot-imports maago.
func importNames(af *ast.File) (maagoNames []string, timeName string, dotMaago bool) {
	for _, imp := range af.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		name := path.Base(p)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		switch {
		case p == maagoPath && name == ".":
			dotMaago = true
		case p == maagoPath && name != "_":
			maagoNames = append(maagoNames, name)
		case p == "time" && name != "_" && name != ".":
			timeName = name
		}
	}
	return maagoNames, timeName, dotMaago
}

// readModel returns nil if the struct does not embed maago.Model.
func readModel(fset *token.FileSet, ts *ast.TypeSpec, st *ast.StructType, maagoNames []string, timeName string, dotMaago bool) (*RawModel, []diag.Diagnostic) {
	isModel := func(e ast.Expr) bool {
		sel, ok := e.(*ast.SelectorExpr)
		if ok {
			x, ok := sel.X.(*ast.Ident)
			return ok && slices.Contains(maagoNames, x.Name) && sel.Sel.Name == "Model"
		}
		id, ok := e.(*ast.Ident)
		return ok && dotMaago && id.Name == "Model"
	}
	errAt := func(p token.Pos, msg, fix string) diag.Diagnostic { return diag.New(fset.Position(p), msg, fix) }

	var embeds []*ast.Field
	var ds []diag.Diagnostic
	for _, f := range st.Fields.List {
		if len(f.Names) > 0 {
			continue
		}
		switch t := f.Type.(type) {
		case *ast.StarExpr:
			if isModel(t.X) {
				ds = append(ds, errAt(f.Pos(), "model "+ts.Name.Name+" embeds a pointer to maago.Model", "embed maago.Model, not *maago.Model"))
			}
		default:
			if isModel(t) {
				if _, dot := t.(*ast.Ident); dot {
					ds = append(ds, errAt(f.Pos(), "model "+ts.Name.Name+" embeds Model from a dot import", "import maago by name and embed maago.Model"))
				}
				embeds = append(embeds, f)
			}
		}
	}
	if len(embeds) == 0 {
		return nil, ds
	}
	if len(embeds) > 1 {
		ds = append(ds, errAt(embeds[1].Pos(), "model "+ts.Name.Name+" embeds maago.Model twice", "remove one of them"))
	}
	if ts.TypeParams != nil {
		ds = append(ds, errAt(ts.Name.Pos(), "model "+ts.Name.Name+" has type parameters", "models cannot be generic; remove the type parameters"))
	}
	if len(ds) > 0 {
		return nil, ds
	}

	m := &RawModel{Name: ts.Name.Name, Pos: ts.Name.Pos()}
	opts, tds := ParseTag(fset, embeds[0].Tag)
	m.Options, ds = opts, append(ds, tds...)
	for _, f := range st.Fields.List {
		if f == embeds[0] {
			continue
		}
		opts, tds := ParseTag(fset, f.Tag)
		ds = append(ds, tds...)
		if len(f.Names) == 0 {
			m.Fields = append(m.Fields, &RawField{Name: types.ExprString(f.Type), Pos: f.Pos(), Embedded: true, Type: f.Type, TimeName: timeName, Options: opts})
			continue
		}
		for _, n := range f.Names {
			m.Fields = append(m.Fields, &RawField{Name: n.Name, Pos: n.Pos(), Type: f.Type, TimeName: timeName, Options: opts})
		}
	}
	return m, ds
}
