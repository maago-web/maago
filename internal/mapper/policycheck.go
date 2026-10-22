package mapper

import (
	"cmp"
	"fmt"
	"go/token"
	"maps"
	"slices"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/model"
)

// Viewer is the user type of the project, as the views take it (D-149).
// A policy's :viewer_<column> is a field of it.
type Viewer struct {
	Expr  string         // the type as the views write it, like auth.User; "" if no view takes the user
	Pos   token.Position // of the view's parameter
	Model *model.Model   // the model it is, nil if none
}

// guarded is a model with policies and the app it is in.
type guarded struct {
	m   *model.Model
	dir string
	ps  []*Policy
}

// CheckPolicies checks the policies and every query that reads the table
// of a model with policies (rule 6, D-155): each place it reads it uses a
// policy of the model there, or says none. It fills in the types of the
// :viewer_ parameters. A project without policies is not checked, so the
// shapes tables cannot read are errors only once a policy is there.
func CheckPolicies(apps []*model.App, pols Policies, queries []*App, v Viewer) []diag.Diagnostic {
	var ds []diag.Diagnostic
	errAt := func(pos token.Position, fix, format string, args ...any) {
		ds = append(ds, diag.New(pos, fmt.Sprintf(format, args...), fix))
	}
	byTable := map[string]*guarded{}
	var first *Policy
	for _, dir := range slices.Sorted(maps.Keys(pols)) {
		var app *model.App
		for _, a := range apps {
			if a.Dir == dir {
				app = a
			}
		}
		for _, p := range pols[dir] {
			if p.Model == "" {
				continue
			}
			if first == nil {
				first = p
			}
			var names []string
			if app != nil {
				for _, m := range app.Models {
					names = append(names, m.Name)
					if m.Name == p.Model {
						p.model = m
					}
				}
			}
			if p.model == nil {
				fix := "name a model of the app at " + dir
				if s := diag.Suggest(p.Model, names); s != "" {
					fix = fmt.Sprintf("did you mean %q? %s", s, fix)
				}
				errAt(p.ModelPos, fix, "no model %s in the app of this policies.sql", p.Model)
				continue
			}
			g := byTable[p.model.Table]
			if g == nil {
				g = &guarded{m: p.model, dir: dir}
				byTable[p.model.Table] = g
			}
			g.ps = append(g.ps, p)
		}
	}
	if first == nil {
		return ds
	}
	for _, g := range byTable {
		var gets []*Policy
		for _, p := range g.ps {
			if p.Get != "" {
				gets = append(gets, p)
			}
		}
		switch {
		case len(gets) == 0:
			errAt(g.ps[0].Pos, "add -- get: Get"+g.m.Name+" to the policy Get"+g.m.Name+" applies: one of "+policyNames(g.ps),
				"model %s has policies, but none says -- get: Get%s", g.m.Name, g.m.Name)
		case len(gets) > 1:
			errAt(gets[1].GetPos, "keep -- get: in one policy of the model", "policies %s and %s both say -- get: Get%s", gets[0].Name, gets[1].Name, g.m.Name)
		}
	}
	// The users' columns.
	if v.Expr == "" {
		errAt(first.Pos, "take the user in a view, like func Home(ctx context.Context, u *auth.User, db maago.DB, in struct{}): a query with a policy takes it too",
			"policies need the user's type, but no view takes the user")
	}
	for _, dir := range slices.Sorted(maps.Keys(pols)) {
		for _, p := range pols[dir] {
			for i := range p.Params {
				prm := &p.Params[i]
				if !prm.Viewer || v.Expr == "" {
					continue
				}
				if v.Model == nil {
					errAt(prm.Pos, "make the user type a model, or compare with a -- param: of the policy", "the user type %s is not a model, so :%s has no column", v.Expr, prm.Name)
					continue
				}
				col := strings.TrimPrefix(prm.Name, viewerPrefix)
				var cols []string
				for _, f := range v.Model.Fields {
					cols = append(cols, viewerPrefix+f.Column)
					if f.Column == col {
						prm.Field, prm.Type = f, Type{Base: f.Type}
					}
				}
				if prm.Field == nil {
					fix := "use :viewer_<column> of " + v.Expr + ": " + strings.Join(cols, ", ")
					if s := diag.Suggest(prm.Name, cols); s != "" {
						fix = fmt.Sprintf("did you mean %q? %s", s, fix)
					}
					errAt(prm.Pos, fix, "%s has no column %s", v.Expr, col)
				}
			}
		}
	}
	for _, qa := range queries {
		for _, q := range qa.Queries {
			for i := range q.Params {
				if q.Params[i].Viewer {
					for _, p := range q.policies {
						if j := slices.IndexFunc(p.Params, func(o Param) bool { return o.Name == q.Params[i].Name }); j >= 0 {
							q.Params[i].Field, q.Params[i].Type = p.Params[j].Field, p.Params[j].Type
						}
					}
				}
			}
			for _, p := range q.policies {
				p.used = true
			}
			ds = append(ds, q.checkPolicies(qa.Dir, byTable)...)
		}
	}
	for _, dir := range slices.Sorted(maps.Keys(pols)) {
		for _, p := range pols[dir] {
			if !p.used && p.Get == "" && p.model != nil {
				errAt(p.Pos, "use it in a query with -- policy: "+p.Name+" alias, or remove it", "policy %s is used by no query", p.Name)
			}
		}
	}
	return ds
}

