package mapper

import (
	"cmp"
	"fmt"
	"go/token"
	"slices"
	"strconv"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/project"
)

// Axis is a parameter some {condition} looks at, with the values that
// tell its lines apart.
type Axis struct {
	Param  string
	Values []string // the choices; or absent/present, false/true, empty/non-empty
}

// Variant is one SQL text of a query, and the combinations that give it
// (rule 5).
type Variant struct {
	Pieces []Piece
	// Args are the scalar parameters in placeholder order: Args[i] is
	// number i+1. The slices follow, each element its own number, in the
	// order of Slices.
	Args   []string
	Slices []string
	Combos [][]int // for each axis, the index of its value
}

// Piece is SQL, a scalar's placeholder, or the place of a slice's
// placeholders: one of SQL, Arg and Slice is set.
type Piece struct {
	SQL   string
	Arg   int // placeholder number, from 1
	Slice string
	// Pos is where the piece comes from in the query file. A piece of SQL
	// can hold several source lines, and starts with a line break,
	// indentation or a Lead that the source has elsewhere, so Pos is
	// exact only to the line.
	Pos token.Position
}

const (
	maxCombos   = 256
	maxVariants = 64
)

// Variants returns every combination of the query's condition values and
// the distinct SQL texts they give, and reports the errors of rule 5 that
// need no database. q must have been read without error.
func (q *Query) Variants() ([]Axis, []Variant, []diag.Diagnostic) {
	ds := q.guaranteed()
	axes := q.axes()
	n := 1
	for _, a := range axes {
		n *= len(a.Values)
	}
	if n > maxCombos {
		return nil, nil, append(ds, diag.New(q.NamePos, fmt.Sprintf("query %s has %d combinations of condition values, over %d", q.Name, n, maxCombos),
			"split the query, or compute conditions in Go and pass bools"))
	}
	var vs []Variant
	index := map[string]int{} // by text
	reported := map[token.Position]bool{}
	for c := range n {
		combo := make([]int, len(axes))
		for i, rest := len(axes)-1, c; i >= 0; i-- { // the first axis turns slowest
			combo[i], rest = rest%len(axes[i].Values), rest/len(axes[i].Values)
		}
		v, empty := q.render(axes, combo)
		for _, open := range empty {
			if !reported[open.Pos] {
				reported[open.Pos] = true
				ds = append(ds, emptyRegion(open, describe(axes, combo)))
			}
		}
		key := v.key()
		i, ok := index[key]
		if !ok {
			i, index[key] = len(vs), len(vs)
			vs = append(vs, v)
		}
		vs[i].Combos = append(vs[i].Combos, combo)
	}
	if len(vs) > maxVariants {
		ds = append(ds, diag.New(q.NamePos, fmt.Sprintf("query %s has %d distinct SQL texts, over %d", q.Name, len(vs), maxVariants),
			"split the query, or compute conditions in Go and pass bools"))
	}
	if ds != nil {
		return nil, nil, ds
	}
	return axes, vs, nil
}

func emptyRegion(open Line, combo string) diag.Diagnostic {
	if strings.EqualFold(open.Keyword, "SET") {
		return diag.New(open.Pos, fmt.Sprintf("SET { } is empty when %s", combo),
			"make one line of SET { } unconditional, so the UPDATE always sets something")
	}
	return diag.New(open.Pos, fmt.Sprintf("WHERE { } of %s is empty when %s, so it would touch every row", cmp.Or(open.Verb, "this statement"), combo),
		"make one line of WHERE { } unconditional, or check in Go before calling the query")
}

// guaranteed reports optional parameters used on a line whose condition
// does not require them.
func (q *Query) guaranteed() []diag.Diagnostic {
	var ds []diag.Diagnostic
	for _, l := range q.Lines {
		for _, p := range l.Parts {
			if p.Param == "" || !q.param(p.Param).Optional || requires(l.Cond, p.Param) {
				continue
			}
			ds = append(ds, diag.New(p.Pos, fmt.Sprintf("optional parameter %s is used on a line that does not require {%s}", p.Param, p.Param),
				fmt.Sprintf("start the line with {%s}, or with {%s & …}; or make %s required", p.Param, p.Param, p.Param)))
		}
	}
	return ds
}

// requires reports whether c holds only when name is present.
func requires(c *Cond, name string) bool {
	if c == nil || c.Or && len(c.Terms) > 1 {
		return false
	}
	for _, t := range c.Terms {
		if t.Name == name && !t.Not && t.Value == "" {
			return true
		}
	}
	return false
}

func (q *Query) param(name string) *Param {
	for i := range q.Params {
		if q.Params[i].Name == name {
			return &q.Params[i]
		}
	}
	panic("mapper: undeclared parameter " + name)
}

// axes are the parameters the conditions look at, in declaration order.
func (q *Query) axes() []Axis {
	seen := map[string]bool{}
	for _, l := range q.Lines {
		if l.Cond != nil {
			for _, t := range l.Cond.Terms {
				seen[t.Name] = true
			}
		}
	}
	var axes []Axis
	for _, p := range q.Params {
		if !seen[p.Name] {
			continue
		}
		a := Axis{Param: p.Name}
		switch {
		case p.Type.Choices != nil:
			a.Values = p.Type.Choices
		case p.Optional:
			a.Values = []string{"absent", "present"}
		case p.Type.Slice:
			a.Values = []string{"empty", "non-empty"}
		case p.Type.Base == model.Bool:
			a.Values = []string{"false", "true"}
		}
		axes = append(axes, a)
	}
	return axes
}

