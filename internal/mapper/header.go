// Package mapper reads SQL mapper files, queries/*.sql of an app, and
// turns them into Go (D-20~D-24).
package mapper

import (
	"fmt"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/ref"
)

// App is a directory holding queries/.
type App struct {
	Dir     string // slash path relative to the project root; "." for the root
	Queries []*Query
}

// Query is one mapper file whose header was read without error.
type Query struct {
	File string // slash path relative to the project root
	// Src is the whole file. The header is -- comments, which give no
	// tokens, so the body is read by tokenizing all of Src.
	Src       string
	HeaderEnd int // offset of the first byte after the header
	Name      string
	NamePos   token.Position
	Kind      Kind
	Result    string     // "" for :exec
	Scalar    model.Type // the result's type when Result names one, like bool; 0 for a struct
	Params    []Param
	Columns   []Column
	Uses      []Use           // the -- policy: lines (D-155)
	Lines     []Line          // the body
	toks      []dialect.Token // of the body's statement, for the joins (D-104)
	policies  []*Policy       // of each of Uses but none, once Read finds them
}

// Use is a -- policy: line: the query reads Alias under policy Name, or,
// when Name is none, reads every row of it, for the reason Why.
type Use struct {
	Name, Alias string
	Why         string
	Pos         token.Position // of the name
	expanded    bool           // the body wrote Name(Alias)
}

// Kind is what a query returns.
type Kind int

const (
	One  Kind = iota + 1 // one row; sql.ErrNoRows if none, an error if more
	Many                 // any number of rows
	Exec                 // no rows; the number of rows affected
)

func (k Kind) String() string { return [...]string{"", ":one", ":many", ":exec"}[k] }

// Param is a -- param: line.
type Param struct {
	Name     string
	Pos      token.Position
	Optional bool // name? : a pointer, nil when absent
	Type     Type
	// Viewer is set for :viewer_<column> of a policy: the value is the
	// user's field, NULL for nobody. It is in no Params struct.
	Viewer bool
	Field  *model.Field // the user's field of a Viewer parameter, once checked
}

// Column is a -- column: line (D-104).
type Column struct {
	Name     string
	Pos      token.Position
	Optional bool
	Type     model.Type
}

// Type is the type of a parameter: a D-77 type, a slice of one, or
// choices. For choices Base is 0.
type Type struct {
	Base    model.Type
	Slice   bool
	Choices []string
}

var (
	kinds     = ref.QueryKinds.Names()
	goName    = regexp.MustCompile(`^[A-Z][A-Za-z0-9_]*$`)
	sqlName   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	baseTypes = map[string]model.Type{}
	typeNames []string
)

func init() {
	for t := model.Int64; t <= model.Bytes; t++ {
		baseTypes[t.String()] = t
		typeNames = append(typeNames, t.String())
	}
}

// Read reads the headers of the mapper files, as project.QueryFiles lists
// them, and groups the queries by app. A file with an error in its header
// gives no query, so later checks do not report what follows from it; nor
// does one using a policy with an error. pols are the apps' policies, which
// a query's body expands where it uses them (D-155).
func Read(root string, files []string, pols Policies) ([]*App, []diag.Diagnostic) {
	var apps []*App
	var ds []diag.Diagnostic
	for _, f := range slices.Sorted(slices.Values(files)) {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			ds = append(ds, diag.New(token.Position{Filename: f}, "cannot read file: "+diag.OneLine(err.Error()), "make sure the file can be read"))
			continue
		}
		q, qds := parse(f, string(src))
		ds = append(ds, qds...)
		if q == nil {
			continue
		}
		if pds, ok := q.findPolicies(pols[path.Dir(path.Dir(f))]); !ok {
			ds = append(ds, pds...)
			continue
		}
		if bds := q.body(); bds != nil {
			ds = append(ds, bds...)
			continue
		}
		if dir := path.Dir(path.Dir(f)); len(apps) == 0 || apps[len(apps)-1].Dir != dir {
			apps = append(apps, &App{Dir: dir})
		}
		app := apps[len(apps)-1]
		if j := slices.IndexFunc(app.Queries, func(p *Query) bool { return p.Name == q.Name }); j >= 0 {
			ds = append(ds, diag.New(q.NamePos, fmt.Sprintf("query name %s is also the name of %s", q.Name, app.Queries[j].File),
				"rename one of them; each query becomes a Go function of the package"))
			continue
		}
		app.Queries = append(app.Queries, q)
	}
	return apps, ds
}

