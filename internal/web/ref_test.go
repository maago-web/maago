package web

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/ref"
)

// The lists of maago doc whose source is elsewhere say what that source
// says (D-152).
func TestRefLists(t *testing.T) {
	var types []string
	for typ := model.Int64; typ <= model.Bytes; typ++ {
		types = append(types, typ.String())
	}
	if got := ref.FieldTypes.Names(); !slices.Equal(got, types) {
		t.Errorf("ref.FieldTypes: %v, model.Type: %v", got, types)
	}
	if got, want := ref.TemplateFuncs.Names(), slices.Sorted(maps.Keys(funcNames)); !slices.Equal(slices.Sorted(slices.Values(got)), want) {
		t.Errorf("ref.TemplateFuncs: %v, funcNames: %v", got, want)
	}
	// The route constructors: the runtime's funcs that return a Route.
	f, err := parser.ParseFile(token.NewFileSet(), "../../route.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var constructors []string
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Type.Results != nil && len(fd.Type.Results.List) == 1 {
			if id, ok := fd.Type.Results.List[0].Type.(*ast.Ident); ok && id.Name == "Route" {
				constructors = append(constructors, fd.Name.Name)
			}
		}
	}
	if got := ref.Methods.Names(); !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(constructors))) {
		t.Errorf("ref.Methods: %v, route.go: %v", got, constructors)
	}
	for _, k := range maago.FileKinds() {
		if accept := ref.FormTags.Items[slices.Index(ref.FormTags.Names(), "accept")].Text; !strings.Contains(accept, k) {
			t.Errorf("accept says not %s: %s", k, accept)
		}
	}
}
