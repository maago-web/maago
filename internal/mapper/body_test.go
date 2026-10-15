package mapper

import (
	"fmt"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/golden"
)

// head declares every kind of parameter the cases use.
const head = "-- name: Q :many R\n" +
	"-- param: status? string\n" +
	"-- param: flag bool\n" +
	"-- param: ids []int64\n" +
	"-- param: sort latest/amount\n" +
	"-- param: limit int64\n" +
	"-- param: data? []byte\n"

// uses uses every parameter of head, so that none is reported unused.
const uses = ":limit :status :flag :ids :sort :data"

func readBody(t *testing.T, src string) (*Query, string) {
	t.Helper()
	q, ds := parse(file, src)
	if ds != nil {
		t.Fatalf("header of %q: %s", src, text(ds))
	}
	return q, text(q.body())
}

func TestBodyErrors(t *testing.T) {
	var all string
	for _, c := range []struct {
		body, want string
		alone      bool // without uses
	}{
		// From the task.
		{"SELECT :x\n", `8:8: parameter x is not declared`, false},
		{"SELECT 1 LIMIT :lim OFFSET :off\n", `8:16: parameter lim is not declared`, false}, // and offset, in the same run
		{"SELECT 1\n{status & flag | ids} AND 1\n", `9:16: condition mixes & and |`, false},
		{"SELECT 1\n{(status)} AND 1\n", `9:2: parentheses in a condition`, false},
		{"SELECT 1\n{sort=oldest} AND 1\n", `9:7: oldest is not a choice of sort`, false},
		{"SELECT 1\n{limit} AND 1\n", `9:2: {limit} is always true: limit is required`, false},
		{"SELECT 1 WHERE {\n{status} AND status = :status\n", `8:10: WHERE { is not closed`, false},
		{"SELECT :2", `8:8: parameter 2 is not declared`, false},
		{"SELECT a[1:2]", `8:11: parameter 2 is not declared`, false},
		{"SELECT :한글", `8:8: parameter 한글 is not declared`, false},
		// Independent errors on separate lines are all reported.
		{"SELECT :x\n{sort} AND 1\nWHERE x = {", `8:8: parameter x is not declared`, false},
		{"SELECT 1\n{Status} AND 1\n", `9:2: parameter Status is not declared`, false},
		{"SELECT :Status", `8:8: parameter Status is not declared`, false},
		// Statements.
		{"SELECT 1;\nSELECT 2", `9:1: second statement in a query file`, false},
		{"", `8:1: query has no SQL`, true},
		{"SELECT 'x", `8:8: unterminated string`, false},
		// Placeholders.
		{"SELECT $1", `8:8: placeholder $1 in a query`, false},
		{"SELECT $name", `8:8: placeholder $name in a query`, false},
		{"SELECT @x", `8:8: placeholder @x in a query`, false},
		{"SELECT ?", `8:8: placeholder ? in a query`, false},
		{"SELECT ?1", `8:8: placeholder ? in a query`, false},
		{"SELECT j ?| array['a']", `8:10: placeholder ? in a query`, false},
		// Conditions.
		{"SELECT 1\n{status=a} AND 1\n", `9:2: {status=…} needs choices, but status is string`, false},
		{"SELECT 1\n{!sort=latest} AND 1\n", `9:2: !name=value is not a condition`, false},
		{"SELECT 1\n{status flag} AND 1\n", `9:9: unexpected "flag" in condition`, false},
		{"SELECT 1\n{} AND 1\n", `9:1: empty condition`, false},
		{"SELECT 1\n{status AND 1\n", `9:1: condition is not closed`, false},
		{"SELECT 1\n{status}\n", `9:1: condition has no SQL after it`, false},
		{"SELECT 1\n{sort} AND 1\n", `9:2: {sort} is always true: sort always has a choice`, false},
		{"SELECT 1\n{stauts} AND 1\n", `9:2: parameter stauts is not declared`, false},
		{"SELECT 1\n{status &} AND 1\n", `9:9: condition ends with &`, false},
		{"SELECT 1\n{!} AND 1\n", `9:2: condition needs a parameter name here`, false},
		{"SELECT 1\n{sort=} AND 1\n", `9:6: = needs a value after it`, false},
		// Regions.
		{"SELECT 1 WHERE {\nAND x IN (SELECT y FROM u WHERE {\n{status} AND y = :status\n}\n)\n}", `9:27: WHERE { inside WHERE { at line 8`, false},
		{"SELECT 1 WHERE x IN (SELECT y FROM u WHERE {\n{status} AND y = :status\n})", `8:38: WHERE { inside parentheses`, false},
		{"SELECT 1 WHERE {\n}", `8:10: WHERE { } is empty`, false},
		{"SELECT 1\n}", `9:1: } closes no region`, false},
		{"SELECT 1 WHERE x = {", `8:20: unexpected {`, false},
		{"SELECT 1\n{status} WHERE {\nAND 1\n}", `9:1: condition on the line that opens WHERE {`, false},
		{"SELECT 1 WHERE {\nAND 1\n{status} }", `10:1: condition on the } that closes WHERE {`, false},
		{"SELECT 1 WHERE\n{\nAND 1\n}", `9:1: { on the line after WHERE`, false},
		{"SELECT 1 WHERE x IN (\nSELECT y FROM u WHERE {\nAND 1\n}\n)", `9:17: WHERE { inside parentheses`, false},
		{"SELECT\n{", `9:1: condition is not closed`, false},
		{"SELECT 1 WHERE {\n{status} AND\n}", `9:10: AND with nothing after it`, false},
		{"UPDATE t SET {\n{status} ,\n}", `9:10: comma with nothing before it`, false},
		{"SELECT 1\n} ORDER BY x", `9:1: unexpected }`, false},
		{"SELECT 1\n{!status=x} AND 1\n", `9:2: !name=value is not a condition`, false},
		// Comments.
		{"SELECT 1\n-- param: x int64\n", `9:4: -- param: comment after the SQL`, false},
		{"SELECT 1\n-- maago: allow-drop\n", `9:4: -- maago: comment after the SQL`, false},
		// Unused, reported only when the rest is right.
		{"SELECT 1\n{limit} AND x = :status\n:flag :ids :data", `5:11: parameter sort is not used`, true},
		// Unused.
		{"SELECT 1", `2:11: parameter status is not used`, true},
	} {
		src := head + c.body
		if !c.alone {
			src += "\n" + uses
		}
		_, out := readBody(t, src)
		first, _, _ := strings.Cut(out, "\n")
		if first != file+":"+c.want {
			t.Errorf("%q:\ngot  %s\nwant %s:%s", c.body, out, file, c.want)
		}
		all += fmt.Sprintf("%q\n%s\n", c.body, out)
	}
	golden.Check(t, "testdata/body_errors.golden", []byte(all))
}