// findPolicies finds the policies q uses among the app's, and adds their
// parameters to q's: a parameter both declare is one, of one type. It
// reports false if q is to be left out.
func (q *Query) findPolicies(pols []*Policy) ([]diag.Diagnostic, bool) {
	var ds []diag.Diagnostic
	broken := false
	for _, u := range q.Uses {
		if u.Name == "none" {
			continue
		}
		i := slices.IndexFunc(pols, func(p *Policy) bool { return p.Name == u.Name })
		if i < 0 {
			fix := "write the policy in policies.sql of the app, beside queries/, or use -- policy: none " + u.Alias + " why"
			var names []string
			for _, p := range pols {
				names = append(names, p.Name)
			}
			if s := diag.Suggest(u.Name, names); s != "" {
				fix = fmt.Sprintf("did you mean %q? or %s", s, fix)
			}
			ds = append(ds, diag.New(u.Pos, "no policy "+u.Name+" in policies.sql of the app", fix))
			continue
		}
		p := pols[i]
		broken = broken || p.broken
		q.policies = append(q.policies, p)
		for _, pp := range p.Params {
			j := slices.IndexFunc(q.Params, func(o Param) bool { return o.Name == pp.Name })
			switch {
			case j < 0:
				q.Params = append(q.Params, pp)
			case pp.Viewer && !q.Params[j].Viewer:
				ds = append(ds, diag.New(q.Params[j].Pos, fmt.Sprintf("parameter %s is the user's column in policy %s", pp.Name, p.Name), "rename it"))
			case pp.Viewer:
			case q.Params[j].Optional || q.Params[j].Type.Slice || q.Params[j].Type.Choices != nil || q.Params[j].Type.Base != pp.Type.Base:
				ds = append(ds, diag.New(q.Params[j].Pos, fmt.Sprintf("parameter %s is not the %s of policy %s", pp.Name, typeString(pp.Type), p.Name),
					fmt.Sprintf("declare it as -- param: %s %s, or rename it", pp.Name, typeString(pp.Type))))
			}
		}
	}
	return ds, ds == nil && !broken
}

// parse reads the header: the lines from the start of src that are empty
// or start with --. It returns nil if the header has an error.
func parse(file, src string) (*Query, []diag.Diagnostic) {
	h := &header{q: &Query{File: file, Src: src}}
	off := 0
	for line := 1; off < len(src); line++ {
		end, next := len(src), len(src)
		if i := strings.IndexByte(src[off:], '\n'); i >= 0 {
			end, next = off+i, off+i+1
		}
		text := strings.TrimSuffix(src[off:end], "\r")
		trimmed := strings.TrimLeft(text, " \t")
		if trimmed != "" && !strings.HasPrefix(trimmed, "--") {
			break
		}
		if trimmed != "" {
			h.line, h.off, h.text = line, off, text
			h.read(len(text) - len(trimmed) + 2)
		}
		off = next
	}
	h.q.HeaderEnd = off
	if !h.seen {
		h.ds = append(h.ds, diag.New(token.Position{Filename: file, Line: 1, Column: 1}, "query has no -- name: line",
			"start the file with -- name: Name :one|:many|:exec [Result]; -- lines after /* */ or SQL are not the header"))
	}
	if h.ds != nil {
		return nil, h.ds
	}
	return h.q, nil
}

type header struct {
	q    *Query
	ds   []diag.Diagnostic
	seen bool // a name line was read, with or without error

	// the line being read
	line, off int
	text      string
}

func (h *header) pos(i int) token.Position {
	return token.Position{Filename: h.q.File, Offset: h.off + i, Line: h.line, Column: i + 1}
}

