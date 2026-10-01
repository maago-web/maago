package web

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/model"
)

// form is a form struct (D-119): a struct that embeds maago.Form, its
// fields maago.Field[T].
type form struct {
	name     string
	model    string // model=X on the embedded maago.Form; "" for a plain form
	fields   []formField
	pos      token.Pos
	validate bool // has the method Validate; Types fills it
	used     bool // an In reads it
}

// formField is one field of a form and its rules, from its tags or its
// model's.
type formField struct {
	name, typ          string // typ: string, int64, int32, float64, bool or time.Time
	label, help        string
	choices            []string
	optional           bool
	min, max           int
	minValue, maxValue string
}

var (
	formTypes = []string{"string", "int64", "int32", "float64", "bool", "time.Time"}
	formKeys  = []string{"label", "help", "choices", "min", "max", "min_value", "max_value", "optional"}
)

// readFormField reads an In field tagged maago:"form": a form type of the
// package, which Forms checks.
func readFormField(fset *token.FileSet, f *ast.Field, name *ast.Ident, o model.Option, ds *[]diag.Diagnostic) (inField, bool) {
	errAt := func(pos token.Pos, msg, fix string) { *ds = append(*ds, diag.New(fset.Position(pos), msg, fix)) }
	if o.Kind != model.NoValue {
		errAt(o.KeyPos, "maago:\"form\" takes no value", `write maago:"form"; the form's keys are its fields' names`)
		return inField{}, false
	}
	if star, ok := f.Type.(*ast.StarExpr); ok {
		errAt(f.Type.Pos(), "form "+name.Name+" is a pointer", "declare it as "+types.ExprString(star.X)+"; the adapter fills it in place")
		return inField{}, false
	}
	id, ok := f.Type.(*ast.Ident)
	if !ok {
		errAt(f.Type.Pos(), fmt.Sprintf("form %s is not of a type of this package: %s", name.Name, types.ExprString(f.Type)),
			"declare the form struct in this package, embedding maago.Form")
		return inField{}, false
	}
	return inField{name: name.Name, source: "form", typ: id.Name, pos: name.Pos()}, true
}

// Forms reads the form structs of each app and checks their tags, and
// that each In's form field names one. It runs after Views. models is
// the app with models at a directory, for model forms.
func Forms(fset *token.FileSet, apps []*App, models func(dir string) *model.App) []diag.Diagnostic {
	var ds []diag.Diagnostic
	for _, app := range apps {
		app.forms = map[string]*form{}
		var specs []*ast.TypeSpec
		for _, ts := range app.types {
			specs = append(specs, ts)
		}
		slices.SortFunc(specs, func(a, b *ast.TypeSpec) int { return int(a.Pos() - b.Pos()) })
		for _, ts := range specs {
			f, fds := readForm(fset, ts, app.maago[ts.Name.Name], models(app.Dir))
			ds = append(ds, fds...)
			if f != nil {
				app.forms[f.name] = f
			}
		}
		for _, r := range app.Routes {
			if r.view == nil {
				continue
			}
			for _, in := range r.view.fields {
				if in.source != "form" {
					continue
				}
				if r.Method == "DELETE" {
					ds = append(ds, diag.New(fset.Position(in.pos), fmt.Sprintf("%s.%s reads a form, but route %s is a DELETE, whose body Go does not parse as a form", r.view.in, in.name, r.Name),
						"send the form with POST; DELETE takes its values from the path"))
				}
				if f := app.forms[in.typ]; f != nil {
					f.used = true
				} else if _, declared := app.types[in.typ]; declared {
					ds = append(ds, diag.New(fset.Position(in.pos), fmt.Sprintf("%s.%s reads form %s, which does not embed maago.Form", r.view.in, in.name, in.typ),
						"embed maago.Form in "+in.typ+", and make its fields maago.Field[T]"))
				} else {
					ds = append(ds, diag.New(fset.Position(in.pos), fmt.Sprintf("%s.%s reads form %s, which package %s does not declare", r.view.in, in.name, in.typ, app.Name),
						"declare type "+in.typ+" struct{ maago.Form; … } in package "+app.Name))
				}
			}
		}
	}
	return ds
}