func describe(axes []Axis, combo []int) string {
	parts := make([]string, len(axes))
	for i, a := range axes {
		sep := " "
		if a.Values[0] != "absent" && a.Values[0] != "false" && a.Values[0] != "empty" {
			sep = "="
		}
		parts[i] = a.Param + sep + a.Values[combo[i]]
	}
	return strings.Join(parts, ", ")
}

// holds evaluates c under combo.
func holds(c *Cond, axes []Axis, combo []int) bool {
	if c == nil {
		return true
	}
	for _, t := range c.Terms {
		var v string
		for i, a := range axes {
			if a.Param == t.Name {
				v = a.Values[combo[i]]
			}
		}
		ok := v == t.Value
		if t.Value == "" {
			ok = v == "present" || v == "true" || v == "non-empty"
			ok = ok != t.Not
		}
		if ok == c.Or {
			return ok
		}
	}
	return !c.Or
}

// render builds the text of one combination. It also returns the regions
// left empty that must not be: SET { } always, WHERE { } unless its
// statement is a SELECT.
func (q *Query) render(axes []Axis, combo []int) (Variant, []Line) {
	r := &renderer{q: q}
	var empty []Line
	for i := 0; i < len(q.Lines); i++ {
		l := q.Lines[i]
		if l.Kind != Open {
			if holds(l.Cond, axes, combo) {
				r.line(l, l.Lead, false)
			}
			continue
		}
		var in []Line
		for i++; q.Lines[i].Kind != Close; i++ {
			if holds(q.Lines[i].Cond, axes, combo) {
				in = append(in, q.Lines[i])
			}
		}
		if len(in) == 0 {
			if strings.EqualFold(l.Keyword, "SET") || l.Verb != "SELECT" {
				empty = append(empty, l)
			}
			continue
		}
		r.newLine(l)
		r.text(l.Keyword, l.Pos)
		for k, il := range in {
			lead := il.Lead
			if k == 0 {
				lead = ""
			}
			r.line(il, lead, il.Trail && k < len(in)-1)
		}
	}
	r.flush()
	return r.v, empty
}

type renderer struct {
	q     *Query
	v     Variant
	b     strings.Builder
	start token.Position // of the text in b
	row   int            // source line of the last text written
}

func (r *renderer) text(s string, pos token.Position) {
	if r.b.Len() == 0 {
		r.start = pos
	}
	r.b.WriteString(s)
}

func (r *renderer) flush() {
	if r.b.Len() > 0 {
		r.v.Pieces = append(r.v.Pieces, Piece{SQL: r.b.String(), Pos: r.start})
		r.b.Reset()
	}
}

// newLine separates what comes from l's source line from what came
// before: a space on the same line, else a line break and l's indentation.
func (r *renderer) newLine(l Line) {
	switch {
	case r.row == 0:
	case r.row == l.Pos.Line:
		r.text(" ", l.Pos)
	default:
		r.text("\n"+r.q.indent(l.Pos), l.Pos)
	}
	r.row = l.Pos.Line
}

func (r *renderer) line(l Line, lead string, comma bool) {
	r.newLine(l)
	if lead != "" {
		r.text(lead+" ", l.Pos)
	}
	for _, p := range l.Parts {
		switch {
		case p.Param == "":
			r.text(p.SQL, p.Pos)
		case r.q.param(p.Param).Type.Slice:
			r.flush()
			r.v.Pieces = append(r.v.Pieces, Piece{Slice: p.Param, Pos: p.Pos})
			if !slices.Contains(r.v.Slices, p.Param) {
				r.v.Slices = append(r.v.Slices, p.Param)
			}
		default:
			n := slices.Index(r.v.Args, p.Param)
			if n < 0 {
				n = len(r.v.Args)
				r.v.Args = append(r.v.Args, p.Param)
			}
			r.flush()
			r.v.Pieces = append(r.v.Pieces, Piece{Arg: n + 1, Pos: p.Pos})
		}
	}
	if comma {
		r.text(",", l.Pos)
	}
}

// indent is the space before the first character of pos's source line.
func (q *Query) indent(pos token.Position) string {
	start := pos.Offset - (pos.Column - 1)
	line := q.Src[start:]
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// key tells variants apart: the same text with the same arguments.
func (v Variant) key() string {
	return v.SQL(project.Postgres, 1) + "\x00" + strings.Join(v.Args, ",") + "\x00" + strings.Join(v.Slices, ",")
}

// SQL is the text of v for d, with n placeholders for each slice. n is at
// least 1: an empty slice has no SQL (IN () is not SQL), which the
// generated code reports before it picks a text.
func (v Variant) SQL(d project.Dialect, n int) string {
	if n < 1 {
		panic("mapper: a slice needs at least one element")
	}
	mark := "$"
	if d == project.SQLite {
		mark = "?"
	}
	var b strings.Builder
	for _, p := range v.Pieces {
		switch {
		case p.Arg > 0:
			b.WriteString(mark + strconv.Itoa(p.Arg))
		case p.Slice != "":
			first := len(v.Args) + slices.Index(v.Slices, p.Slice)*n + 1
			for k := range n {
				if k > 0 {
					b.WriteString(", ")
				}
				b.WriteString(mark + strconv.Itoa(first+k))
			}
		default:
			b.WriteString(p.SQL)
		}
	}
	return b.String()
}
