package mapper

import (
	"fmt"
	"go/token"
	"slices"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/model"
)

// Line is one line of a query's body (rules 3 and 4). A line is the
// tokens that start on one source line.
type Line struct {
	Pos     token.Position // of the first token after the condition and the Lead
	Cond    *Cond          // nil if the line is always in
	Kind    LineKind
	Keyword string // Open: WHERE or SET, as written
	Verb    string // Open: SELECT, INSERT, UPDATE or DELETE, whichever is last at depth 0 before it; "" if none
	Lead    string // a line in WHERE { }: its leading AND or OR, as written; not in Parts
	Trail   bool   // a line in SET { }: it ends with a comma, which is not in Parts
	Parts   []Part
}

// LineKind is what a Line is.
type LineKind int

const (
	Text  LineKind = iota // SQL
	Open                  // WHERE { or SET {
	Close                 // }
)

func (k LineKind) String() string { return [...]string{"text", "open", "close"}[k] }

// Part is a piece of a Text line: SQL as written, or a :name binding.
// Joined, with each binding written as :name, the parts of a line are
// the source from its first token to its last, byte for byte.
type Part struct {
	SQL   string
	Param string
	Pos   token.Position
}

// Cond is a {condition} at the start of a line (rule 3).
type Cond struct {
	Pos   token.Position // of the {
	Or    bool           // terms joined by |; otherwise by &
	Terms []Term
}

// Term is name, !name or name=value.
type Term struct {
	Pos         token.Position
	Not         bool
	Name, Value string
}

// body reads the SQL after the header into q.Lines.
func (q *Query) body() []diag.Diagnostic {
	stmts, comments, ds := dialect.SplitComments(q.File, q.Src)
	if ds != nil {
		return ds
	}
	b := &bodyReader{q: q, params: map[string]*Param{}, used: map[string]bool{}, maybe: map[string]bool{}}
	for i := range q.Params {
		b.params[q.Params[i].Name] = &q.Params[i]
	}
	for _, c := range comments {
		if c.Pos.Offset < q.HeaderEnd {
			continue
		}
		if start, end, colon := headerKey(c.Text, 2); colon >= 0 {
			p := c.Pos
			p.Column += start
			b.ds = append(b.ds, diag.New(p, fmt.Sprintf("-- %s: comment after the SQL", c.Text[start:end]),
				`move header lines above the SQL; start other comments with something other than "word:"`))
		}
	}
	switch {
	case len(stmts) == 0:
		line := strings.Count(q.Src[:q.HeaderEnd], "\n") + 1
		b.ds = append(b.ds, diag.New(token.Position{Filename: q.File, Line: line, Column: 1}, "query has no SQL", "write the SQL after the header"))
		return b.ds
	case len(stmts) > 1:
		b.ds = append(b.ds, diag.New(stmts[1].Pos, "second statement in a query file", "keep one statement per file; put the other in a file of its own"))
		return b.ds
	}
	toks := stmts[0].Tokens
	q.toks = toks
	for i := 0; i < len(toks); {
		j := i + 1
		for j < len(toks) && toks[j].Pos.Line == toks[i].Pos.Line {
			j++
		}
		var prev *dialect.Token
		if i > 0 {
			prev = &toks[i-1]
		}
		n := len(b.ds)
		b.verb = verb(toks[i:j], b.depth, b.verb) // a WHERE { on the line comes after its tokens
		b.line(toks[i:j], prev)
		if len(b.ds) > n { // the line was not read to its end
			for _, t := range toks[i:j] {
				b.maybe[t.Text] = true
			}
		}
		b.depth += depth(toks[i:j])
		i = j
	}
	if b.open != nil {
		b.errf(b.open.Pos, "close it with } on a line of its own", "%s { is not closed", b.open.Keyword)
	}
	for _, p := range q.Params {
		if !b.used[p.Name] && !b.maybe[p.Name] {
			b.errf(p.Pos, "use it as :"+p.Name+" or in a {condition}, or remove the -- param: line", "parameter %s is not used", p.Name)
		}
	}
	return b.ds
}

type bodyReader struct {
	q        *Query
	params   map[string]*Param
	used     map[string]bool
	maybe    map[string]bool // words on lines with errors, which may be uses
	ds       []diag.Diagnostic
	depth    int    // of parentheses, before the line
	open     *Line  // the open region, if any
	inside   int    // lines in the open region
	rejected int    // region openings reported as errors and not yet closed
	verb     string // the last statement verb at depth 0, up to the end of the line
}

