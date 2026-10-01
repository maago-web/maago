package mapper

import (
	"fmt"
	"go/token"
	"slices"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/practicedb"
)

// Result is the Go type of a query's rows (D-23).
type Result struct {
	Name   string
	Pos    token.Position // the -- name: of the first query that gave the name
	Model  *model.Model   // the model; nil for a new type
	Fields []ResultField  // one per result column, in column order, the order of Scan
}

// ResultField is where a result column goes: for a model, one of its
// fields; for a new type, a field of it.
type ResultField struct {
	Name, Column string // Go field name, result column name
	Type         model.Type
	Pointer      bool // the column may be NULL (D-75)
}

// Results works out the result type of each query of the app at dir,
// from the columns task 4's Describe gave (D-23, D-104). A query with
// :exec, or without columns because Describe reported errors, has none.
// Queries naming the same new type share one *Result, and must give it
// the same shape. typeName names a PostgreSQL type OID, for messages.
func Results(apps []*model.App, dir string, qs []*Query, cols [][]practicedb.Column, typeName func(uint32) string) ([]*Result, []diag.Diagnostic) {
	var app *model.App
	tables := map[string]*model.Model{} // every model's, by lower-case table name
	for _, a := range apps {
		if a.Dir == dir {
			app = a
		}
		for _, m := range a.Models {
			tables[strings.ToLower(m.Table)] = m
		}
	}
	results := make([]*Result, len(qs))
	shared := map[string]*Result{}
	var ds []diag.Diagnostic
	for i, q := range qs {
		if q.Kind == Exec || cols[i] == nil {
			continue
		}
		r, rds := q.result(app, tables, cols[i], typeName)
		if rds != nil {
			ds = append(ds, rds...)
			continue
		}
		if r.Model == nil {
			if prev, ok := shared[r.Name]; ok {
				if !slices.Equal(prev.Fields, r.Fields) {
					ds = append(ds, diag.New(q.NamePos, fmt.Sprintf("result type %s has another shape in %s: %s", r.Name, prev.Pos.Filename, firstDifference(prev.Fields, r.Fields)),
						"select the same columns in the same order as there, or give this result another name"))
					continue
				}
				r = prev
			}
			shared[r.Name] = r
		}
		results[i] = r
	}
	return results, ds
}

func firstDifference(a, b []ResultField) string {
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return fmt.Sprintf("field %d is %s there, %s here", i+1, fieldString(a[i]), fieldString(b[i]))
		}
	}
	return fmt.Sprintf("%d fields there, %d here", len(a), len(b))
}

func fieldString(f ResultField) string { return f.Name + " " + fieldType(f.Type, f.Pointer) }

// result works out q's result type.
func (q *Query) result(app *model.App, tables map[string]*model.Model, cols []practicedb.Column, typeName func(uint32) string) (*Result, []diag.Diagnostic) {
	j, ds := q.joins()
	if ds != nil {
		return nil, ds
	}
	r := &Result{Name: q.Result, Pos: q.NamePos}
	src := func(c practicedb.Column) *model.Field {
		m := tables[strings.ToLower(c.Table)]
		if j.noSource || m == nil {
			return nil
		}
		for _, f := range m.Fields {
			if f.Column == c.Origin {
				return f
			}
		}
		return nil
	}
	if r.Model = modelNamed(app, q.Result); r.Model != nil {
		return r, q.modelResult(r, cols, src, j)
	}
	return r, q.newResult(r, cols, src, j, typeName)
}

// modelNamed is the model of app, which may be nil, named name: a query
// naming it as its result gets rows of it, and any other name is a new
// type (D-23).
func modelNamed(app *model.App, name string) *model.Model {
	if app == nil {
		return nil
	}
	for _, m := range app.Models {
		if m.Name == name {
			return m
		}
	}
	return nil
}

