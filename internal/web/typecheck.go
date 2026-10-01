package web

import (
	"fmt"
	"go/token"
	"go/types"
	"html/template"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/gotypes"
	"github.com/maago-web/maago/internal/model"
)

// Types checks each view against the types: the signature D-116 allows,
// the page type, and the page's template. It fills what the adapters
// need. Views ran before, and the routes it rejected have no view.
func Types(root string, fset *token.FileSet, res *gotypes.Result, apps []*App) []diag.Diagnostic {
	var ds []diag.Diagnostic
	for _, app := range apps {
		pkg := res.Packages[app.Dir]
		if pkg == nil {
			continue
		}
		var set *template.Template // the app's templates, parsed as at run time
		for i := range app.Routes {
			r := &app.Routes[i]
			if r.view == nil {
				continue
			}
			obj := pkg.Scope().Lookup(r.View)
			if obj == nil { // the syntax has it, the build does not
				ds = append(ds, diag.New(fset.Position(app.funcs[r.View].Name.Pos()), "view "+r.View+" is in a file go build leaves out",
					"remove the file's build constraint, or move the view to a file without one"))
				continue
			}
			pos := res.Fset.Position(obj.Pos())
			fn, _ := obj.(*types.Func)
			if fn == nil {
				ds = append(ds, diag.New(pos, r.View+" is not a function", "name a function of package "+app.Name))
				continue
			}
			msg, fix := r.view.signature(fn.Type().(*types.Signature), pkg, hasOption(r.Options, "API"))
			if msg != "" {
				ds = append(ds, diag.New(pos, fmt.Sprintf("view %s %s", r.View, msg), fix))
				continue
			}
			if r.view.escape || hasOption(r.Options, "API") {
				continue
			}
			if set == nil {
				var tds []diag.Diagnostic
				if set, tds = parseTemplates(root, app.Dir); tds != nil {
					ds = append(ds, tds...)
					break // the set does not parse: no page can be looked up in it
				}
			}
			r.view.template = model.Snake(r.view.page) + ".html"
			if set.Lookup(r.view.template) == nil {
				rel := path.Join(app.Dir, "templates", r.view.template)
				ds = append(ds, diag.New(pos, fmt.Sprintf("page %s has no template %s", r.view.page, rel),
					"create "+rel+"; a page type's template is its name in snake_case (D-116)"))
			}
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

// templateError is html/template's "template: NAME:LINE: message".
var templateError = regexp.MustCompile(`^template: ([^:]+):(\d+): (.*)$`)

// parseTemplates parses the app's templates/*.html as one set, the way
// the generated code does when the app starts: a file that does not parse
// would stop the app, whether a page uses it or not. A missing directory
// is an empty set. Names are looked up as written, on any file system.
func parseTemplates(root, dir string) (*template.Template, []diag.Diagnostic) {
	set := template.New("")
	fsys := os.DirFS(filepath.Join(root, filepath.FromSlash(dir), "templates"))
	files, _ := fs.Glob(fsys, "*.html")
	if len(files) == 0 {
		return set, nil
	}
	if _, err := set.ParseFS(fsys, "*.html"); err != nil {
		pos := token.Position{Filename: path.Join(dir, "templates")}
		msg := err.Error()
		if m := templateError.FindStringSubmatch(msg); m != nil {
			pos.Filename = path.Join(dir, "templates", m[1])
			pos.Line, _ = strconv.Atoi(m[2])
			msg = m[3]
		}
		return nil, []diag.Diagnostic{diag.New(pos, diag.OneLine(msg), "fix the template; it is Go's html/template")}
	}
	return set, nil
}
