package admingen

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/mapper"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/project"
	"github.com/maago-web/maago/internal/web"
)

// editable reports whether the admin's form changes f: not hidden, not
// read-only, not set by the database or the CRUD, and not bytes, which
// v0 does not edit (D-132).
func editable(f *model.Field) bool {
	return f.Admin == "" && f.Name != "ID" && f.Auto == "" && f.Type != model.Bytes
}

// blocks is why a new row of m cannot be made in the admin, or "": the
// fields its form leaves out that would be a zero value (D-132).
func blocks(m *model.Model) string {
	var why []string
	for _, f := range m.Fields {
		if editable(f) || f.Name == "ID" || f.Auto != "" || f.Pointer || f.Default != nil {
			continue
		}
		what := "admin=" + f.Admin
		if f.Admin == "" {
			what = "bytes, which the admin does not edit"
		}
		why = append(why, f.Name+" is "+what)
	}
	if why == nil {
		return ""
	}
	return m.Name + ": no adding: " + strings.Join(why, "; ") + ", with no default"
}

// updateSQL is the UPDATE of the admin's form of m: only the fields it
// changes, and the auto=update times, so that it never writes back a
// hidden field another writer changed (D-132). "" when it changes none.
// After the id, a model with a version takes the one the form was opened
// with (D-150).
func updateSQL(m *model.Model, d project.Dialect) (string, []*model.Field) {
	var set []string
	var args []*model.Field
	for _, f := range m.Fields {
		if editable(f) || f.Auto == "update" {
			args = append(args, f)
			set = append(set, dialect.Quote(f.Column)+" = "+mark(d, len(args)))
		}
	}
	if !slices.ContainsFunc(m.Fields, editable) {
		return "", nil
	}
	where := ` WHERE "id" = ` + mark(d, len(args)+1)
	if v := m.Version(); v != nil {
		where += " AND " + dialect.Quote(v.Column) + " = " + mark(d, len(args)+2)
	}
	return "UPDATE " + dialect.Quote(m.Table) + " SET " + strings.Join(set, ", ") + where, args
}

// existsSQL is the check that an fk field's number is of a row.
func existsSQL(f *model.Field, d project.Dialect) string {
	return "SELECT 1 FROM " + dialect.Quote(f.FK) + ` WHERE "id" = ` + mark(d, 1)
}

// rowSQL is the check that m's row of an id is there.
func rowSQL(m *model.Model, d project.Dialect) string {
	return "SELECT 1 FROM " + dialect.Quote(m.Table) + ` WHERE "id" = ` + mark(d, 1)
}

// mark is placeholder n: $n, or ?n for SQLite, numbered so that one
// value can be used twice (D-134).
func mark(d project.Dialect, n int) string {
	if d == project.SQLite {
		return "?" + strconv.Itoa(n)
	}
	return "$" + strconv.Itoa(n)
}

