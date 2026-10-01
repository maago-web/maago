package web

import (
	"bytes"
	"embed"
	"fmt"
	"go/format"
	"go/token"
	"net/url"
	"slices"
	"strings"
	"text/template"

	"github.com/maago-web/maago/internal/mapper"
	"github.com/maago-web/maago/internal/pipeline"
)

//go:embed templates
var templateFiles embed.FS

var templates = template.Must(template.ParseFS(templateFiles, "templates/*.go.tmpl"))

// Imports are the packages the files web generates may import, beside
// maago (D-66).
var Imports = []string{"embed", "html/template", "net/http", "strconv"}

// URLsFile is the urls_gen.go of app: its URL functions, and its App.
// Before the views are checked, stub declares App without a value, so
// that main and the views type-check; after, App holds the routes.
func URLsFile(app *App, stub bool) []byte {
	imports := map[string]bool{}
	var rs []map[string]any
	for _, r := range app.Routes {
		var params []string
		expr := `""`
		for _, seg := range strings.Split(r.Path, "/")[1:] {
			part := fmt.Sprintf("%q", "/"+url.PathEscape(seg))
			switch {
			case seg == "{$}":
				part = `"/"`
			case strings.HasPrefix(seg, "{"):
				p := r.view.param(strings.Trim(seg, "{}"))
				params = append(params, p.name+" "+p.typ)
				switch p.typ {
				case "string":
					part = `"/" + maago.PathSegment(` + p.name + `)`
				case "int64":
					part, imports["strconv"] = `"/" + strconv.FormatInt(`+p.name+`, 10)`, true
				default:
					part, imports["strconv"] = `"/" + strconv.FormatInt(int64(`+p.name+`), 10)`, true
				}
			}
			expr += " + " + part
		}
		expr = strings.ReplaceAll(strings.TrimPrefix(expr, `"" + `), `" + "`, "")
		rs = append(rs, map[string]any{
			"Method": r.Method, "Path": r.Path, "Name": r.Name, "API": hasOption(r.Options, "API"),
			"URL": mapper.FieldName(r.Name) + "URL", "Serve": "serve" + mapper.FieldName(r.Name),
			"Params": strings.Join(params, ", "), "URLExpr": expr,
		})
	}
	return execute("urls.go.tmpl", imports, map[string]any{"Package": app.Name, "ImportPath": app.ImportPath, "Stub": stub, "Routes": rs})
}

// param is the URL function's parameter for wildcard name.
func (v *view) param(name string) param {
	i := slices.IndexFunc(v.wildcards, func(p param) bool { return p.name == name })
	return v.wildcards[i]
}

// ViewsFile is the views_gen.go of app: an adapter for each route, which
// reads the request into the view's In, calls the view, and writes its
// page (D-116).
func ViewsFile(app *App, apps []*App) []byte {
	imports := map[string]bool{"net/http": true}
	html := false
	var rs []map[string]any
	var pages []string
	for _, r := range app.Routes {
		v := r.view
		in := v.in
		if in == "" {
			in = "struct{}"
		}
		var fields []map[string]any
		query := false
		for _, f := range v.fields {
			query = query || f.source == "query"
			fields = append(fields, map[string]any{"Name": f.name, "Source": f.source, "Key": f.key, "Pointer": f.pointer,
				"Parser": strings.ToUpper(f.typ[:1]) + f.typ[1:]})
		}
		api := hasOption(r.Options, "API")
		html = html || !v.escape && !api
		rs = append(rs, map[string]any{
			"Name": r.Name, "View": r.View, "Serve": "serve" + mapper.FieldName(r.Name), "Escape": v.escape,
			"In": in, "Fields": fields, "Query": query, "Result": v.result, "API": api, "Template": v.template, "Block": blockOf(&r),
		})
		if v.template != "" && !slices.Contains(pages, v.template) {
			pages = append(pages, v.template)
		}
	}
	if html {
		imports["embed"], imports["html/template"] = true, true
	}
	imports["maago"] = slices.ContainsFunc(app.Routes, func(r Route) bool { return !r.view.escape })
	var all []map[string]string
	for _, a := range apps {
		for _, r := range a.Routes {
			all = append(all, map[string]string{"Name": r.Name, "Path": r.Path})
		}
	}
	slices.SortFunc(all, func(a, b map[string]string) int { return strings.Compare(a["Name"], b["Name"]) })
	return execute("views.go.tmpl", imports, map[string]any{"Package": app.Name, "HTML": html, "Routes": rs, "Pages": pages, "AllRoutes": all})
}

// execute writes the file name. imports are the standard packages it uses;
// maago it imports unless imports["maago"] is false.
func execute(name string, imports map[string]bool, data map[string]any) []byte {
	data["Marker"] = pipeline.Marker
	var list []string
	for _, i := range Imports {
		if imports[i] {
			list = append(list, i)
		}
	}
	if m, ok := imports["maago"]; m || !ok {
		list = append(list, "", "github.com/maago-web/maago")
	}
	data["Imports"] = list
	var b bytes.Buffer
	if err := templates.ExecuteTemplate(&b, name, data); err != nil {
		panic(err)
	}
	out, err := format.Source(b.Bytes())
	if err != nil { // a bug of maago: never write what is not Go
		panic(fmt.Sprintf("web: %s is not Go: %v\n%s", name, err, b.Bytes()))
	}
	return out
}

// More is what the files of app declare, for the name checks of the
// mapper, which see every generator's names in a package at once: App,
// each route's URL function and adapter, and the templates of an app with
// HTML pages.
func More(fset *token.FileSet, app *App) *mapper.More {
	at := fset.Position(app.Pos)
	m := &mapper.More{Imports: Imports, Names: []mapper.Name{{Name: "App", By: "the routes of app " + app.Name, Pos: at,
		Fix: "rename what else declares App; maago generates each app's App (D-114)"}}}
	seen := map[string]bool{}
	html := false
	for _, r := range app.Routes {
		html = html || r.view != nil && !r.view.escape && !hasOption(r.Options, "API")
		if seen[r.Name] {
			continue // Check reported the name taken
		}
		seen[r.Name] = true
		by, pos, fix := "route "+r.Name, fset.Position(r.NamePos), "rename the route, or what else declares that name"
		for _, n := range []string{mapper.FieldName(r.Name) + "URL", "serve" + mapper.FieldName(r.Name)} {
			m.Names = append(m.Names, mapper.Name{Name: n, By: by, Pos: pos, Fix: fix})
		}
	}
	if html {
		for _, n := range []string{"maagoTemplateFiles", "maagoRoutes", "maagoPages"} {
			m.Names = append(m.Names, mapper.Name{Name: n, By: "the templates of app " + app.Name, Pos: at, Fix: "rename what else declares " + n})
		}
	}
	return m
}