// readForm reads ts if it is a form: nil and nothing if it embeds no
// maago.Form. A form that has errors is reported and still returned, so
// the In that reads it is not reported too.
func readForm(fset *token.FileSet, ts *ast.TypeSpec, names []string, models *model.App) (*form, []diag.Diagnostic) {
	st, ok := ts.Type.(*ast.StructType)
	if !ok {
		return nil, nil
	}
	embed := slices.IndexFunc(st.Fields.List, func(f *ast.Field) bool { return len(f.Names) == 0 && isMaago(f.Type, "Form", names) })
	if embed < 0 {
		return nil, nil
	}
	var ds []diag.Diagnostic
	errAt := func(pos token.Pos, msg, fix string) { ds = append(ds, diag.New(fset.Position(pos), msg, fix)) }
	f := &form{name: ts.Name.Name, pos: ts.Name.Pos()}
	if ts.TypeParams != nil {
		errAt(ts.Name.Pos(), "form "+f.name+" has type parameters", "declare the form without them")
	}
	opts, tds := model.ParseTag(fset, st.Fields.List[embed].Tag)
	ds = append(ds, tds...)
	var modelPos token.Pos
	for _, o := range opts {
		switch {
		case o.Key != "model":
			errAt(o.KeyPos, fmt.Sprintf("unknown key %q in the tag of maago.Form", o.Key), `the form's own tag takes model=X: maago:"model=Board"`)
		case o.Kind != model.Bare || len(o.Values) != 1:
			errAt(o.KeyPos, "model takes one model name", `write maago:"model=Board"`)
		default:
			f.model, modelPos = o.Values[0], o.ValuePos[0]
		}
	}
	var m *model.Model
	if f.model != "" {
		var names []string
		if models != nil {
			for _, mm := range models.Models {
				names = append(names, mm.Name)
				if mm.Name == f.model {
					m = mm
				}
			}
		}
		if m == nil {
			fix := "name a model of this package"
			if s := diag.Suggest(f.model, names); s != "" {
				fix = fmt.Sprintf("did you mean %q?", s)
			}
			errAt(modelPos, fmt.Sprintf("form %s names model %s, which this package does not declare", f.name, f.model), fix)
		}
	}
	for i, fd := range st.Fields.List {
		if i == embed {
			continue
		}
		if len(fd.Names) == 0 {
			errAt(fd.Pos(), "form "+f.name+" embeds "+types.ExprString(fd.Type), "embed only maago.Form; name the fields")
			continue
		}
		typ, ok := fieldType(fd.Type, names)
		if !ok {
			errAt(fd.Type.Pos(), fmt.Sprintf("form field %s is %s, not maago.Field[T]", fd.Names[0].Name, types.ExprString(fd.Type)),
				"use maago.Field[T] with T one of "+strings.Join(formTypes, ", ")+"; a file field comes with M7")
			continue
		}
		for _, name := range fd.Names {
			if !name.IsExported() {
				errAt(name.Pos(), "form field "+name.Name+" is not exported", "export it: templates draw the form's fields")
				continue
			}
			ff := formField{name: name.Name, typ: typ, label: name.Name}
			if m != nil {
				ds = append(ds, modelRules(fset, &ff, fd, name, m)...)
			} else {
				ds = append(ds, tagRules(fset, &ff, fd, name)...)
			}
			f.fields = append(f.fields, ff)
		}
	}
	return f, ds
}

// fieldType is T of maago.Field[T], if T is one a form reads.
func fieldType(e ast.Expr, names []string) (string, bool) {
	ix, ok := e.(*ast.IndexExpr)
	if !ok || !isMaago(ix.X, "Field", names) {
		return "", false
	}
	t := types.ExprString(ix.Index)
	return t, slices.Contains(formTypes, t)
}