func (b *bodyReader) errf(pos token.Position, fix, format string, args ...any) {
	b.ds = append(b.ds, diag.New(pos, fmt.Sprintf(format, args...), fix))
}

func depth(toks []dialect.Token) int {
	d := 0
	for _, t := range toks {
		switch {
		case t.Kind != dialect.Punct:
		case t.Text == "(":
			d++
		case t.Text == ")":
			d--
		}
	}
	return d
}

// verb returns the last of SELECT, INSERT, UPDATE and DELETE in toks at
// depth 0, where toks start at depth d; or v if there is none.
func verb(toks []dialect.Token, d int, v string) string {
	for _, t := range toks {
		switch {
		case is(t, "("):
			d++
		case is(t, ")"):
			d--
		case d == 0 && t.Kind == dialect.Word:
			if u := strings.ToUpper(t.Text); u == "SELECT" || u == "INSERT" || u == "UPDATE" || u == "DELETE" {
				v = u
			}
		}
	}
	return v
}

func is(t dialect.Token, punct string) bool { return t.Kind == dialect.Punct && t.Text == punct }

// line reads the tokens of one line. prev is the token before them.
func (b *bodyReader) line(toks []dialect.Token, prev *dialect.Token) {
	var cond *Cond
	if is(toks[0], "{") {
		end := slices.IndexFunc(toks, func(t dialect.Token) bool { return is(t, "}") })
		if end < 0 {
			if len(toks) == 1 && prev != nil && prev.Kind == dialect.Word && isRegionWord(prev.Text) {
				b.errf(toks[0].Pos, fmt.Sprintf("write %s { on one line", prev.Text), "{ on the line after %s", prev.Text)
				b.rejected++
				return
			}
			b.errf(toks[0].Pos, "close the condition with } on the same line", "condition is not closed")
			return
		}
		var ok bool
		if cond, ok = b.cond(toks[0].Pos, toks[1:end]); !ok {
			return
		}
		toks = toks[end+1:]
		if len(toks) == 0 {
			b.errf(cond.Pos, "write the SQL the condition keeps after it, on the same line", "condition has no SQL after it")
			return
		}
	}
	last := len(toks) - 1
	switch {
	case len(toks) == 1 && is(toks[0], "}"):
		switch {
		case b.rejected > 0: // closes an opening already reported
			b.rejected--
			return
		case b.open == nil:
			b.errf(toks[0].Pos, "remove it, or open a region with WHERE { or SET {", "} closes no region")
			return
		case cond != nil:
			b.errf(cond.Pos, "remove the condition", "condition on the } that closes %s {", b.open.Keyword)
		case b.inside == 0:
			b.errf(b.open.Pos, "remove the empty region", "%s { } is empty", b.open.Keyword)
		}
		b.q.Lines = append(b.q.Lines, Line{Pos: toks[0].Pos, Kind: Close})
		b.open = nil
		return
	case is(toks[last], "{") && last > 0 && toks[last-1].Kind == dialect.Word && isRegionWord(toks[last-1].Text):
		kw := toks[last-1]
		b.rejected++ // until it is accepted below
		switch {
		case cond != nil:
			b.errf(cond.Pos, "remove the condition; put conditions on the lines inside", "condition on the line that opens %s {", kw.Text)
			return
		case b.open != nil:
			b.errf(kw.Pos, fmt.Sprintf("close %s { first, or move this into Go", b.open.Keyword), "%s { inside %s { at line %d", kw.Text, b.open.Keyword, b.open.Pos.Line)
			return
		case b.depth+depth(toks) != 0:
			b.errf(kw.Pos, fmt.Sprintf("write %s without { inside parentheses", kw.Text), "%s { inside parentheses", kw.Text)
			return
		}
		if last > 1 && !b.text(nil, toks[:last-1]) {
			return
		}
		b.rejected--
		open := Line{Pos: kw.Pos, Kind: Open, Keyword: kw.Text, Verb: b.verb}
		b.q.Lines = append(b.q.Lines, open)
		b.open, b.inside = &open, 0
		return
	}
	b.text(cond, toks)
}

