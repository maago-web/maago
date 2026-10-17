package web

import (
	"bytes"
	"cmp"
	"embed"
	"fmt"
	"go/format"
	"go/token"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/maago-web/maago/internal/mapper"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/pipeline"
)

//go:embed templates
var templateFiles embed.FS

var templates = template.Must(template.ParseFS(templateFiles, "templates/*.go.tmpl"))

// Imports are the packages the files web generates may import, beside
// maago (D-66).
var Imports = []string{"context", "embed", "html/template", "net/http", "net/url", "strconv"}

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
			"LoginRequired": r.view != nil && r.view.user && !r.view.userPtr, "User": r.view != nil && r.view.user,
			"Files": app.readsFile(&r),
			"URL":   mapper.FieldName(r.Name) + "URL", "Serve": "serve" + mapper.FieldName(r.Name),
			"Params": strings.Join(params, ", "), "URLExpr": expr,
		})
	}
	return execute("urls.go.tmpl", imports, map[string]any{"Package": app.Name, "ImportPath": app.ImportPath, "Stub": stub, "Routes": rs, "Static": len(app.Static) > 0})
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
	userImport := ""
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
				"Parser": strings.ToUpper(f.typ[:1]) + f.typ[1:], "Form": f.typ, "Keys": keysOf(app.forms[f.typ]), "Multipart": app.forms[f.typ] != nil && app.forms[f.typ].hasFile()})
		}
		api := hasOption(r.Options, "API")
		html = html || !v.escape && !api
		rs = append(rs, map[string]any{
			"Name": r.Name, "View": r.View, "Serve": "serve" + mapper.FieldName(r.Name), "Escape": v.escape,
			"In": in, "Fields": fields, "Query": query, "Result": v.result, "API": api, "Template": v.template, "Block": blockOf(&r),
			"User": v.userType, "UserPtr": v.userPtr, "DB": v.db, "Tx": v.tx,
		})
		imports["context"] = imports["context"] || v.tx // the transaction's func
		userImport = cmp.Or(userImport, v.userImport)   // one user type in the project
		if v.template != "" && !slices.Contains(pages, v.template) {
			pages = append(pages, v.template)
		}
	}
	static := len(app.Static) > 0
	if html {
		imports["embed"], imports["html/template"] = true, true
	}
	imports["embed"] = imports["embed"] || static
	imports["maago"] = static || slices.ContainsFunc(app.Routes, func(r Route) bool { return !r.view.escape })
	forms := formsData(app)
	if len(forms) > 0 {
		imports["net/url"] = true // bindXxx takes url.Values
	}
	var all []map[string]string
	for _, a := range apps {
		for _, r := range a.Routes {
			all = append(all, map[string]string{"Name": r.Name, "Path": r.Path})
		}
	}
	slices.SortFunc(all, func(a, b map[string]string) int { return strings.Compare(a["Name"], b["Name"]) })
	return execute("views.go.tmpl", imports, map[string]any{"Package": app.Name, "HTML": html, "Routes": rs, "Pages": pages, "AllRoutes": all, "Forms": forms,
		"Static": static, "Used": slices.Sorted(maps.Keys(app.used)), "UserImport": userImport})
}

// FormsFile is forms_gen.go of app: the Valid method of each form, read
// by an In or not yet, or nil when there is none. The views call Valid, so
// it exists before the type check, unlike views_gen.go.
func FormsFile(app *App) []byte {
	var forms []map[string]any
	for _, name := range slices.Sorted(maps.Keys(app.forms)) {
		var fields []map[string]string
		for _, ff := range app.forms[name].fields {
			fields = append(fields, map[string]string{"Name": ff.name})
		}
		forms = append(forms, map[string]any{"Name": name, "Fields": fields})
	}
	if len(forms) == 0 {
		return nil
	}
	return execute("forms.go.tmpl", map[string]bool{}, map[string]any{"Package": app.Name, "Forms": forms})
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
		Fix: "rename what else declares App; maago generates each app's App (D-114)"},
		{Name: "maagouser", By: "the views' adapters, which import the user's package under it", Pos: at, Fix: "rename what else declares maagouser"}}}
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
	binds := map[string]bool{}
	for _, r := range app.Routes {
		if r.view == nil {
			continue
		}
		for _, f := range r.view.fields {
			if f.source != "form" {
				continue
			}
			names := []string{"bind" + f.typ}
			if hasOption(r.Options, "API") {
				names = append(names, "invalid"+f.typ)
			}
			for _, n := range names {
				if !binds[n] {
					binds[n] = true
					m.Names = append(m.Names, mapper.Name{Name: n, By: "form " + f.typ, Pos: fset.Position(f.pos), Fix: "rename what else declares " + n})
				}
			}
		}
	}
	if len(app.Static) > 0 {
		for _, n := range []string{"maagoStaticFiles", "maagoStatic"} {
			m.Names = append(m.Names, mapper.Name{Name: n, By: "the static files of app " + app.Name, Pos: at, Fix: "rename what else declares " + n})
		}
	}
	if html {
		for _, n := range []string{"maagoTemplateFiles", "maagoRoutes", "maagoPages"} {
			m.Names = append(m.Names, mapper.Name{Name: n, By: "the templates of app " + app.Name, Pos: at, Fix: "rename what else declares " + n})
		}
	}
	return m
}