// tagRules reads the rules of a plain form's field from its tag.
func tagRules(fset *token.FileSet, ff *formField, fd *ast.Field, name *ast.Ident) []diag.Diagnostic {
	opts, ds := model.ParseTag(fset, fd.Tag)
	if len(ds) > 0 {
		return ds
	}
	errAt := func(pos token.Pos, msg, fix string) { ds = append(ds, diag.New(fset.Position(pos), msg, fix)) }
	number := ff.typ == "int64" || ff.typ == "int32" || ff.typ == "float64"
	where := "form field " + name.Name
	shape := func(o model.Option, kind model.Kind, n int, fix string) bool {
		if o.Kind != kind || n > 0 && len(o.Values) != n {
			errAt(o.KeyPos, fmt.Sprintf("%s of %s is not written as the key takes it", o.Key, where), fix)
			return false
		}
		return true
	}
	for i, o := range opts {
		if slices.ContainsFunc(opts[:i], func(p model.Option) bool { return p.Key == o.Key }) {
			errAt(o.KeyPos, fmt.Sprintf("%s of %s is given twice", o.Key, where), "keep one")
			continue
		}
		switch o.Key {
		case "label", "help":
			if shape(o, model.Text, 1, "write "+o.Key+"='text'") {
				if o.Key == "label" {
					ff.label = o.Values[0]
				} else {
					ff.help = o.Values[0]
				}
			}
		case "choices":
			switch {
			case ff.typ != "string":
				errAt(o.KeyPos, fmt.Sprintf("choices apply to strings, but %s is %s", where, ff.typ), "remove choices, or make the field a string")
			case shape(o, model.Bare, -1, "list them with /, like choices=draft/published"):
				ff.choices = o.Values
			}
		case "min", "max":
			switch {
			case ff.typ != "string":
				fix := "remove " + o.Key
				if number {
					fix = fmt.Sprintf("use %s_value=…: min and max count the characters of a string", o.Key)
				}
				errAt(o.KeyPos, fmt.Sprintf("%s applies to strings, but %s is %s", o.Key, where, ff.typ), fix)
			case shape(o, model.Bare, 1, "write "+o.Key+"=N, a number of characters"):
				n, err := strconv.Atoi(o.Values[0])
				if err != nil || n < 1 {
					errAt(o.ValuePos[0], fmt.Sprintf("%s of %s is %q, not a number of characters", o.Key, where, o.Values[0]), "write a whole number from 1")
				} else if o.Key == "min" {
					ff.min = n
				} else {
					ff.max = n
				}
			}
		case "min_value", "max_value":
			switch {
			case !number:
				errAt(o.KeyPos, fmt.Sprintf("%s applies to numbers, but %s is %s", o.Key, where, ff.typ), "remove it; min and max bound a string's length")
			case shape(o, model.Bare, 1, "write "+o.Key+"=N"):
				v := o.Values[0]
				if !parses(ff.typ, v) {
					errAt(o.ValuePos[0], fmt.Sprintf("%s of %s is %q, not a %s", o.Key, where, v, ff.typ), "write a number of the field's type")
				} else if o.Key == "min_value" {
					ff.minValue = v
				} else {
					ff.maxValue = v
				}
			}
		case "optional":
			switch {
			case ff.typ == "bool":
				errAt(o.KeyPos, where+" is a checkbox, which is never required", "remove optional")
			case shape(o, model.NoValue, 0, "write optional, with no value"):
				ff.optional = true
			}
		default:
			fix := "allowed: " + strings.Join(formKeys, ", ")
			if m := diag.Suggest(o.Key, formKeys); m != "" {
				fix = fmt.Sprintf("did you mean %q? %s", m, fix)
			}
			errAt(o.KeyPos, fmt.Sprintf("unknown key %q in the tag of %s", o.Key, where), fix)
		}
	}
	if ff.min > 0 && ff.max > 0 && ff.min > ff.max {
		errAt(name.Pos(), fmt.Sprintf("%s has min=%d above max=%d", where, ff.min, ff.max), "swap them")
	}
	if ff.minValue != "" && ff.maxValue != "" {
		lo, _ := strconv.ParseFloat(ff.minValue, 64)
		hi, _ := strconv.ParseFloat(ff.maxValue, 64)
		if lo > hi {
			errAt(name.Pos(), fmt.Sprintf("%s has min_value=%s above max_value=%s", where, ff.minValue, ff.maxValue), "swap them")
		}
	}
	return ds
}

// parses reports whether v is a value of the number type typ.
func parses(typ, v string) bool {
	var err error
	switch typ {
	case "int64":
		_, err = strconv.ParseInt(v, 10, 64)
	case "int32":
		_, err = strconv.ParseInt(v, 10, 32)
	default:
		_, err = strconv.ParseFloat(v, 64)
	}
	return err == nil
}

// modelRules takes a model form's field's rules from its model field: the
// field's tag may only add optional (D-119).
func modelRules(fset *token.FileSet, ff *formField, fd *ast.Field, name *ast.Ident, m *model.Model) []diag.Diagnostic {
	opts, ds := model.ParseTag(fset, fd.Tag)
	if len(ds) > 0 {
		return ds
	}
	errAt := func(pos token.Pos, msg, fix string) { ds = append(ds, diag.New(fset.Position(pos), msg, fix)) }
	for _, o := range opts {
		switch {
		case o.Key == "optional" && ff.typ == "bool":
			errAt(o.KeyPos, "form field "+name.Name+" is a checkbox, which is never required", "remove optional")
		case o.Key == "optional" && o.Kind == model.NoValue:
			ff.optional = true
		default:
			errAt(o.KeyPos, fmt.Sprintf("form field %s takes its rules from model %s", name.Name, m.Name),
				"remove "+o.Key+"; a model form's field may only add optional, and model "+m.Name+" declares the rest")
		}
	}
	i := slices.IndexFunc(m.Fields, func(f *model.Field) bool { return f.Name == name.Name })
	if i < 0 {
		var names []string
		for _, f := range m.Fields {
			names = append(names, f.Name)
		}
		fix := "name a field of model " + m.Name
		if s := diag.Suggest(name.Name, names); s != "" {
			fix = fmt.Sprintf("did you mean %q?", s)
		}
		errAt(name.Pos(), fmt.Sprintf("model %s has no field %s", m.Name, name.Name), fix)
		return ds
	}
	mf := m.Fields[i]
	switch {
	case mf.Name == "ID" || mf.Auto != "":
		errAt(name.Pos(), fmt.Sprintf("field %s of model %s is not for a form to set", name.Name, m.Name), "remove it; the database or the generated code sets it")
	case mf.Type.String() != ff.typ:
		errAt(fd.Type.Pos(), fmt.Sprintf("form field %s is maago.Field[%s], but model %s has it as %s", name.Name, ff.typ, m.Name, mf.Type),
			fmt.Sprintf("make it maago.Field[%s]", mf.Type))
	}
	if mf.Label != "" {
		ff.label = mf.Label
	}
	ff.help, ff.choices, ff.max = mf.Help, mf.Choices, mf.Max
	return ds
}