func (h *header) errf(i int, fix, format string, args ...any) {
	h.ds = append(h.ds, diag.New(h.pos(i), fmt.Sprintf(format, args...), fix))
}

// field is a word of a header line and where it starts.
type field struct {
	s string
	i int
}

// fields splits h.text from i at spaces and tabs.
func (h *header) fields(i int) []field {
	var fs []field
	for i < len(h.text) {
		if c := h.text[i]; c == ' ' || c == '\t' {
			i++
			continue
		}
		j := i
		for j < len(h.text) && h.text[j] != ' ' && h.text[j] != '\t' {
			j++
		}
		fs = append(fs, field{h.text[i:j], i})
		i = j
	}
	return fs
}

// headerKey finds word: in t from i, where a comment's text starts. The
// word is t[start:end] and the colon is at colon; colon is -1 if the
// comment does not start with word:.
func headerKey(t string, i int) (start, end, colon int) {
	i += len(t[i:]) - len(strings.TrimLeft(t[i:], " \t"))
	start = i
	for i < len(t) && (isNameStart(t[i]) || i > start && '0' <= t[i] && t[i] <= '9') {
		i++
	}
	colon = len(t) - len(strings.TrimLeft(t[i:], " \t"))
	if i == start || colon == len(t) || t[colon] != ':' {
		return start, i, -1
	}
	return start, i, colon
}

// read reads the line from i, just past its --. A line whose comment
// starts with word: is a header line; any other is a description.
func (h *header) read(i int) {
	t := h.text
	start, i, colon := headerKey(t, i)
	if colon < 0 {
		return
	}
	key := t[start:i]
	if keys := ref.QueryKeys.Names(); !slices.Contains(keys, key) {
		fix := "allowed: " + strings.Join(keys, ", ") + "; start a comment with something other than \"word:\""
		if s := diag.Suggest(key, keys); s != "" {
			fix = fmt.Sprintf("did you mean %q? %s", s, fix)
		}
		h.errf(start, fix, "unknown header key %q", key)
		return
	}
	if key == "name" {
		if h.seen {
			h.errf(start, "keep one -- name: line; a file holds one query", "second -- name: line")
			return
		}
		h.seen = true
	}
	if colon > i {
		h.errf(i, fmt.Sprintf("write -- %s: with no space before the colon", key), "space between %s and its colon", key)
		return
	}
	fs := h.fields(colon + 1)
	switch key {
	case "name":
		h.name(fs)
	case "param":
		h.param(fs)
	case "column":
		h.column(fs)
	case "policy":
		h.policy(fs)
	default:
		panic("maago bug: header key " + key + " is in ref.QueryKeys but not handled")
	}
}

