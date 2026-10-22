package mapper

import (
	"fmt"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/project"
	"github.com/maago-web/maago/internal/ref"
)

// Policy is a read policy of policies.sql (D-155): a condition on the rows
// of a model, which its SQL calls Alias.
type Policy struct {
	File     string // slash path relative to the project root
	Name     string
	Pos      token.Position
	Model    string
	ModelPos token.Position
	Alias    string
	Get      string // the GetX that applies it; "" if none
	GetPos   token.Position
	Params   []Param // its -- param: lines, then the :viewer_ columns it uses
	parts    []policyPart
	broken   bool         // its block has an error: the queries using it are left out
	model    *model.Model // Check finds it
	used     bool         // a query uses it
}

// policyPart is a token of a policy's SQL: as written, a :param, or its
// alias, which a use replaces with the query's. space is whether a gap
// follows it in the source.
type policyPart struct {
	sql, param string
	alias      bool
	space      bool
	pos        token.Position
}

// Policies are the policies of each app, by the app's directory.
type Policies map[string][]*Policy

// viewerPrefix starts the parameters a policy takes from the user.
const viewerPrefix = "viewer_"

// ReadPolicies reads the policies.sql files, as project.PolicyFiles lists
// them. A policy with an error is kept, marked broken, so a query using it
// reports nothing that follows from it.
func ReadPolicies(root string, files []string) (Policies, []diag.Diagnostic) {
	ps := Policies{}
	var ds []diag.Diagnostic
	for _, f := range files {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			ds = append(ds, diag.New(token.Position{Filename: f}, "cannot read file: "+diag.OneLine(err.Error()), "make sure the file can be read"))
			continue
		}
		pols, pds := parsePolicies(f, string(src))
		ds = append(ds, pds...)
		ps[path.Dir(f)] = pols
	}
	return ps, ds
}

// parsePolicies reads one policies.sql: blocks, each opened by a -- name:
// line, its other header lines, then its SQL up to the next -- name:.
func parsePolicies(file, src string) ([]*Policy, []diag.Diagnostic) {
	var ps []*Policy
	var ds []diag.Diagnostic
	errAt := func(pos token.Position, fix, format string, args ...any) {
		ds = append(ds, diag.New(pos, fmt.Sprintf(format, args...), fix))
	}
	type block struct {
		p          *Policy
		start, end int // the source of its SQL
		failed     int // diagnostics before it: a block with more is broken, as are those before it
	}
	var blocks []*block
	keys := ref.PolicyKeys.Names()
	h := &header{q: &Query{File: file}}
	off := 0
	for line := 1; off < len(src); line++ {
		end, next := len(src), len(src)
		if i := strings.IndexByte(src[off:], '\n'); i >= 0 {
			end, next = off+i, off+i+1
		}
		text := strings.TrimSuffix(src[off:end], "\r")
		trimmed := strings.TrimLeft(text, " \t")
		if !strings.HasPrefix(trimmed, "--") {
			if strings.TrimSpace(text) != "" && len(blocks) == 0 {
				errAt(token.Position{Filename: file, Offset: off, Line: line, Column: 1}, "start each policy with -- name: name Model alias", "SQL before the first -- name: line")
				return nil, ds
			}
			off = next
			continue
		}
		h.line, h.off, h.text = line, off, text
		i := len(text) - len(trimmed) + 2
		start, kend, colon := headerKey(text, i)
		if colon < 0 {
			off = next
			continue
		}
		key := text[start:kend]
		var b *block
		if len(blocks) > 0 {
			b = blocks[len(blocks)-1]
		}
		switch {
		case !slices.Contains(keys, key):
			fix := "allowed: " + strings.Join(keys, ", ") + "; start a comment with something other than \"word:\""
			if s := diag.Suggest(key, keys); s != "" {
				fix = fmt.Sprintf("did you mean %q? %s", s, fix)
			}
			if key == "policy" {
				fix = "start a policy with -- name: name Model alias; -- policy: goes in a query that uses one"
			}
			h.errf(start, fix, "unknown header key %q in policies.sql", key)
		case colon > kend:
			h.errf(kend, fmt.Sprintf("write -- %s: with no space before the colon", key), "space between %s and its colon", key)
		case key == "name":
			if b != nil {
				b.end = off
			}
			b = &block{p: &Policy{File: file}, start: next, end: len(src), failed: len(ds)}
			blocks = append(blocks, b)
			policyName(h, b.p, h.fields(colon+1))
		case b == nil:
			h.errf(start, "start each policy with -- name: name Model alias", "-- %s: before the first -- name: line", key)
		case key == "param":
			n := len(h.q.Params)
			h.param(h.fields(colon + 1))
			if len(h.q.Params) > n {
				p := h.q.Params[n]
				h.q.Params = nil
				switch {
				case p.Optional || p.Type.Slice || p.Type.Choices != nil:
					h.errf(p.Pos.Column-1, "declare a value of one type, like now time.Time; a policy has no {conditions}", "a policy's parameter %s is optional, a slice or choices", p.Name)
				case strings.HasPrefix(p.Name, viewerPrefix):
					h.errf(p.Pos.Column-1, "remove the line: :"+p.Name+" is a column of the user", "parameter %s is the user's", p.Name)
				case p.Name == "id" || p.Name == "u" || p.Name == "db" || p.Name == "ctx":
					h.errf(p.Pos.Column-1, "rename it: GetX takes "+p.Name+" already", "parameter %s is a name GetX takes", p.Name)
				case slices.ContainsFunc(b.p.Params, func(o Param) bool { return o.Name == p.Name }):
					h.errf(p.Pos.Column-1, "remove one of them", "parameter %s is declared twice", p.Name)
				default:
					b.p.Params = append(b.p.Params, p)
				}
			}
		case key == "get":
			fs := h.fields(colon + 1)
			switch {
			case b.p.Get != "":
				h.errf(start, "keep one -- get: line", "second -- get: line in policy %s", b.p.Name)
			case len(fs) != 1 || b.p.Model != "" && fs[0].s != "Get"+b.p.Model:
				h.errf(colon+1, "write -- get: Get"+b.p.Model, "-- get: names the GetX of the policy's model, Get%s", b.p.Model)
			default:
				b.p.Get, b.p.GetPos = fs[0].s, h.pos(fs[0].i)
			}
		}
		ds, h.ds = append(ds, h.ds...), nil
		off = next
	}
	if len(blocks) == 0 {
		errAt(token.Position{Filename: file, Line: 1, Column: 1}, "start each policy with -- name: name Model alias, then its condition", "policies.sql has no policy")
		return nil, ds
	}
	// The whole file is one statement of tokens: comments give none.
	stmts, _, sds := dialect.SplitComments(file, src)
	if sds != nil { // the queries using them are left out, as broken
		for _, b := range blocks {
			if b.p.Name != "" {
				b.p.broken = true
				ps = append(ps, b.p)
			}
		}
		return ps, append(ds, sds...)
	}
	var toks []dialect.Token
	for i, st := range stmts {
		if i > 0 {
			last := stmts[i-1].Tokens[len(stmts[i-1].Tokens)-1]
			at := last.Pos
			at.Offset, at.Column = at.Offset+len(last.Text), at.Column+len(last.Text)
			errAt(at, "remove the ; a policy is one condition", "; in policies.sql")
		}
		toks = append(toks, st.Tokens...)
	}
	for _, b := range blocks {
		var bt []dialect.Token
		for _, t := range toks {
			if t.Pos.Offset >= b.start && t.Pos.Offset < b.end {
				bt = append(bt, t)
			}
		}
		ds = append(ds, b.p.read(src, bt)...)
		b.p.broken = len(ds) > b.failed || b.p.Name == ""
		if b.p.Name == "" {
			continue
		}
		if prev := slices.IndexFunc(ps, func(o *Policy) bool { return o.Name == b.p.Name }); prev >= 0 {
			errAt(b.p.Pos, "rename one of them", "policy %s is also at line %d", b.p.Name, ps[prev].Pos.Line)
			b.p.broken = true
		}
		ps = append(ps, b.p)
	}
	return ps, ds
}