func isRegionWord(s string) bool { return strings.EqualFold(s, "WHERE") || strings.EqualFold(s, "SET") }

// text reads a line of SQL. It reports false after an error.
func (b *bodyReader) text(cond *Cond, toks []dialect.Token) bool {
	for _, t := range toks {
		switch {
		case is(t, "{"):
			b.errf(t.Pos, "use { only for a {condition} at the start of a line, or right after WHERE or SET at the end of a line", "unexpected {")
			return false
		case is(t, "}"):
			b.errf(t.Pos, "close a region with } alone on its line, or remove it", "unexpected }")
			return false
		}
	}
	l := Line{Pos: toks[0].Pos, Cond: cond}
	if b.open != nil {
		b.inside++
		first, last := toks[0], toks[len(toks)-1]
		switch {
		case strings.EqualFold(b.open.Keyword, "WHERE") && first.Kind == dialect.Word && (strings.EqualFold(first.Text, "AND") || strings.EqualFold(first.Text, "OR")):
			l.Lead, toks = first.Text, toks[1:]
			if len(toks) == 0 {
				b.errf(first.Pos, "write the condition after it, or remove the line", "%s with nothing after it", first.Text)
				return false
			}
		case strings.EqualFold(b.open.Keyword, "SET") && is(last, ","):
			l.Trail, toks = true, toks[:len(toks)-1]
			if len(toks) == 0 {
				b.errf(last.Pos, "write the assignment before it, or remove the line", "comma with nothing before it")
				return false
			}
		}
	}
	src := b.q.Src
	l.Pos = toks[0].Pos
	from := toks[0].Pos // where the next SQL part starts
	undeclared := false
	for i, t := range toks {
		var next dialect.Token
		glued := false // next starts right where t ends
		if i+1 < len(toks) {
			next = toks[i+1]
			glued = next.Pos.Offset == t.Pos.Offset+len(t.Text)
		}
		switch {
		case is(t, "?"):
			b.errf(t.Pos, "write values as :name (rule 2); for the PostgreSQL jsonb operators ? ?| ?& use jsonb_exists, jsonb_exists_any, jsonb_exists_all", "placeholder ? in a query")
			return false
		case (is(t, "$") || is(t, "@")) && glued && next.Kind == dialect.Word:
			b.errf(t.Pos, "write values as :name (rule 2)", "placeholder %s%s in a query", t.Text, next.Text)
			return false
		// SQLite reads any :word as its own parameter, even :2 or :한글.
		case is(t, ":") && glued && next.Kind == dialect.Word &&
			!(i > 0 && is(toks[i-1], ":") && toks[i-1].Pos.Offset+1 == t.Pos.Offset):
			p, ok := b.params[next.Text]
			if !ok { // the line's others too, in the same run
				b.unknownParam(t.Pos, next.Text, "; if it is not a parameter, put a space after the colon, like a[1: 2]")
				undeclared = true
				continue
			}
			b.used[p.Name] = true
			if from.Offset < t.Pos.Offset {
				l.Parts = append(l.Parts, Part{SQL: src[from.Offset:t.Pos.Offset], Pos: from})
			}
			l.Parts = append(l.Parts, Part{Param: p.Name, Pos: t.Pos})
			from = next.Pos // a word is on one line
			from.Offset += len(next.Text)
			from.Column += len(next.Text)
		}
	}
	if undeclared {
		return false
	}
	last := toks[len(toks)-1]
	if end := last.Pos.Offset + len(last.Text); from.Offset < end {
		l.Parts = append(l.Parts, Part{SQL: src[from.Offset:end], Pos: from})
	}
	b.q.Lines = append(b.q.Lines, l)
	return true
}

func isNameStart(c byte) bool { return c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }

func (b *bodyReader) unknownParam(pos token.Position, name, more string) {
	names := make([]string, len(b.q.Params))
	for i, p := range b.q.Params {
		names[i] = p.Name
	}
	fix := "declare it with -- param: " + name + " TYPE, TYPE one of int64, int32, float64, string, bool, time.Time (" + name + "? for an optional one)" + more
	if !sqlName.MatchString(name) {
		fix = "parameter names are lowercase snake_case" + more
	}
	if s := diag.Suggest(name, names); s != "" {
		fix = fmt.Sprintf("did you mean %q? or %s", s, fix)
	}
	b.errf(pos, fix, "parameter %s is not declared", name)
}