// formsData is what views.go.tmpl needs of the forms an In reads: each
// field's Bind function and Rule (D-119).
func formsData(app *App) []map[string]any {
	var fs []map[string]any
	for _, name := range slices.Sorted(maps.Keys(app.forms)) {
		f := app.forms[name]
		if !f.used {
			continue
		}
		var fields []map[string]string
		for _, ff := range f.fields {
			arg := "r.Context()"
			if ff.typ == "maago.File" {
				arg = "r" // BindFile reads the files from the request
			}
			fields = append(fields, map[string]string{"Name": ff.name, "Bind": binds[ff.typ], "Arg": arg, "Rule": ruleLit(ff), "Key": strconv.Quote(model.Snake(ff.name))})
		}
		fs = append(fs, map[string]any{"Name": name, "Validate": f.validate, "Fields": fields, "API": f.api})
	}
	return fs
}

// binds are the Bind functions of the form field types.
var binds = map[string]string{"string": "BindString", "int64": "BindInt64", "int32": "BindInt32", "float64": "BindFloat64", "bool": "BindBool", "time.Time": "BindTime", "maago.File": "BindFile"}

// ModelRule is the Bind function and the maago.Rule, as Go, of a model
// form's field for mf: the rules a model form takes from its model
// (D-119), for the admin's forms (D-131).
func ModelRule(mf *model.Field, optional bool) (bind, rule string) {
	ff := formField{name: mf.Name, typ: mf.Type.String(), label: mf.Name, help: mf.Help, choices: mf.Choices, max: mf.Max, optional: optional}
	if mf.Label != "" {
		ff.label = mf.Label
	}
	return binds[ff.typ], ruleLit(ff)
}

// ruleLit is ff's maago.Rule as Go: only the parts it has.
func ruleLit(ff formField) string {
	parts := []string{"Name: " + strconv.Quote(model.Snake(ff.name)), "Label: " + strconv.Quote(ff.label)}
	if ff.help != "" {
		parts = append(parts, "Help: "+strconv.Quote(ff.help))
	}
	if ff.choices != nil {
		var qs []string
		for _, c := range ff.choices {
			qs = append(qs, strconv.Quote(c))
		}
		parts = append(parts, "Choices: []string{"+strings.Join(qs, ", ")+"}")
	}
	if ff.optional {
		parts = append(parts, "Optional: true")
	}
	if ff.min > 0 {
		parts = append(parts, "Min: "+strconv.Itoa(ff.min))
	}
	if ff.max > 0 {
		parts = append(parts, "Max: "+strconv.Itoa(ff.max))
	}
	if ff.minValue != "" {
		parts = append(parts, "MinValue: "+strconv.Quote(ff.minValue))
	}
	if ff.maxValue != "" {
		parts = append(parts, "MaxValue: "+strconv.Quote(ff.maxValue))
	}
	if ff.accept != nil {
		var qs []string
		for _, k := range ff.accept {
			qs = append(qs, strconv.Quote(k))
		}
		parts = append(parts, "Accept: []string{"+strings.Join(qs, ", ")+"}")
	}
	return "maago.Rule{" + strings.Join(parts, ", ") + "}"
}

// keysOf are the keys of f's fields, quoted for Go.
func keysOf(f *form) []string {
	var keys []string
	if f != nil {
		for _, ff := range f.fields {
			keys = append(keys, strconv.Quote(model.Snake(ff.name)))
		}
	}
	return keys
}