func policyNames(ps []*Policy) string {
	var names []string
	for _, p := range ps {
		names = append(names, p.Name)
	}
	return strings.Join(names, ", ")
}

// policyUse is a policy's use in a query's tokens: toks[i] is its name.
type policyUse struct {
	i      int
	u      Use
	policy *Policy
}

// checkPolicies checks where q reads the tables of byTable. q is in the
// app at dir.
func (q *Query) checkPolicies(dir string, byTable map[string]*guarded) []diag.Diagnostic {
	refs, blocks, ds := q.tables()
	if ds != nil {
		return ds
	}
	errAt := func(pos token.Position, fix, format string, args ...any) {
		ds = append(ds, diag.New(pos, fmt.Sprintf(format, args...), fix))
	}
	toks := q.toks
	key := func(r tableRef) string { return cmp.Or(r.Alias, r.Table) }
	var uses []policyUse
	for i := 0; i+3 < len(toks); i++ {
		t := toks[i]
		if t.Kind != dialect.Word || !is(toks[i+1], "(") || !is(toks[i+3], ")") {
			continue
		}
		for _, u := range q.Uses {
			if u.Name != "none" && strings.EqualFold(u.Name, t.Text) && u.Alias == fold(toks[i+2]) {
				p := q.policies[slices.IndexFunc(q.policies, func(p *Policy) bool { return p.Name == u.Name })]
				uses = append(uses, policyUse{i, u, p})
			}
		}
	}
	conditional := map[int]bool{} // the lines with a {condition}
	for _, l := range q.Lines {
		if l.Cond != nil {
			conditional[l.Cond.Pos.Line] = true
		}
	}
	none := map[string]Use{}
	for _, u := range q.Uses {
		if u.Name == "none" {
			none[u.Alias] = u
		}
	}
	for _, r := range refs {
		g := byTable[r.Table]
		if g == nil {
			continue
		}
		k := key(r)
		switch {
		case g.dir != dir:
			errAt(r.Pos, "move the query into the app at "+g.dir+", whose policies.sql has the policies of "+g.m.Name,
				"query of the app at %s reads %s, the table of model %s, whose policies are another app's", dir, r.Table, g.m.Name)
			continue
		case none[k] != Use{}:
			continue
		}
		ok := slices.ContainsFunc(uses, func(u policyUse) bool { return u.u.Alias == k && blocks[u.i] == r.Block && u.policy.model == g.m })
		if !ok {
			errAt(r.Pos, fmt.Sprintf("declare -- policy: %s %s and write %s(%s) in the WHERE of the SELECT that reads it (policies of %s: %s); or -- policy: none %s why, if it may read every row",
				g.ps[0].Name, k, g.ps[0].Name, k, g.m.Name, policyNames(g.ps), k),
				"query reads %s %s without a policy (rule 6)", r.Table, k)
		}
	}
	for _, u := range uses {
		at := toks[u.i].Pos
		var mine []tableRef
		for _, r := range refs {
			if key(r) == u.u.Alias && r.Block == blocks[u.i] {
				mine = append(mine, r)
			}
		}
		if len(mine) == 0 {
			errAt(at, fmt.Sprintf("write %s(%s) in the SELECT whose FROM or JOIN reads %s; for a derived table, inside it", u.u.Name, u.u.Alias, u.u.Alias),
				"%s(%s), but this SELECT reads no table as %s", u.u.Name, u.u.Alias, u.u.Alias)
			continue
		}
		if u.policy.model != nil && mine[0].Table != u.policy.model.Table {
			errAt(at, fmt.Sprintf("use a policy of the model of %s, or -- policy: none %s why", mine[0].Table, u.u.Alias),
				"%s reads %s, but policy %s is for %s", u.u.Alias, mine[0].Table, u.u.Name, u.policy.Model)
			continue
		}
		if conditional[at.Line] && !slices.ContainsFunc(mine, func(r tableRef) bool { return r.Pos.Line == at.Line }) {
			errAt(at, fmt.Sprintf("move %s(%s) to a line without a {condition}: it would be left out when the condition does not hold", u.u.Name, u.u.Alias),
				"policy %s on a line with a {condition}", u.u.Name)
			continue
		}
		if why := narrows(toks, u.i, refs, u.u.Alias); why != "" {
			errAt(at, fmt.Sprintf("write %s(%s) in WHERE, joined by AND, like WHERE … AND %s(%s)", u.u.Name, u.u.Alias, u.u.Name, u.u.Alias),
				"%s(%s) %s, so it may not narrow the rows", u.u.Name, u.u.Alias, why)
		}
	}
	for _, alias := range slices.Sorted(maps.Keys(none)) {
		u := none[alias]
		var read []tableRef
		for _, r := range refs {
			if key(r) == alias {
				read = append(read, r)
			}
		}
		switch {
		case len(read) == 0:
			errAt(u.Pos, "remove the -- policy: none line, or name the alias the query reads", "-- policy: none %s, but the query reads no table as %s", alias, alias)
		case !slices.ContainsFunc(read, func(r tableRef) bool { return byTable[r.Table] != nil }):
			errAt(u.Pos, "remove the -- policy: none line: only the tables of models with policies need one", "-- policy: none %s, but %s has no policy", alias, read[0].Table)
		case slices.ContainsFunc(read, func(r tableRef) bool { return r.Table != read[0].Table }):
			r := read[slices.IndexFunc(read, func(r tableRef) bool { return r.Table != read[0].Table })]
			errAt(r.Pos, "give each table its own alias, so that -- policy: none names one", "-- policy: none %s, but %s is %s here and %s at line %d", alias, alias, r.Table, read[0].Table, read[0].Pos.Line)
		}
	}
	return ds
}