// cond reads the tokens between { and } (rule 3).
func (b *bodyReader) cond(pos token.Position, toks []dialect.Token) (*Cond, bool) {
	c := &Cond{Pos: pos}
	if len(toks) == 0 {
		b.errf(pos, "write a condition, like {status}, or remove the {}", "empty condition")
		return nil, false
	}
	var op string
	for i := 0; i < len(toks); {
		if i > 0 {
			t := toks[i]
			if !is(t, "&") && !is(t, "|") {
				b.errf(t.Pos, "join terms with & or |", "unexpected %q in condition", t.Text)
				return nil, false
			}
			if op != "" && t.Text != op {
				b.errf(t.Pos, "use only & or only | in one condition; compute anything more complex in Go and pass a bool (rule 3)", "condition mixes & and |")
				return nil, false
			}
			op = t.Text
			if i++; i == len(toks) {
				b.errf(t.Pos, "write a term after it, or remove it", "condition ends with %s", op)
				return nil, false
			}
		}
		term, n, ok := b.term(toks[i:])
		if !ok {
			return nil, false
		}
		c.Terms = append(c.Terms, term)
		i += n
	}
	c.Or = op == "|"
	return c, true
}

// term reads name, !name or name=value from the start of toks, and
// returns how many tokens it took.
func (b *bodyReader) term(toks []dialect.Token) (Term, int, bool) {
	fail := func(t dialect.Token, fix, format string, args ...any) (Term, int, bool) {
		b.errf(t.Pos, fix, format, args...)
		return Term{}, 0, false
	}
	const form = "write name, !name or name=value"
	t := Term{Pos: toks[0].Pos}
	i := 0
	if is(toks[0], "!") {
		t.Not, i = true, 1
	}
	if i < len(toks) && is(toks[i], "(") {
		return fail(toks[i], "compute the condition in Go and pass a bool (rule 3)", "parentheses in a condition")
	}
	if i == len(toks) || toks[i].Kind != dialect.Word {
		at := toks[len(toks)-1]
		if i < len(toks) {
			at = toks[i]
		}
		return fail(at, form, "condition needs a parameter name here")
	}
	name := toks[i]
	t.Name = name.Text
	i++
	if i < len(toks) && is(toks[i], "=") {
		if t.Not {
			return fail(toks[0], "write {name=choice} for each of the other choices, joined by |", "!name=value is not a condition")
		}
		if i+1 == len(toks) || toks[i+1].Kind != dialect.Word {
			return fail(toks[i], form, "= needs a value after it")
		}
		t.Value = toks[i+1].Text
		i += 2
	}
	p, ok := b.params[t.Name]
	if !ok {
		b.unknownParam(name.Pos, t.Name, "")
		return Term{}, 0, false
	}
	b.used[p.Name] = true
	switch {
	case t.Value != "" && p.Type.Choices == nil:
		return fail(name, fmt.Sprintf("write {%s}", p.Name), "{%s=…} needs choices, but %s is %s", p.Name, p.Name, typeString(p.Type))
	case t.Value != "" && !slices.Contains(p.Type.Choices, t.Value):
		fix := "allowed: " + strings.Join(p.Type.Choices, ", ")
		if s := diag.Suggest(t.Value, p.Type.Choices); s != "" {
			fix = fmt.Sprintf("did you mean %q? %s", s, fix)
		}
		return fail(toks[i-1], fix, "%s is not a choice of %s", t.Value, p.Name)
	case t.Value == "" && p.Type.Choices != nil:
		return fail(name, fmt.Sprintf("compare it with a choice, like {%s=%s}", p.Name, p.Type.Choices[0]), "{%s} is always true: %s always has a choice", p.Name, p.Name)
	case t.Value == "" && !p.Optional && !p.Type.Slice && p.Type.Base != model.Bool:
		return fail(name, fmt.Sprintf("declare it optional (%s? %s), or remove the condition", p.Name, typeString(p.Type)), "{%s} is always true: %s is required", p.Name, p.Name)
	}
	return t, i, true
}

// typeString is a type other than choices, as a header writes it.
func typeString(t Type) string {
	if t.Slice {
		return "[]" + t.Base.String()
	}
	return t.Base.String()
}