func (q *Query) modelResult(r *Result, cols []practicedb.Column, src func(practicedb.Column) *model.Field, j joins) []diag.Diagnostic {
	m := r.Model
	if len(q.Columns) > 0 {
		return []diag.Diagnostic{diag.New(q.Columns[0].Pos, fmt.Sprintf("-- column: declares a column of a new result type, but %s is a model", m.Name),
			"remove the -- column: lines; the model's fields give the types")}
	}
	if j.noSource {
		return []diag.Diagnostic{diag.New(q.bodyPos(), fmt.Sprintf("result columns do not match model %s: they come through UNION, INTERSECT or EXCEPT, which hide their source", m.Name),
			"declare a new result type name and its columns with -- column:")}
	}
	var problems []string
	seen := map[string]bool{}
	for _, c := range cols {
		f := src(c)
		switch {
		case c.Table != "" && !strings.EqualFold(c.Table, m.Table):
			problems = append(problems, fmt.Sprintf("%q comes from %s", c.Name, c.Table))
		case f == nil:
			if slices.ContainsFunc(m.Fields, func(f *model.Field) bool { return f.Column == c.Name }) {
				problems = append(problems, fmt.Sprintf("%q is an expression, not %s.%s", c.Name, m.Table, c.Name))
			} else {
				problems = append(problems, fmt.Sprintf("extra %q", c.Name))
			}
		case seen[f.Column]:
			problems = append(problems, fmt.Sprintf("%q is selected twice", f.Column))
		case j.nullable(c):
			problems = append(problems, fmt.Sprintf("%q is on the outer side of a join", c.Name))
		default:
			seen[f.Column] = true
			r.Fields = append(r.Fields, ResultField{f.Name, c.Name, f.Type, f.Pointer})
		}
	}
	var missing []string
	for _, f := range m.Fields {
		// A field another problem already names is not missing too.
		if !seen[f.Column] && !slices.ContainsFunc(cols, func(c practicedb.Column) bool { return src(c) == f || c.Name == f.Column }) {
			missing = append(missing, fmt.Sprintf("%q", f.Column))
		}
	}
	if missing != nil {
		problems = append([]string{"missing " + strings.Join(missing, ", ")}, problems...)
	}
	if problems == nil {
		return nil
	}
	return []diag.Diagnostic{diag.New(q.bodyPos(), fmt.Sprintf("result columns do not match model %s: %s", m.Name, strings.Join(problems, "; ")),
		fmt.Sprintf("select every column of %s, or declare a new result type name (e.g. \"%s %sSummary\")", m.Name, q.Kind, m.Name))}
}

// bodyPos is the start of the SQL's first line.
func (q *Query) bodyPos() token.Position {
	p := q.toks[0].Pos
	p.Offset -= p.Column - 1
	p.Column = 1
	return p
}

func (q *Query) newResult(r *Result, cols []practicedb.Column, src func(practicedb.Column) *model.Field, j joins, typeName func(uint32) string) []diag.Diagnostic {
	var ds []diag.Diagnostic
	names := map[string]bool{}
	for _, c := range cols {
		switch {
		case !sqlName.MatchString(c.Name):
			ds = append(ds, diag.New(q.NamePos, fmt.Sprintf("result column %q is not a lowercase snake_case name, which a Go field needs", c.Name),
				"name it with AS, like AS snake_name"))
		case names[c.Name]:
			ds = append(ds, diag.New(q.NamePos, fmt.Sprintf("result column %s appears twice", c.Name),
				"give one of them another name with AS"))
		}
		names[c.Name] = true
	}
	if ds != nil {
		return ds
	}
	declared := map[string]Column{}
	for _, d := range q.Columns {
		declared[d.Name] = d
	}
	for _, c := range cols {
		f := src(c)
		name := FieldName(c.Name)
		if f != nil && c.Name == c.Origin {
			name = f.Name
		}
		d, isDeclared := declared[c.Name]
		delete(declared, c.Name)
		if f != nil && !j.nullable(c) {
			if isDeclared {
				ds = append(ds, diag.New(d.Pos, fmt.Sprintf("result column %s needs no declaration", c.Name),
					fmt.Sprintf("remove it; %s is %s.%s (%s)", c.Name, c.Table, c.Origin, fieldType(f.Type, f.Pointer))))
				continue
			}
			r.Fields = append(r.Fields, ResultField{name, c.Name, f.Type, f.Pointer})
			continue
		}
		if !isDeclared {
			why, line := "it is an expression", fmt.Sprintf("-- column: %s TYPE", c.Name)
			switch {
			case j.noSource:
				why = "UNION, INTERSECT and EXCEPT leave no source column"
			case f != nil:
				why = fmt.Sprintf("%s.%s is on the outer side of a join", c.Table, c.Origin)
				line = fmt.Sprintf("-- column: %s? %s", c.Name, f.Type)
			case c.Table != "":
				why = fmt.Sprintf("%s is not a model's table", c.Table)
			}
			ds = append(ds, diag.New(q.NamePos, fmt.Sprintf("result column %s needs a declaration: %s", c.Name, why), "add "+line))
			continue
		}
		if f != nil && d.Type != f.Type {
			ds = append(ds, diag.New(d.Pos, fmt.Sprintf("column %s is declared %s, but %s.%s is %s", c.Name, d.Type, c.Table, c.Origin, f.Type),
				fmt.Sprintf("declare it as %s? %s", c.Name, f.Type)))
			continue
		}
		if c.Type != 0 && !slices.Contains(pgTypes[d.Type].oids, c.Type) {
			fix := fmt.Sprintf("cast the expression to %s, like …::%s", pgTypes[d.Type].name, pgTypes[d.Type].name)
			for t, pt := range pgTypes {
				if slices.Contains(pt.oids, c.Type) {
					fix = "declare it as " + t.String()
				}
			}
			ds = append(ds, diag.New(d.Pos, fmt.Sprintf("column %s is declared %s, but PostgreSQL gives %s", c.Name, d.Type, typeName(c.Type)), fix))
			continue
		}
		r.Fields = append(r.Fields, ResultField{name, c.Name, d.Type, d.Optional})
	}
	byField := map[string]string{} // column, by field name
	for _, f := range r.Fields {
		if other, ok := byField[f.Name]; ok {
			ds = append(ds, diag.New(q.NamePos, fmt.Sprintf("result columns %s and %s both become the field %s", other, f.Column, f.Name),
				"give one of them another name with AS"))
		}
		byField[f.Name] = f.Column
	}
	for _, d := range q.Columns {
		if _, unused := declared[d.Name]; unused {
			ds = append(ds, diag.New(d.Pos, fmt.Sprintf("-- column: %s is not a result column", d.Name),
				"remove it, or select a column of that name"))
		}
	}
	return ds
}