// policyName reads -- name: name Model alias.
func policyName(h *header, p *Policy, fs []field) {
	const form = "write -- name: name Model alias, like -- name: visible Post p"
	switch {
	case len(fs) != 3:
		h.errf(len(h.text), form, "-- name: line of a policy has %d words, not 3", len(fs))
	case !sqlName.MatchString(fs[0].s):
		h.errf(fs[0].i, "write it in lowercase snake_case, like visible", "policy name %q is not lowercase snake_case", fs[0].s)
	case fs[0].s == "none":
		h.errf(fs[0].i, "name it otherwise; -- policy: none alias says a query uses no policy", "policy name none is taken")
	case !goName.MatchString(fs[1].s):
		h.errf(fs[1].i, form, "%q is not a model's name", fs[1].s)
	case !sqlName.MatchString(fs[2].s):
		h.errf(fs[2].i, "write it in lowercase snake_case, like p", "alias %q is not lowercase snake_case", fs[2].s)
	default:
		p.Name, p.Pos = fs[0].s, h.pos(fs[0].i)
		p.Model, p.ModelPos = fs[1].s, h.pos(fs[1].i)
		p.Alias = fs[2].s
	}
}

// read reads the tokens of p's SQL into its parts.
func (p *Policy) read(src string, toks []dialect.Token) []diag.Diagnostic {
	var ds []diag.Diagnostic
	errAt := func(pos token.Position, fix, format string, args ...any) {
		ds = append(ds, diag.New(pos, fmt.Sprintf(format, args...), fix))
	}
	if len(toks) == 0 {
		if p.Name != "" {
			errAt(p.Pos, "write its condition after the header, like NOT p.hidden", "policy %s has no SQL", p.Name)
		}
		return ds
	}
	used := map[string]bool{}
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		var next dialect.Token
		glued := false
		if i+1 < len(toks) {
			next = toks[i+1]
			glued = next.Pos.Offset == t.Pos.Offset+len(t.Text)
		}
		part := policyPart{sql: t.Text, pos: t.Pos}
		switch {
		case is(t, "{") || is(t, "}"):
			errAt(t.Pos, "remove it; a policy holds always, so it has no {condition}", "%s in a policy", t.Text)
			continue
		case is(t, "?"):
			errAt(t.Pos, "write values as :name", "placeholder ? in a policy")
			continue
		case (is(t, "$") || is(t, "@")) && glued && next.Kind == dialect.Word:
			errAt(t.Pos, "write values as :name", "placeholder %s%s in a policy", t.Text, next.Text)
			continue
		case is(t, ":") && glued && next.Kind == dialect.Word && !(i > 0 && is(toks[i-1], ":") && toks[i-1].Pos.Offset+1 == t.Pos.Offset):
			name := next.Text
			i++
			switch {
			case strings.HasPrefix(name, viewerPrefix) && sqlName.MatchString(name):
				if !slices.ContainsFunc(p.Params, func(o Param) bool { return o.Name == name }) {
					p.Params = append(p.Params, Param{Name: name, Pos: t.Pos, Viewer: true})
				}
			case !slices.ContainsFunc(p.Params, func(o Param) bool { return o.Name == name }):
				errAt(t.Pos, "declare it with -- param: "+name+" TYPE in the policy's header; :viewer_<column> is a column of the user", "parameter %s is not declared", name)
				continue
			}
			used[name] = true
			part = policyPart{param: name, pos: t.Pos}
			t = next
		case (t.Kind == dialect.Word || t.Kind == dialect.Quoted) && fold(t) == p.Alias && glued && is(next, "."):
			part.alias = true
		}
		part.space = i+1 < len(toks) && toks[i+1].Pos.Offset > t.Pos.Offset+len(t.Text)
		p.parts = append(p.parts, part)
	}
	for _, prm := range p.Params {
		if !prm.Viewer && !used[prm.Name] {
			errAt(prm.Pos, "use it as :"+prm.Name+", or remove the -- param: line", "parameter %s is not used", prm.Name)
		}
	}
	return ds
}