// formCode is the Go of m's Form function. models maps each table to its
// model, as "app.Model".
func formCode(m *model.Model, d project.Dialect, models map[string]string, imports map[string]bool) string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	add := blocks(m) == ""
	update, updateArgs := updateSQL(m, d)
	version := m.Version()
	if update == "" {
		version = nil // nothing to save
	}
	form := "admin.Form{Inputs: in" // its fields but Saved
	w("func(ctx context.Context, db maago.DB, id int64, vals url.Values) (admin.Form, error) {")
	w("var o %s", m.Name)
	if version != nil {
		imports["time"] = true
		form += ", Version: ver"
		w("var ver time.Time")
	}
	w("if id != 0 {")
	w("var err error")
	w("if o, err = Get%s(ctx, db, id); err != nil {", m.Name)
	w("return admin.Form{}, err")
	w("}")
	if version != nil {
		w("if ver, err = admin.Version(vals, o.%s); err != nil {", version.Name)
		w("return admin.Form{}, err")
		w("}")
	}
	w("}")
	var fields []*model.Field
	for _, f := range m.Fields {
		if editable(f) {
			fields = append(fields, f)
		}
	}
	if fields != nil {
		w("var f struct {")
		for _, f := range fields {
			w("%s maago.Field[%s]", f.Name, formType(f))
			if f.Type == model.Time {
				imports["time"] = true
			}
		}
		w("}")
		for _, f := range fields {
			mf := f
			if f.Type == model.Bool && f.Pointer { // a select of yes and no: a checkbox cannot say NULL
				c := *f
				c.Type, c.Choices = model.String, []string{"yes", "no"}
				mf = &c
			}
			bind, rule := web.ModelRule(mf, optional(f))
			w("rt.%s(ctx, vals, &f.%s, %s)", bind, f.Name, rule)
		}
		w("var form maago.Form // the form's own errors")
		w("rt.FinishForm(ctx, &form, vals != nil, true)")
		w("if vals == nil && id != 0 {")
		for _, f := range fields {
			w("f.%s.Raw = admin.Raw(o.%s)", f.Name, f.Name)
		}
		if slices.ContainsFunc(fields, func(f *model.Field) bool { return f.Default != nil }) {
			w("} else if vals == nil { // a new row: the defaults")
			for _, f := range fields {
				if f.Default != nil {
					w("f.%s.Raw = %s", f.Name, strconv.Quote(defaultRaw(f)))
				}
			}
		}
		w("}")
		for _, f := range fields {
			if f.FK == "" {
				continue
			}
			w("if vals != nil && f.%s.Error == \"\" && f.%s.Given {", f.Name, f.Name)
			w("if found, err := admin.Exists(ctx, db, %s, f.%s.Value); err != nil {", mapper.GoString(existsSQL(f, d)), f.Name)
			w("return admin.Form{}, err")
			w("} else if !found {")
			w("rt.NoRow(ctx, &f.%s)", f.Name)
			w("}")
			w("}")
		}
	}
	inputs := func() { // after the save, which may set an error
		w("var in []admin.Input")
		for _, f := range m.Fields {
			label := f.Label
			if label == "" {
				label = f.Name
			}
			switch {
			case f.Admin == "hidden":
			case editable(f):
				w("in = append(in, admin.In(&f.%s, %t, %q))", f.Name, !optional(f) && f.Type != model.Bool, models[f.FK])
			default:
				w("if id != 0 {")
				w("in = append(in, admin.Show(%q, o.%s))", label, f.Name)
				w("}")
			}
		}
	}
	if fields == nil {
		inputs()
		if add { // nothing to fill in: the defaults, the auto times and the new ID
			w("if vals != nil && id == 0 {")
			w("_, err := Insert%s(ctx, db, New%sInput{})", m.Name, m.Name)
			w("return admin.Form{Inputs: in, Saved: err == nil}, err")
			w("}")
		}
		w("return admin.Form{Inputs: in}, nil")
		w("}")
		return strings.TrimSuffix(b.String(), "\n")
	}
	var ok, fails []string
	for _, f := range fields {
		ok = append(ok, "f."+f.Name+".Error == \"\"")
		fails = append(fails, strconv.Quote(f.Name)+": &f."+f.Name+".Error")
	}
	keys := "nil"
	if len(m.Uniques) > 0 {
		keys = "unique" + m.Name
	}
	w("var err error")
	w("if vals != nil && %s {", strings.Join(ok, " && "))
	w("if id == 0 {")
	if add {
		w("_, err = Insert%s(ctx, db, New%sInput{", m.Name, m.Name)
		for _, f := range fields {
			v := value(f)
			if f.Default != nil {
				v = "maago.Set(" + v + ")"
			}
			w("%s: %s,", f.Name, v)
		}
		w("})")
	} else {
		w("return admin.Form{}, admin.NoAdding(%q)", m.Name)
	}
	w("} else {")
	var args []string
	for _, f := range updateArgs {
		v := "now"
		if f.Auto == "" {
			v = value(f)
		} else {
			imports["time"] = true
		}
		args = append(args, sqlArg(d, v, f))
	}
	if slices.ContainsFunc(updateArgs, func(f *model.Field) bool { return f.Auto == "update" }) {
		w("now := time.Now().Truncate(time.Microsecond)")
	}
	args = append(args, "id")
	if version != nil {
		args = append(args, sqlArg(d, "ver", version))
	}
	w("err = rt.Write(ctx, db, %s, func() error {", keys)
	w("return admin.Updated(rt.Exec(ctx, db, %s, %s))", mapper.GoString(update), strings.Join(args, ", "))
	w("})")
	if version != nil {
		w("err = admin.Changed(ctx, db, %s, id, err)", mapper.GoString(rowSQL(m, d)))
	}
	w("}")
	w("if err == nil {")
	w("return admin.Form{Saved: true}, nil")
	w("}")
	w("if rt.Fail(&form, err, map[string]*string{%s}) { // drawn again, with the error", strings.Join(fails, ", "))
	w("err = nil")
	w("}")
	w("}")
	inputs()
	w("return admin.Form{Inputs: in, Errors: form.Errors%s}, err", strings.TrimPrefix(form, "admin.Form{Inputs: in"))
	w("}")
	return strings.TrimSuffix(b.String(), "\n")
}

// optional reports whether f's input may be left empty: a pointer is
// then NULL, and a string whose default is ” is then ”.
func optional(f *model.Field) bool {
	return f.Pointer || f.Type == model.String && f.Default != nil && *f.Default == ""
}

// formType is T of the maago.Field[T] that reads f.
func formType(f *model.Field) string {
	if f.Type == model.Bool && f.Pointer {
		return "string"
	}
	return f.Type.String()
}

// value is the Go of f's value, read from its form field.
func value(f *model.Field) string {
	switch {
	case f.Type == model.Bool && f.Pointer:
		return "admin.Bool(f." + f.Name + ")"
	case f.Pointer:
		return "admin.Ptr(f." + f.Name + ")"
	}
	return "f." + f.Name + ".Value"
}

// sqlArg is v as the driver takes f's column: SQLite keeps a time as
// text (D-105).
func sqlArg(d project.Dialect, v string, f *model.Field) string {
	switch {
	case d != project.SQLite || f.Type != model.Time:
		return v
	case f.Pointer:
		return "rt.SQLiteNullTime(" + v + ")"
	}
	return "rt.SQLiteTime(" + v + ")"
}

// defaultRaw is f's default as its input draws it, for a new row.
func defaultRaw(f *model.Field) string {
	v := *f.Default
	switch f.Type {
	case model.Bool:
		switch {
		case f.Pointer && v == "true":
			return "yes"
		case f.Pointer:
			return "no"
		case v == "true":
			return "on"
		}
		return ""
	case model.String, model.Time:
		return v
	}
	return dialect.Number(f.Type, v)
}