// narrows tells why the use of a policy at toks[i], name(alias), may not
// narrow the rows; "" if it does. It does when it is an AND term of
// WHERE, or of the ON of the JOIN that reads alias, other than a RIGHT
// or FULL one: in parentheses too, with no OR anywhere on the way.
// keywords may stand before parentheses that only group.
var keywords = map[string]bool{"AND": true, "OR": true, "NOT": true, "WHERE": true, "ON": true, "HAVING": true, "WHEN": true, "THEN": true, "ELSE": true}

func narrows(toks []dialect.Token, i int, refs []tableRef, alias string) string {
	word := func(k int) string {
		if k >= 0 && k < len(toks) && toks[k].Kind == dialect.Word {
			return strings.ToUpper(toks[k].Text)
		}
		return ""
	}
	stop := func(w string) bool { return clauseWords[w] || joinWords[w] || w == "SELECT" || w == "FROM" }
	start, end := i, i+3
	for {
		switch n, w := end+1, word(end+1); {
		case n >= len(toks), w == "AND", is(toks[n], ")"), is(toks[n], "{"), is(toks[n], "}"), stop(w) && w != "ON" && w != "USING":
		default:
			return fmt.Sprintf("is followed by %q", toks[n].Text)
		}
		switch p, w := start-1, word(start-1); {
		case p < 0:
			return "is not in WHERE"
		case w == "AND", w == "WHERE", w == "ON", is(toks[p], "("), is(toks[p], "{"):
		default:
			return fmt.Sprintf("follows %q", toks[p].Text)
		}
		// Out to the clause, or to the parentheses around.
		depth := 0
		k := start - 1
		for ; k >= 0; k-- {
			t := toks[k]
			if is(t, ")") {
				depth++
				continue
			}
			if is(t, "(") {
				if depth == 0 {
					break
				}
				depth--
				continue
			}
			if depth > 0 {
				continue
			}
			if w := word(k); w == "OR" {
				return "is joined by OR"
			} else if w == "WHERE" || w == "ON" || w == "HAVING" || stop(w) {
				break
			}
		}
		// And to its end.
		depth = 0
		for e := end + 1; e < len(toks); e++ {
			t := toks[e]
			if is(t, "(") {
				depth++
			} else if is(t, ")") {
				if depth == 0 {
					break
				}
				depth--
			} else if w := word(e); depth == 0 && w == "OR" {
				return "is joined by OR"
			} else if depth == 0 && stop(w) && w != "ON" {
				break
			}
		}
		switch w := word(k); {
		case k < 0:
			return "is not in WHERE"
		case is(toks[k], "("):
			if pw := word(k - 1); k > 0 && (toks[k-1].Kind == dialect.Word || toks[k-1].Kind == dialect.Quoted) && !keywords[pw] {
				return "is inside " + toks[k-1].Text + "(…)"
			}
			start, end = k, dialect.Closing(toks, k)
			continue
		case w == "WHERE":
			return ""
		case w == "ON":
			j := k - 1
			for depth := 0; j >= 0; j-- {
				if is(toks[j], ")") {
					depth++
				} else if is(toks[j], "(") {
					depth--
				} else if depth == 0 && word(j) == "JOIN" {
					break
				}
			}
			for b := j - 1; b >= 0 && joinWords[word(b)]; b-- {
				if w := word(b); w == "RIGHT" || w == "FULL" {
					return "is in the ON of a " + w + " JOIN, which keeps the rows it does not match"
				}
			}
			for _, r := range refs {
				if r.Index > j && r.Index < k {
					if cmp.Or(r.Alias, r.Table) == alias {
						return ""
					}
					break
				}
			}
			return "is in the ON of a JOIN that reads another table, which keeps the rows of " + alias + " it does not match"
		case w == "HAVING":
			return "is in HAVING"
		}
		return "is not in WHERE"
	}
}