func (h *header) name(fs []field) {
	const form = "write -- name: Name :one|:many|:exec [Result]"
	if len(fs) == 0 {
		h.errf(len(h.text), form, "-- name: line has no query name")
		return
	}
	if !goName.MatchString(fs[0].s) {
		h.errf(fs[0].i, "start it with an uppercase ASCII letter, followed by letters, digits or _", "query name %q is not an exported Go name", fs[0].s)
		return
	}
	if len(fs) == 1 {
		h.errf(len(h.text), "add :one, :many or :exec after the name", "query %s has no kind", fs[0].s)
		return
	}
	k := fs[1]
	kind := Kind(slices.Index(kinds, k.s) + 1)
	if kind == 0 {
		if slices.Contains(kinds, ":"+k.s) {
			h.errf(k.i, fmt.Sprintf("write :%s", k.s), "query kind %q has no colon", k.s)
			return
		}
		fix := "allowed: " + strings.Join(kinds, ", ")
		if s := diag.Suggest(k.s, kinds); s != "" {
			fix = fmt.Sprintf("did you mean %q? %s", s, fix)
		}
		h.errf(k.i, fix, "unknown query kind %q", k.s)
		return
	}
	var result string
	switch {
	case kind == Exec && len(fs) > 2:
		h.errf(fs[2].i, fmt.Sprintf("remove %s, or use :one or :many", fs[2].s), ":exec returns no rows, so it has no result type")
		return
	case kind != Exec && len(fs) == 2:
		h.errf(len(h.text), fmt.Sprintf("name a model or a new type after %s, like %s PostSummary, or a type of one column, like %s bool", kind, kind, kind), "query %s %s has no result type", fs[0].s, kind)
		return
	case kind != Exec:
		result = fs[2].s
		if t, ok := baseTypes[result]; ok && t != model.Bytes {
			h.q.Scalar = t // one column of that type (M10 task 3)
		} else if r := result[0]; r == '[' || r == '*' || 'a' <= r && r <= 'z' {
			scalars := slices.DeleteFunc(slices.Clone(typeNames), func(n string) bool { return n == "[]byte" })
			fix := "use one of " + strings.Join(scalars, ", ") + " for one column, or name a new type, like " + kind.String() + " PostSummary"
			if s := diag.Suggest(result, scalars); s != "" {
				fix = fmt.Sprintf("did you mean %q? or %s", s, fix)
			}
			h.errf(fs[2].i, fix, "unknown result type %q", result)
			return
		} else if !goName.MatchString(result) {
			h.errf(fs[2].i, "start it with an uppercase ASCII letter, followed by letters, digits or _; or, for one column, use bool, int64, int32, float64, string or time.Time", "result type %q is not an exported Go name", result)
			return
		}
		if len(fs) > 3 {
			h.errf(fs[3].i, "remove it; "+form[len("write "):], "unexpected %q after the result type", fs[3].s)
			return
		}
	}
	h.q.Name, h.q.NamePos, h.q.Kind, h.q.Result = fs[0].s, h.pos(fs[0].i), kind, result
}

// nameAndType reads name[?] type from fs, the part common to param and
// column lines. ok is false after an error.
func (h *header) nameAndType(key string, fs []field) (name string, optional bool, typ field, ok bool) {
	form := fmt.Sprintf("write -- %s: name[?] type", key)
	if len(fs) == 0 {
		h.errf(len(h.text), form, "-- %s: line has no name", key)
		return
	}
	name, optional = strings.CutSuffix(fs[0].s, "?")
	if name == "" {
		h.errf(fs[0].i, form, "-- %s: line has no name", key)
		return
	}
	if !sqlName.MatchString(name) {
		h.errf(fs[0].i, "write it in lowercase snake_case, like author_id", "%s name %q is not lowercase snake_case", key, name)
		return
	}
	if len(fs) > 1 && fs[1].s == "?" {
		h.errf(fs[1].i, fmt.Sprintf("write %s? with no space", name), "space between %s and ?", name)
		return
	}
	if len(fs) == 1 {
		h.errf(len(h.text), form, "%s %s has no type", key, name)
		return
	}
	if len(fs) > 2 {
		h.errf(fs[2].i, "remove it; "+form[len("write "):], "unexpected %q after the type of %s", fs[2].s, name)
		return
	}
	return name, optional, fs[1], true
}

func (h *header) param(fs []field) {
	name, optional, tf, ok := h.nameAndType("param", fs)
	if !ok {
		return
	}
	for _, p := range h.q.Params {
		if p.Name == name {
			h.errf(fs[0].i, "remove one of them", "parameter %s is declared twice", name)
			return
		}
	}
	typ, ok := h.paramType(name, tf)
	if !ok {
		return
	}
	if optional {
		var why string
		switch {
		case typ.Base == model.Bool && !typ.Slice:
			why = fmt.Sprintf("a bool cannot be optional: {%s} would mean both present and true; use choices for three values", name)
		case typ.Slice:
			why = "a slice cannot be optional: a nil or empty slice is already absent"
		case typ.Choices != nil:
			why = "choices cannot be optional: add a choice for none, like none/latest/amount"
		}
		if why != "" {
			h.errf(fs[0].i, fmt.Sprintf("remove the ? from %s?", name), "%s", why)
			return
		}
	}
	h.q.Params = append(h.q.Params, Param{Name: name, Pos: h.pos(fs[0].i), Optional: optional, Type: typ})
}

