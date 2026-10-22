package mapper

import (
	"go/token"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
)

// A tableRef is a place where a query reads a table (D-155).
type tableRef struct {
	Table string         // lower case, without a schema
	Alias string         // lower case unless quoted; "" if none
	Pos   token.Position // of the first token of the name, the schema if any
}

// A cte is a name a WITH defines. It hides the table of that name in
// toks[from] through toks[to]: after its body, or from the WITH
// if RECURSIVE, to the end of the parentheses holding the WITH.
type cte struct {
	name     string // as fold gives it
	from, to int
}

func ctes(toks []dialect.Token) []cte {
	var cs []cte
	for i, t := range toks {
		if t.Kind != dialect.Word || !strings.EqualFold(t.Text, "WITH") {
			continue
		}
		end, depth := len(toks)-1, 0
		for k := i; k < len(toks); k++ {
			if is(toks[k], "(") {
				depth++
			} else if is(toks[k], ")") {
				if depth == 0 {
					end = k
					break
				}
				depth--
			}
		}
		k := i + 1
		recursive := k < len(toks) && strings.EqualFold(toks[k].Text, "RECURSIVE")
		if recursive {
			k++
		}
		for k < len(toks) {
			if toks[k].Kind != dialect.Word && toks[k].Kind != dialect.Quoted {
				break
			}
			c := cte{name: fold(toks[k]), from: i, to: end}
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
			if !recursive {
				c.from = k
			}
			cs = append(cs, c)
			if k >= len(toks) || !is(toks[k], ",") {
				break
			}
			k++
		}
	}
	return cs
}

// fold is the name a Word or Quoted token stands for, as PostgreSQL
// compares names: a quoted one keeps its case. SQLite ignores the case of
// quoted names too, so there a quoted CTE may hide less than it does.
func fold(t dialect.Token) string {
	if t.Kind == dialect.Quoted {
		return dialect.Name(t)
	}
	return strings.ToLower(t.Text)
}

var (
	// clauseWords end a FROM list.
	clauseWords = map[string]bool{
		"WHERE": true, "GROUP": true, "HAVING": true, "WINDOW": true, "ORDER": true, "LIMIT": true, "OFFSET": true,
		"FETCH": true, "FOR": true, "UNION": true, "INTERSECT": true, "EXCEPT": true, "RETURNING": true, "SET": true,
	}
	// joinWords may follow a table in a FROM list, besides clauseWords.
	joinWords = map[string]bool{
		"ON": true, "USING": true, "JOIN": true, "INNER": true, "LEFT": true, "RIGHT": true, "FULL": true,
		"OUTER": true, "CROSS": true, "NATURAL": true,
	}
)

// tables lists where the statement reads tables: the items of FROM
// lists and joins, in subqueries, CTEs and every SELECT of a UNION too,
// but not the tables that INSERT, UPDATE or DELETE write. It reports the
// shapes that may read a table it cannot name.
func (q *Query) tables() ([]tableRef, []diag.Diagnostic) {
	toks := q.toks
	cs := ctes(toks)
	word := func(i int) string {
		if i >= 0 && i < len(toks) && toks[i].Kind == dialect.Word && (i == 0 || !is(toks[i-1], ":")) { // not :from
			return strings.ToUpper(toks[i].Text)
		}
		return ""
	}
	name := func(i int) bool {
		return i < len(toks) && (toks[i].Kind == dialect.Word || toks[i].Kind == dialect.Quoted)
	}
	fail := func(i int, why string) ([]tableRef, []diag.Diagnostic) {
		i = min(i, len(toks)-1)
		return nil, []diag.Diagnostic{diag.New(toks[i].Pos, "cannot tell which tables the query reads: "+why,
			"name the table in FROM or JOIN, or split the query")}
	}
	// The {condition} that starts a line is not SQL (body.go).
	cond := make([]bool, len(toks))
	for i, t := range toks {
		if is(t, "{") && (i == 0 || toks[i-1].Pos.Line != t.Pos.Line) {
			for k := i; k < len(toks) && !cond[k] && (k == i || !is(toks[k-1], "}")); k++ {
				cond[k] = true
			}
		}
	}
	// What each open parenthesis has seen: a verb, so FROM starts a
	// FROM list, and whether it is in one, so a comma starts an item.
	type level struct{ verb, list bool }
	levels := []level{{}}
	var refs []tableRef
	for i, t := range toks {
		if cond[i] {
			continue
		}
		top := &levels[len(levels)-1]
		switch w := word(i); {
		case is(t, "("):
			levels = append(levels, level{})
			continue
		case is(t, ")"):
			if len(levels) > 1 {
				levels = levels[:len(levels)-1]
			}
			continue
		case is(t, ","):
			if !top.list {
				continue
			}
		case w == "SELECT" || w == "UPDATE" || w == "DELETE":
			top.verb = true
			continue
		case w == "TABLE" && (i == 0 || is(toks[i-1], "(") || word(i-1) == "UNION" || word(i-1) == "INTERSECT" || word(i-1) == "EXCEPT" || word(i-1) == "ALL"):
			what := t.Text
			if i+1 < len(toks) {
				what += " " + toks[i+1].Text
			}
			return fail(i, what)
		case w == "FROM":
			isDistinct := word(i-1) == "DISTINCT" && (word(i-2) == "IS" || word(i-2) == "NOT" && word(i-3) == "IS")
			if !top.verb || isDistinct || word(i-1) == "DELETE" { // EXTRACT(year FROM x), the table DELETE writes
				continue
			}
		case w == "JOIN":
		case w == "USING":
			if i+1 < len(toks) && is(toks[i+1], "(") { // JOIN … USING (a)
				continue
			}
		case clauseWords[w]:
			top.list = false
			continue
		default:
			continue
		}
		top.list = true
		k := i + 1
		if word(k) == "LATERAL" {
			k++
		}
		if word(k) == "ONLY" {
			k++
		}
		if k < len(toks) && is(toks[k], "(") {
			if w := word(k + 1); w != "SELECT" && w != "WITH" && w != "VALUES" {
				return fail(k, "a join in parentheses")
			}
			continue // a derived table: its tables are read where they are
		}
		if !name(k) {
			return fail(k, t.Text+" not followed by a table name")
		}
		start := k
		for name(k+2) && is(toks[k+1], ".") { // schema.table
			k += 2
		}
		if k+1 < len(toks) && is(toks[k+1], "(") {
			continue // a table function, like json_each(x)
		}
		ref := tableRef{Table: strings.ToLower(dialect.Name(toks[k])), Pos: toks[start].Pos}
		bare := k == start // a CTE has no schema
		id := fold(toks[k])
		k++
		if word(k) == "AS" {
			if !name(k + 1) {
				return fail(k, "AS not followed by a name")
			}
			k++
		}
		if name(k) && !clauseWords[word(k)] && !joinWords[word(k)] {
			ref.Alias = fold(toks[k])
			k++
		}
		if k < len(toks) && !clauseWords[word(k)] && !joinWords[word(k)] && !is(toks[k], ",") && !is(toks[k], ")") && !is(toks[k], "{") && !is(toks[k], "}") {
			return fail(k, toks[k].Text+" after the table "+toks[start].Text)
		}
		hidden := false
		for _, c := range cs {
			hidden = hidden || bare && c.name == id && c.from <= start && start <= c.to
		}
		if !hidden {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}
