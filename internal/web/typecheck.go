package web

import (
	"fmt"
	"go/token"
	"go/types"
	"maps"
	"slices"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/gotypes"
	"github.com/maago-web/maago/internal/model"
)

// Types checks each view against the types: the signature D-116 allows,
// the page type, and the page's templates. It fills what the adapters
// need. Views ran before, and the routes it rejected have no view.
func Types(root string, fset *token.FileSet, res *gotypes.Result, apps []*App) []diag.Diagnostic {
	routes := map[string][]param{} // every route of the project, for {{url}}
	statics := map[string]*App{}   // the apps with static files, by package name, for {{static}}
	for _, app := range apps {
		if len(app.Static) > 0 {
			statics[app.Name] = app
		}
		for _, r := range app.Routes {
			if r.view == nil {
				continue
			}
			ps := r.view.wildcards
			if r.view.escape { // it reads r.PathValue; URLFunc prints a number too
				ps = slices.Clone(ps)
				for i := range ps {
					ps[i].typ = "string or int"
				}
			}
			routes[r.Name] = ps
		}
	}
	var ds []diag.Diagnostic
	for _, app := range apps {
		pkg := res.Packages[app.Dir]
		if pkg == nil {
			continue
		}
		var pages []page
		viewPos := map[*Route]token.Position{}
		failed := false
		for i := range app.Routes {
			r := &app.Routes[i]
			if r.view == nil {
				continue
			}
			obj := pkg.Scope().Lookup(r.View)
			if obj == nil { // the syntax has it, the build does not
				ds = append(ds, diag.New(fset.Position(app.funcs[r.View].Name.Pos()), "view "+r.View+" is in a file go build leaves out",
					"remove the file's build constraint, or move the view to a file without one"))
				failed = true
				continue
			}
			pos := res.Fset.Position(obj.Pos())
			fn, _ := obj.(*types.Func)
			if fn == nil {
				ds = append(ds, diag.New(pos, r.View+" is not a function", "name a function of package "+app.Name))
				failed = true
				continue
			}
			msg, fix := r.view.signature(fn.Type().(*types.Signature), pkg, hasOption(r.Options, "API"))
			if msg != "" {
				ds = append(ds, diag.New(pos, fmt.Sprintf("view %s %s", r.View, msg), fix))
				failed = true
				continue
			}
			if r.view.escape || hasOption(r.Options, "API") {
				if r.view.escape && hasOption(r.Options, "Block") { // with API, Check said it
					ds = append(ds, diag.New(fset.Position(r.Pos), "route "+r.Name+" renders a block, but its view returns no page", "remove maago.Block"))
				}
				continue
			}
			r.view.template = model.Snake(r.view.page) + ".html"
			viewPos[r] = pos
			pages = append(pages, page{route: r, typ: pkg.Scope().Lookup(r.view.page).Type().(*types.Named), label: r.view.page, multipart: app.readsFile(r)})
		}
		ds = append(ds, validates(res, pkg, app)...)
		if !failed { // a template is walked with the page types, which must be right
			ds = append(ds, checkTemplates(fset, root, app.Dir, pkg, pages, routes, statics, viewPos)...)
		}
	}
	return ds
}

func hasOption(opts []Option, name string) bool {
	for _, o := range opts {
		if o.Name == name {
			return true
		}
	}
	return false
}

// signature checks sig against the view's shape, and records its page
// type. It returns what is wrong, and how to fix it.
func (v *view) signature(sig *types.Signature, pkg *types.Package, api bool) (msg, fix string) {
	const shapes = "write func(ctx context.Context, in In) (Page, error), func(ctx context.Context, in In) (maago.Result[Page], error), or func(w http.ResponseWriter, r *http.Request)"
	ps, rs := sig.Params(), sig.Results()
	if v.escape {
		if !named(ps.At(0).Type(), "net/http", "ResponseWriter") || !isPointerTo(ps.At(1).Type(), "net/http", "Request") {
			return "takes " + ps.At(0).Type().String() + " and " + ps.At(1).Type().String() + ", not an http.ResponseWriter and an *http.Request", shapes
		}
		return "", ""
	}
	if !named(ps.At(0).Type(), "context", "Context") {
		return "takes " + types.TypeString(ps.At(0).Type(), types.RelativeTo(pkg)) + " first, not a context.Context", shapes
	}
	if !types.Identical(rs.At(1).Type(), types.Universe.Lookup("error").Type()) {
		return "returns " + types.TypeString(rs.At(1).Type(), types.RelativeTo(pkg)) + " second, not an error", shapes
	}
	page := rs.At(0).Type()
	if n, ok := page.(*types.Named); ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == maagoPath && n.Obj().Name() == "Result" {
		v.result, page = true, n.TypeArgs().At(0)
	}
	if api {
		return "", "" // any type is JSON
	}
	n, ok := page.(*types.Named)
	if !ok || n.Obj().Pkg() != pkg || n.TypeArgs().Len() > 0 {
		return "returns " + types.TypeString(page, types.RelativeTo(pkg)) + ", which names no template",
			"return a struct type declared in package " + pkg.Name() + ": its name names the template (PostDetailPage → templates/post_detail_page.html)"
	}
	if _, ok := n.Underlying().(*types.Struct); !ok {
		return "returns " + n.Obj().Name() + ", which is not a struct", "make the page a struct type"
	}
	v.page = n.Obj().Name()
	return "", ""
}

func named(t types.Type, pkg, name string) bool {
	n, ok := t.(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == pkg && n.Obj().Name() == name
}

func isPointerTo(t types.Type, pkg, name string) bool {
	p, ok := t.(*types.Pointer)
	return ok && named(p.Elem(), pkg, name)
}

// validates finds each form's Validate: a method of the form's pointer,
// which takes and returns nothing (D-119).
func validates(res *gotypes.Result, pkg *types.Package, app *App) []diag.Diagnostic {
	var ds []diag.Diagnostic
	for _, name := range slices.Sorted(maps.Keys(app.forms)) {
		f := app.forms[name]
		tn, _ := pkg.Scope().Lookup(name).(*types.TypeName)
		if tn == nil {
			continue
		}
		obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(tn.Type()), true, pkg, "Validate")
		if obj == nil {
			continue
		}
		_, isFunc := obj.(*types.Func)
		if sig, _ := obj.Type().(*types.Signature); !isFunc || sig.Params().Len() > 0 || sig.Results().Len() > 0 {
			ds = append(ds, diag.New(res.Fset.Position(obj.Pos()), fmt.Sprintf("Validate of form %s is not a method func()", name),
				"declare func (f *"+name+") Validate(): it sets a field's Error or calls f.AddError; a check that needs the database goes in the view (D-119)"))
			continue
		}
		if _, ptr := obj.Type().(*types.Signature).Recv().Type().(*types.Pointer); !ptr {
			ds = append(ds, diag.New(res.Fset.Position(obj.Pos()), fmt.Sprintf("Validate of form %s has a value receiver: the errors it sets are lost", name),
				"declare it func (f *"+name+") Validate()"))
			continue
		}
		f.validate = true
	}
	return ds
}
