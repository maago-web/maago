package mapper

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/practicedb"
)

// pgTypes are the PostgreSQL types a parameter of each type may be read
// as: the types the models' columns get (M1), so a parameter compared
// with a column of its own field type always fits.
var pgTypes = map[model.Type]struct {
	oids []uint32
	name string // for a cast
}{
	model.Int64:   {[]uint32{20}, "bigint"},
	model.Int32:   {[]uint32{23}, "integer"},
	model.String:  {[]uint32{25, 1043, 1042}, "text"},
	model.Float64: {[]uint32{701}, "double precision"},
	model.Bool:    {[]uint32{16}, "boolean"},
	model.Time:    {[]uint32{1184}, "timestamptz"},
	model.Bytes:   {[]uint32{17}, "bytea"},
}

// Describe prepares every variant of q on db, slices with one element and
// with two, and returns the result columns, which every variant must
// share. It reports what the database finds wrong, at the place in the
// query file and with the combination that shows it (rule 5). The
// parameter types are checked for PostgreSQL only: SQLite does not know
// them.
func Describe(ctx context.Context, db *practicedb.DB, q *Query, axes []Axis, vs []Variant) ([]practicedb.Column, []diag.Diagnostic) {
	d := db.Dialect()
	var ds []diag.Diagnostic
	seen := map[string]bool{} // what was reported, so each is reported once
	report := func(key string, dg diag.Diagnostic) {
		if !seen[key] {
			seen[key] = true
			ds = append(ds, dg)
		}
	}
	var cols []practicedb.Column
	colsFrom := -1              // the variant cols come from
	var differ *diag.Diagnostic // reported only if every variant prepares
	failed := false
	for vi, v := range vs {
		when := ""
		if len(axes) > 0 {
			when = " (when " + describe(axes, v.Combos[0]) + ")"
		}
		ns := []int{1}
		if len(v.Slices) > 0 {
			ns = []int{1, 2}
		}
		for _, n := range ns {
			desc, err, fatal := db.Describe(ctx, v.SQL(d, n))
			if fatal != nil {
				return nil, fatal
			}
			if err != nil {
				failed = true
				pos := q.NamePos
				if c := practicedb.ErrorPosition(err); c > 0 {
					pos = v.Position(d, n, c)
				}
				msg := diag.OneLine(err.Error())
				report(fmt.Sprint(pos, msg), diag.New(pos, msg+when, "fix the SQL"))
				continue
			}
			for i, oid := range desc.Params {
				name := ""
				if i < len(v.Args) {
					name = v.Args[i]
				} else {
					name = v.Slices[(i-len(v.Args))/n]
				}
				if dg, ok := paramType(ctx, db, q, v, name, oid); !ok {
					report(fmt.Sprint("param ", name, oid), dg)
				}
			}
			switch {
			case colsFrom < 0:
				cols, colsFrom = desc.Columns, vi
			case differ == nil && !slices.Equal(cols, desc.Columns):
				dg := diag.New(q.NamePos,
					fmt.Sprintf("result columns differ: %s gives %s, but %s gives %s",
						describe(axes, vs[colsFrom].Combos[0]), columnNames(cols), describe(axes, v.Combos[0]), columnNames(desc.Columns)),
					"keep the result columns out of {conditions}; write another query for other columns")
				differ = &dg
			}
		}
	}
	if failed {
		return nil, ds
	}
	if differ != nil {
		ds = append(ds, *differ)
	}
	if q.Kind == Exec && len(cols) > 0 {
		ds = append(ds, diag.New(q.NamePos, fmt.Sprintf(":exec query %s returns columns %s, which would be thrown away", q.Name, columnNames(cols)),
			"use :one or :many, or remove RETURNING"))
	}
	if ds != nil {
		return nil, ds
	}
	return cols, nil
}

// paramType checks the type PostgreSQL reads parameter name as.
func paramType(ctx context.Context, db *practicedb.DB, q *Query, v Variant, name string, oid uint32) (diag.Diagnostic, bool) {
	p := q.param(name)
	base := p.Type.Base
	if p.Type.Choices != nil {
		base = model.String
	}
	if slices.Contains(pgTypes[base].oids, oid) {
		return diag.Diagnostic{}, true
	}
	pos := q.NamePos
	for _, pc := range v.Pieces {
		if pc.Slice == name || pc.Arg > 0 && v.Args[pc.Arg-1] == name {
			pos = pc.Pos
			break
		}
	}
	typ := typeString(Type{Base: base, Slice: p.Type.Slice})
	// Declaring the type PostgreSQL reads is the fix that fits a
	// comparison with a column; a cast there makes another error.
	fix := ""
	for t, pt := range pgTypes {
		if slices.Contains(pt.oids, oid) && p.Type.Choices == nil {
			fix = "declare it as " + typeString(Type{Base: t, Slice: p.Type.Slice}) + ", or "
		}
	}
	switch {
	case p.Type.Slice || p.Type.Choices != nil:
		fix += "compare it with a column of type " + pgTypes[base].name
	default:
		fix += fmt.Sprintf("write :%s::%s if the SQL needs a %s there", name, pgTypes[base].name, pgTypes[base].name)
	}
	return diag.New(pos, fmt.Sprintf("parameter %s is %s, but PostgreSQL reads :%s here as %s", name, typ, name, db.TypeName(ctx, oid)), fix), false
}

func columnNames(cols []practicedb.Column) string {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.Name
		if c.Table != "" {
			names[i] += " from " + c.Table + "." + c.Origin
		}
	}
	return "(" + strings.Join(names, ", ") + ")"
}