func (h *header) paramType(name string, f field) (Type, bool) {
	s := f.s
	switch {
	case strings.HasPrefix(s, "*"):
		h.errf(f.i, fmt.Sprintf("write %s? %s instead", name, s[1:]), "pointer type %s in a header", s)
		return Type{}, false
	case strings.Contains(s, "/"):
		var t Type
		j := f.i
		for _, c := range strings.Split(s, "/") {
			switch {
			case !sqlName.MatchString(c):
				h.errf(j, "write each choice in lowercase snake_case, separated by /, like latest/amount", "choice %q is not lowercase snake_case", c)
				return Type{}, false
			case slices.Contains(t.Choices, c):
				h.errf(j, "remove the repeated choice", "choice %s appears twice", c)
				return Type{}, false
			}
			t.Choices = append(t.Choices, c)
			j += len(c) + 1
		}
		return t, true
	}
	base, slice := s, false
	if s != "[]byte" {
		base, slice = strings.CutPrefix(s, "[]")
	}
	b, ok := baseTypes[base]
	if !ok {
		h.unknownType(f, base, slice, "; or a slice of one, like []int64, or choices, like latest/amount")
		return Type{}, false
	}
	return Type{Base: b, Slice: slice}, true
}

func (h *header) column(fs []field) {
	name, optional, tf, ok := h.nameAndType("column", fs)
	if !ok {
		return
	}
	for _, c := range h.q.Columns {
		if c.Name == name {
			h.errf(fs[0].i, "remove one of them", "column %s is declared twice", name)
			return
		}
	}
	b, ok := baseTypes[tf.s]
	if !ok {
		h.unknownType(tf, tf.s, false, "; a column cannot be a slice or choices")
		return
	}
	h.q.Columns = append(h.q.Columns, Column{name, h.pos(fs[0].i), optional, b})
}

// policy reads -- policy: name alias, or -- policy: none alias why.
func (h *header) policy(fs []field) {
	const form = "write -- policy: name alias, or -- policy: none alias why"
	switch {
	case len(fs) < 2:
		h.errf(len(h.text), form, "-- policy: line has %d words", len(fs))
		return
	case !sqlName.MatchString(fs[0].s):
		h.errf(fs[0].i, form, "policy name %q is not lowercase snake_case", fs[0].s)
		return
	case !sqlName.MatchString(fs[1].s):
		fix := form
		if len(fs) == 3 && goName.MatchString(fs[1].s) {
			fix = fmt.Sprintf("write -- policy: %s %s; the model is in policies.sql", fs[0].s, fs[2].s)
		}
		h.errf(fs[1].i, fix, "alias %q is not lowercase snake_case", fs[1].s)
		return
	case fs[0].s == "none" && len(fs) == 2:
		h.errf(len(h.text), "say why the query reads every row of "+fs[1].s+": -- policy: none "+fs[1].s+" why", "-- policy: none with no reason")
		return
	case fs[0].s != "none" && len(fs) > 2:
		h.errf(fs[2].i, "remove it; "+form[len("write "):], "unexpected %q after the alias of -- policy:", fs[2].s)
		return
	}
	for _, u := range h.q.Uses {
		if u.Alias == fs[1].s {
			h.errf(fs[1].i, "keep one -- policy: line for each alias", "alias %s has a -- policy: line already", u.Alias)
			return
		}
	}
	u := Use{Name: fs[0].s, Alias: fs[1].s, Pos: h.pos(fs[0].i)}
	if u.Name == "none" {
		u.Why = strings.TrimSpace(h.text[fs[2].i:])
	}
	h.q.Uses = append(h.q.Uses, u)
}

func (h *header) unknownType(f field, base string, slice bool, more string) {
	fix := "allowed: " + strings.Join(typeNames, ", ") + more
	if s := diag.Suggest(base, typeNames); s != "" {
		if slice {
			s = "[]" + s
		}
		fix = fmt.Sprintf("did you mean %q? %s", s, fix)
	}
	h.errf(f.i, fix, "unknown type %q", f.s)
}
