package mapper

import (
	"fmt"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/model"
)

// Versions checks the UPDATE statements of qs against the models of apps
// that have a version, their first auto=update field (D-150): an UPDATE of
// such a table sets it from a time.Time parameter. Left as it was, a
// generated Update that read the row before would pass its check and undo
// the change; set by SQL, SQLite would hold it as other text than maago
// writes, and every Update of the row would be a conflict.
func Versions(apps []*model.App, qs []*Query) []diag.Diagnostic {
	versions := map[string]*model.Field{} // by lower-case table
	for _, a := range apps {
		for _, m := range a.Models {
			if f := m.Version(); f != nil {
				versions[strings.ToLower(m.Table)] = f
			}
		}
	}
	var ds []diag.Diagnostic
	for _, q := range qs {
		conditional := map[int]bool{} // the lines a call may leave out
		for _, l := range q.Lines {
			if l.Cond != nil {
				conditional[l.Pos.Line] = true
			}
		}
		toks := withoutConds(q.toks)
		for i, t := range toks {
			if t.Kind != dialect.Word || !strings.EqualFold(t.Text, "UPDATE") {
				continue
			}
			table, set := updated(toks, i)
			if table == "" { // INSERT INTO t … ON CONFLICT … DO UPDATE SET
				table = into(toks[:i])
			}
			v := versions[strings.ToLower(table)]
			if v == nil {
				continue
			}
			expr, ok := set[v.Column]
			switch {
			case !ok:
				ds = append(ds, diag.New(t.Pos, fmt.Sprintf("query %s updates %s without its version %s, so an Update that read the row before would undo this change", q.Name, table, v.Column),
					fmt.Sprintf("add , %s = :now to SET, with -- param: now time.Time", v.Column)))
			case !q.timeParam(expr):
				ds = append(ds, diag.New(t.Pos, fmt.Sprintf("query %s sets %s, the version of %s, to something other than a time.Time parameter", q.Name, v.Column, table),
					fmt.Sprintf("write %s = :now, with -- param: now time.Time: maago writes the version in the form it compares", v.Column)))
			case conditional[expr[0].Pos.Line]:
				ds = append(ds, diag.New(t.Pos, fmt.Sprintf("query %s sets %s, the version of %s, only under a condition, so a call without it would be undone by an Update that read the row before", q.Name, v.Column, table),
					fmt.Sprintf("set %s = :now on a line without a condition", v.Column)))
			}
		}
	}
	return ds
}

// withoutConds is toks without the {conditions} of lines and the braces
// of SET { } and WHERE { } (rule 3): an UPDATE's SET may be under them,
// and its columns are still its own. A condition opens and closes on its
// line; a region's brace is alone.
func withoutConds(toks []dialect.Token) []dialect.Token {
	var out []dialect.Token
	for i := 0; i < len(toks); i++ {
		if !is(toks[i], "{") && !is(toks[i], "}") {
			out = append(out, toks[i])
			continue
		}
		for j := i + 1; is(toks[i], "{") && j < len(toks) && toks[j].Pos.Line == toks[i].Pos.Line; j++ {
			if is(toks[j], "}") { // {cond}
				i = j
				break
			}
		}
	}
	return out
}

// updated reads the UPDATE at toks[at]: its table, and the expression of
// each column its SET assigns, by lower-case column name.
func updated(toks []dialect.Token, at int) (table string, set map[string][]dialect.Token) {
	i := at + 1
	if i < len(toks) && strings.EqualFold(toks[i].Text, "ONLY") {
		i++
	}
	for ; i < len(toks) && !(toks[i].Kind == dialect.Word && strings.EqualFold(toks[i].Text, "SET")); i++ {
		if (toks[i].Kind == dialect.Word || toks[i].Kind == dialect.Quoted) && table == "" {
			table = dialect.Name(toks[i])
		} else if is(toks[i], ".") && i+1 < len(toks) { // schema.table
			table = dialect.Name(toks[i+1])
			i++
		}
	}
	set = map[string][]dialect.Token{}
	var col string
	var expr []dialect.Token
	flush := func() {
		if col != "" {
			set[strings.ToLower(col)] = expr
		}
		col, expr = "", nil
	}
	for i++; i < len(toks); i++ {
		t := toks[i]
		switch {
		case t.Kind == dialect.Word && (strings.EqualFold(t.Text, "WHERE") || strings.EqualFold(t.Text, "RETURNING") || strings.EqualFold(t.Text, "FROM")) || is(t, ")"): // ) ends a WITH's
			flush()
			return table, set
		case is(t, "("):
			end := dialect.Closing(toks, i)
			if col != "" {
				expr = append(expr, toks[i:end+1]...)
			} else { // (a, b) = (…): each column set, but not as col = :param
				for _, c := range toks[i+1 : end] {
					if c.Kind == dialect.Word || c.Kind == dialect.Quoted {
						set[strings.ToLower(dialect.Name(c))] = nil
					}
				}
				if end+2 < len(toks) && is(toks[end+1], "=") && is(toks[end+2], "(") {
					end = dialect.Closing(toks, end+2)
				}
			}
			i = end
		case is(t, ","):
			flush()
		case col == "" && (t.Kind == dialect.Word || t.Kind == dialect.Quoted):
			col = dialect.Name(t)
			if i+1 < len(toks) && is(toks[i+1], "=") {
				i++
			}
		default:
			expr = append(expr, t)
		}
	}
	flush()
	return table, set
}

// into is the table of the last INSERT INTO in toks, or "".
func into(toks []dialect.Token) string {
	for i := len(toks) - 2; i >= 0; i-- {
		if toks[i].Kind == dialect.Word && strings.EqualFold(toks[i].Text, "INTO") {
			if i+3 < len(toks) && is(toks[i+2], ".") { // schema.table
				return dialect.Name(toks[i+3])
			}
			return dialect.Name(toks[i+1])
		}
	}
	return ""
}

// timeParam reports whether expr is :name alone, a required time.Time
// parameter.
func (q *Query) timeParam(expr []dialect.Token) bool {
	if len(expr) != 2 || !is(expr[0], ":") || expr[1].Kind != dialect.Word {
		return false
	}
	for _, p := range q.Params {
		if p.Name == expr[1].Text {
			return p.Type.Base == model.Time && !p.Type.Slice && !p.Optional
		}
	}
	return false
}