// dump writes q.Lines one per line, with everything a Line holds.
func dump(q *Query) string {
	var b strings.Builder
	for _, l := range q.Lines {
		fmt.Fprintf(&b, "%d:%d %s", l.Pos.Line, l.Pos.Column, l.Kind)
		if l.Keyword != "" {
			fmt.Fprintf(&b, " %s verb=%s", l.Keyword, l.Verb)
		}
		if c := l.Cond; c != nil {
			op := " & "
			if c.Or {
				op = " | "
			}
			var terms []string
			for _, t := range c.Terms {
				s := fmt.Sprintf("%d:%d ", t.Pos.Line, t.Pos.Column)
				if t.Not {
					s += "!"
				}
				s += t.Name
				if t.Value != "" {
					s += "=" + t.Value
				}
				terms = append(terms, s)
			}
			fmt.Fprintf(&b, " cond@%d:%d%s{%s}", c.Pos.Line, c.Pos.Column, strings.TrimSpace(op), strings.Join(terms, op))
		}
		if l.Lead != "" {
			fmt.Fprintf(&b, " lead=%s", l.Lead)
		}
		if l.Trail {
			b.WriteString(" trail")
		}
		for _, p := range l.Parts {
			if p.Param != "" {
				fmt.Fprintf(&b, "\n    %d:%d param %s", p.Pos.Line, p.Pos.Column, p.Param)
			} else {
				fmt.Fprintf(&b, "\n    %d:%d sql %q", p.Pos.Line, p.Pos.Column, p.SQL)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func TestBody(t *testing.T) {
	var all string
	for _, src := range []string{
		// DECISIONS, SQL rules, as written there.
		"-- name: SearchOrders :many OrderSummary\n" +
			"-- param: status?    string\n" +
			"-- param: from?      time.Time\n" +
			"-- param: to?        time.Time\n" +
			"-- param: sort       latest/amount\n" +
			"-- param: limit      int64\n" +
			"SELECT id, status, amount, created_at FROM orders\n" +
			"WHERE {\n" +
			"  {status}          AND status = :status\n" +
			"  {from & to}       AND created_at BETWEEN :from AND :to\n" +
			"  {!status}         AND status <> 'deleted'\n" +
			"}\n" +
			"ORDER BY\n" +
			"  {sort=latest}     created_at DESC\n" +
			"  {sort=amount}     amount DESC\n" +
			"LIMIT :limit\n",
		// SET { } with trailing commas, a | condition, and CRLF.
		"-- name: UpdatePost :exec\r\n" +
			"-- param: id int64\r\n" +
			"-- param: title? string\r\n" +
			"-- param: body? string\r\n" +
			"UPDATE posts SET {\r\n" +
			"  {title} title = :title,\r\n" +
			"  {body | title} body = :body,\r\n" +
			"}\r\n" +
			"WHERE id = :id;\r\n",
		// A region opened after SQL on the same line; OR as a lead.
		"-- name: DeletePosts :exec\n" +
			"-- param: ids []int64\n" +
			"-- param: flag bool\n" +
			"DELETE FROM posts WHERE {\n" +
			"  {ids} id IN (:ids)\n" +
			"  {!flag} or draft\n" +
			"}\n",
		// Trimming is only for its own region: a comma in WHERE { } and an
		// AND in SET { } stay. A ) earlier on the line closes before WHERE {.
		"-- name: Keep :exec\n" +
			"-- param: flag bool\n" +
			"UPDATE t SET {\n" +
			"  {flag} AND = 1,\n" +
			"}\n" +
			"FROM (SELECT 1) AS s WHERE {\n" +
			"  {flag} x = 1,\n" +
			"}\n",
		// What is not a binding, a placeholder or a condition.
		"-- name: Odd :many R\n" +
			"-- param: from time.Time\n" +
			"-- param: xfoo int64\n" +
			"-- param: data? []byte\n" +
			"-- param: flag bool\n" +
			"SELECT a::text, a[1: 2], foo$bar, $tag$ :x ? $1 $tag$, '{1,2}', ' :x ', \"a:b\" -- :x {a}\n" +
			"  /* :x { } ? */ , :from::date, :xfoo, x = : x\n" +
			"{data} AND d = :data\n" +
			"{flag} AND e\n",
	} {
		q, out := readBody(t, src)
		if out != "" {
			t.Errorf("%q: unexpected errors:\n%s", src, out)
			continue
		}
		// Joined, the parts of a line are its source, byte for byte.
		for _, l := range q.Lines {
			var joined strings.Builder
			for _, p := range l.Parts {
				joined.WriteString(p.SQL)
				if p.Param != "" {
					joined.WriteString(":" + p.Param)
				}
			}
			if len(l.Parts) > 0 && !strings.HasPrefix(src[l.Parts[0].Pos.Offset:], joined.String()) {
				t.Errorf("parts of line %d do not join to its source: %q", l.Pos.Line, joined.String())
			}
		}
		all += fmt.Sprintf("%q\n%s\n", src, dump(q))
	}
	golden.Check(t, "testdata/body.golden", []byte(all))
}