// expand is p as a query's SQL reads it: its parts in parentheses, the
// alias replaced with alias, at the place of the use.
func (p *Policy) expand(alias string, at token.Position) []Part {
	parts := []Part{{SQL: "(", Pos: at}}
	for _, pp := range p.parts {
		switch {
		case pp.param != "":
			parts = append(parts, Part{Param: pp.param, Pos: pp.pos})
		case pp.alias:
			parts = append(parts, Part{SQL: alias, Pos: pp.pos})
		default:
			parts = append(parts, Part{SQL: pp.sql, Pos: pp.pos})
		}
		if pp.space {
			parts = append(parts, Part{SQL: " ", Pos: pp.pos})
		}
	}
	return append(parts, Part{SQL: ")", Pos: at})
}

// sql is p in parentheses with its parameters as placeholders, numbered
// after args, which it returns with p's parameters added in order.
func (p *Policy) sql(d project.Dialect, args []string) (string, []string) {
	mark := "$"
	if d == project.SQLite {
		mark = "?"
	}
	var b strings.Builder
	b.WriteString("(")
	for _, pp := range p.parts {
		switch {
		case pp.param != "":
			n := slices.Index(args, pp.param)
			if n < 0 {
				n, args = len(args), append(args, pp.param)
			}
			b.WriteString(mark + strconv.Itoa(n+1))
		case pp.alias:
			b.WriteString(p.Alias)
		default:
			b.WriteString(pp.sql)
		}
		if pp.space {
			b.WriteString(" ")
		}
	}
	return b.String() + ")", args
}

// Probe is a query that reads the table of p's model under p alone, for
// the database to check p's SQL: what it finds wrong is at its place in
// policies.sql. CheckPolicies must have found the model.
func (p *Policy) Probe() (*Query, Variant) {
	q := &Query{File: p.File, Name: p.Name, NamePos: p.Pos, Kind: Many, Params: p.Params}
	v := Variant{Pieces: []Piece{{SQL: "SELECT 1 FROM " + dialect.Quote(p.model.Table) + " " + p.Alias + " WHERE ", Pos: p.Pos}}}
	for _, part := range p.expand(p.Alias, p.Pos) {
		if part.Param == "" {
			v.Pieces = append(v.Pieces, Piece{SQL: part.SQL, Pos: part.Pos})
			continue
		}
		n := slices.Index(v.Args, part.Param)
		if n < 0 {
			n, v.Args = len(v.Args), append(v.Args, part.Param)
		}
		v.Pieces = append(v.Pieces, Piece{Arg: n + 1, Pos: part.Pos})
	}
	return q, v
}

// Guarded are the models of app whose GetX applies one of pols.
func Guarded(app *model.App, pols []*Policy) map[*model.Model]bool {
	g := map[*model.Model]bool{}
	for _, m := range app.Models {
		g[m] = slices.ContainsFunc(pols, func(p *Policy) bool { return p.Get != "" && p.Model == m.Name })
	}
	return g
}