func fieldType(t model.Type, pointer bool) string {
	if pointer {
		return "*" + t.String()
	}
	return t.String()
}

// FieldName is a Go field name for a snake_case column name: each word
// capitalized, and id in capitals, the one initialism D-76 needs undone
// (board_id → BoardID).
func FieldName(column string) string {
	var b strings.Builder
	for _, w := range strings.Split(column, "_") {
		if w == "id" {
			b.WriteString("ID")
		} else if w != "" {
			b.WriteString(strings.ToUpper(w[:1]) + w[1:])
		}
	}
	return b.String()
}

// joins is what a query's joins tell about its result columns (D-104).
type joins struct {
	null     map[string]bool // tables on the outer side of a LEFT JOIN, lower case
	all      bool            // a RIGHT or FULL JOIN: every source may be NULL
	noSource bool            // UNION, INTERSECT or EXCEPT: no column has a source
}

func (j joins) nullable(c practicedb.Column) bool {
	return c.Table != "" && (j.all || j.null[strings.ToLower(c.Table)])
}

// joins reads the joins of the whole statement, subqueries and CTEs
// included, since the drivers give sources through them. It reports the
// shapes whose NULL side is not a table it can name.
func (q *Query) joins() (joins, []diag.Diagnostic) {
	j := joins{null: map[string]bool{}}
	toks := q.toks
	ctes := map[string]bool{}
	word := func(i int) string {
		if i >= 0 && i < len(toks) && toks[i].Kind == dialect.Word {
			return strings.ToUpper(toks[i].Text)
		}
		return ""
	}
	fail := func(i int, why string) (joins, []diag.Diagnostic) {
		return joins{}, []diag.Diagnostic{diag.New(toks[i].Pos, "cannot tell which result columns may be NULL: "+why+" (D-104)",
			"join the table itself, with ON, or split the query")}
	}
	for i := range toks {
		switch word(i) {
		case "WITH":
			for k := i + 1; k < len(toks); {
				if word(k) == "RECURSIVE" {
					k++
				}
				if k >= len(toks) || toks[k].Kind != dialect.Word && toks[k].Kind != dialect.Quoted {
					break
				}
				ctes[strings.ToLower(dialect.Name(toks[k]))] = true
				k++
				if k < len(toks) && is(toks[k], "(") { // column names
					k = dialect.Closing(toks, k) + 1
				}
				for k < len(toks) && !is(toks[k], "(") { // AS [NOT] MATERIALIZED
					k++
				}
				if k < len(toks) {
					k = dialect.Closing(toks, k) + 1
				}
				if k >= len(toks) || !is(toks[k], ",") {
					break
				}
				k++
			}
		case "UNION", "INTERSECT", "EXCEPT":
			j.noSource = true
		case "NATURAL":
			return fail(i, "a NATURAL join")
		case "FROM":
			if i+1 < len(toks) && is(toks[i+1], "(") {
				if w := word(i + 2); w != "SELECT" && w != "WITH" && w != "VALUES" {
					return fail(i+1, "a join in parentheses")
				}
			}
		case "JOIN":
			k := i - 1
			if word(k) == "OUTER" {
				k--
			}
			switch word(k) {
			case "RIGHT", "FULL":
				j.all = true
			case "LEFT":
				k = i + 1
				switch {
				case word(k) == "LATERAL":
					return fail(k, "a LEFT JOIN LATERAL")
				case k < len(toks) && is(toks[k], "("):
					return fail(k, "a LEFT JOIN of a subquery")
				}
				if word(k) == "ONLY" {
					k++
				}
				if k >= len(toks) || toks[k].Kind != dialect.Word && toks[k].Kind != dialect.Quoted {
					return fail(i, "a LEFT JOIN not followed by a table name")
				}
				for k+2 < len(toks) && is(toks[k+1], ".") { // schema.table
					k += 2
				}
				name := strings.ToLower(dialect.Name(toks[k]))
				if ctes[name] {
					return fail(k, "a LEFT JOIN of the CTE "+dialect.Name(toks[k])+", whose columns come from other tables")
				}
				j.null[name] = true
			}
		}
	}
	return j, nil
}
