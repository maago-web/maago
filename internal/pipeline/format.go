package pipeline

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path"
	"slices"
	"strconv"
)

// Runtime are the packages a generated file may import from maago: each
// generator imports both, and Format drops the one the file does not use.
var Runtime = []string{"github.com/maago-web/maago", "github.com/maago-web/maago/rt"}

// Format is the generated file name as gofmt writes it, without an import
// of Runtime it does not use: go/format keeps one. src that is not Go is a
// bug of maago, so it panics.
func Format(name string, src []byte) []byte {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		panic(fmt.Sprintf("%s is not Go: %v\n%s", name, err, src))
	}
	used := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if s, ok := n.(*ast.SelectorExpr); ok {
			if x, ok := s.X.(*ast.Ident); ok {
				used[x.Name] = true
			}
		}
		return true
	})
	for _, d := range f.Decls {
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
			g.Specs = slices.DeleteFunc(g.Specs, func(s ast.Spec) bool {
				p, _ := strconv.Unquote(s.(*ast.ImportSpec).Path.Value)
				return slices.Contains(Runtime, p) && !used[path.Base(p)]
			})
		}
	}
	var b bytes.Buffer
	if err := format.Node(&b, fset, f); err != nil {
		panic(fmt.Sprintf("%s is not Go: %v\n%s", name, err, src))
	}
	return b.Bytes()
}
