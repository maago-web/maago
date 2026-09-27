package mapper

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/maago-web/maago/internal/project"
)

func variants(t *testing.T, src string) ([]Axis, []Variant, string) {
	t.Helper()
	q, out := readBody(t, src)
	if out != "" {
		t.Fatalf("%q: %s", src, out)
	}
	axes, vs, ds := q.Variants()
	return axes, vs, text(ds)
}

// render writes the variants the way the hand-written files do, slices
// with two elements.
func render(axes []Axis, vs []Variant, d project.Dialect) string {
	var out []string
	for i, v := range vs {
		s := fmt.Sprintf("-- variant %d, args: %s; slices:", i+1, strings.Join(v.Args, ", "))
		if len(v.Slices) > 0 {
			s += " " + strings.Join(v.Slices, ", ")
		}
		s += "\n"
		for _, c := range v.Combos {
			s += "-- " + describe(axes, c) + "\n"
		}
		out = append(out, s+v.SQL(d, 2)+"\n")
	}
	return strings.Join(out, "\n")
}

const deletePosts = "-- name: DeletePosts :exec\n" +
	"-- param: ids []int64\n" +
	"-- param: flag bool\n" +
	"-- param: board int64\n" +
	"DELETE FROM posts WHERE {\n" +
	"  {ids} id IN (:ids)\n" +
	"  {!flag} OR draft\n" +
	"  AND board_id = :board\n" +
	"}\n"

// The middle line has no comma: maago does not add one; the prepare
// (task 4) reports the SQL.
const updatePost = "-- name: UpdatePost :exec\n" +
	"-- param: id int64\n" +
	"-- param: title? string\n" +
	"-- param: body? string\n" +
	"-- param: now time.Time\n" +
	"UPDATE posts SET {\n" +
	"  {title} title = :title,\n" +
	"  {body}  body = :body\n" +
	"  updated_at = :now,\n" +
	"}\n" +
	"WHERE id = :id\n"

// The expected files are written by hand, not by -update: generating
// them from the code would pass whatever the code does.
func TestVariants(t *testing.T) {
	search, err := os.ReadFile("../testdata/orders/queries/search_orders.sql")
	if err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string]string{
		"search_orders": string(search),
		"delete_posts":  deletePosts,
		"update_post":   updatePost,
	} {
		axes, vs, errs := variants(t, src)
		if errs != "" {
			t.Errorf("%s: %s", name, errs)
			continue
		}
		for _, d := range []project.Dialect{project.SQLite, project.Postgres} {
			path := fmt.Sprintf("testdata/%s.%s.sql", name, d)
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := render(axes, vs, d); got != string(want) {
				t.Errorf("%s differs from what %s gives:\n%s", path, name, got)
			}
		}
	}
}

// The fixture is the SearchOrders example of DECISIONS, to the letter.
func TestSearchOrdersIsTheDecisionsExample(t *testing.T) {
	doc, err := os.ReadFile("../../docs/DECISIONS.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(doc)
	s = s[strings.Index(s, "**SQL 규칙 5개**"):]
	s = s[strings.Index(s, "```sql\n")+len("```sql\n"):]
	s = s[:strings.Index(s, "```")]
	fixture, err := os.ReadFile("../testdata/orders/queries/search_orders.sql")
	if err != nil {
		t.Fatal(err)
	}
	if string(fixture) != s {
		t.Errorf("search_orders.sql differs from DECISIONS:\n%s", s)
	}
}

// An emptied WHERE { } of a SELECT goes with its keyword (rule 4).
func TestEmptyWhere(t *testing.T) {
	_, vs, errs := variants(t, "-- name: Q :many R\n-- param: flag bool\nSELECT 1 FROM t WHERE {\n  {flag} x\n}\n")
	if errs != "" || len(vs) != 2 || vs[0].SQL(project.SQLite, 1) != "SELECT 1 FROM t" {
		t.Errorf("got %d variants, first %q, %s; want the first to be SELECT 1 FROM t", len(vs), vs[0].SQL(project.SQLite, 1), errs)
	}
}

// Texts that read the same but bind other parameters are other variants.
func TestSameTextOtherArgs(t *testing.T) {
	_, vs, errs := variants(t, "-- name: Q :many R\n-- param: a? int64\n-- param: b int64\n-- param: c int64\n"+
		"-- param: ids []int64\n-- param: ids2 []int64\nSELECT 1\n{a} x = :b AND y IN (:ids)\n{!a} x = :c AND y IN (:ids2)\n")
	if errs != "" || len(vs) != 2 {
		t.Fatalf("got %d variants, %s; want 2", len(vs), errs)
	}
	if got := fmt.Sprint(vs[0].Args, vs[0].Slices, vs[1].Args, vs[1].Slices); got != "[c] [ids2] [b] [ids]" {
		t.Errorf("got %s, want [c] [ids2] [b] [ids]", got)
	}
}

func TestVariantsWithoutAxes(t *testing.T) {
	axes, vs, errs := variants(t, "-- name: Q :exec\n-- param: id int64\nDELETE FROM t WHERE id = :id\n")
	if errs != "" || len(axes) != 0 || len(vs) != 1 || len(vs[0].Combos) != 1 || len(vs[0].Combos[0]) != 0 {
		t.Errorf("got %v %+v %s, want no axes and one variant for the empty combination", axes, vs, errs)
	}
}

// Scalars are numbered in order of first use, slices after them in
// order of first use; a repeated parameter reuses its numbers.
func TestPlaceholders(t *testing.T) {
	_, vs, errs := variants(t, "-- name: Q :many R\n"+
		"-- param: a int64\n-- param: b int64\n-- param: ids []int64\n-- param: ids2 []int64\n"+
		"SELECT x FROM t WHERE a = :a AND id IN (:ids) AND b = :b AND c = :a AND d IN (:ids2) AND e IN (:ids)\n")
	if errs != "" || len(vs) != 1 {
		t.Fatalf("got %d variants, %s", len(vs), errs)
	}
	v := vs[0]
	if fmt.Sprint(v.Args, v.Slices) != "[a b] [ids ids2]" {
		t.Errorf("args %v, slices %v; want [a b] [ids ids2]", v.Args, v.Slices)
	}
	for _, c := range []struct {
		d    project.Dialect
		n    int
		want string
	}{
		{project.Postgres, 2, "SELECT x FROM t WHERE a = $1 AND id IN ($3, $4) AND b = $2 AND c = $1 AND d IN ($5, $6) AND e IN ($3, $4)"},
		{project.Postgres, 1, "SELECT x FROM t WHERE a = $1 AND id IN ($3) AND b = $2 AND c = $1 AND d IN ($4) AND e IN ($3)"},
		{project.SQLite, 2, "SELECT x FROM t WHERE a = ?1 AND id IN (?3, ?4) AND b = ?2 AND c = ?1 AND d IN (?5, ?6) AND e IN (?3, ?4)"},
	} {
		if got := v.SQL(c.d, c.n); got != c.want {
			t.Errorf("%s, %d elements:\ngot  %s\nwant %s", c.d, c.n, got, c.want)
		}
	}
}

func TestVariantErrors(t *testing.T) {
	const opt = "-- name: Q :exec\n-- param: x? int64\n-- param: y? int64\n-- param: z? int64\n-- param: flag bool\n"
	var many strings.Builder // 9 bools: 512 combinations
	many.WriteString("-- name: Q :many R\n")
	for i := range 9 {
		fmt.Fprintf(&many, "-- param: b%d bool\n", i)
	}
	many.WriteString("SELECT 1\n")
	for i := range 9 {
		fmt.Fprintf(&many, "{b%d} , %d\n", i, i)
	}
	var texts strings.Builder // 7 bools, a line each: 128 texts
	texts.WriteString("-- name: Q :many R\n")
	for i := range 7 {
		fmt.Fprintf(&texts, "-- param: b%d bool\n", i)
	}
	texts.WriteString("SELECT 1\n")
	for i := range 7 {
		fmt.Fprintf(&texts, "{b%d} , %d\n", i, i)
	}
	for _, c := range []struct{ src, want string }{
		{"-- name: Q :exec\n-- param: ids []int64\nDELETE FROM t WHERE {\n  {ids} id IN (:ids)\n}\n",
			"blog/queries/q.sql:3:15: WHERE { } of DELETE is empty when ids empty, so it would touch every row"},
		{"-- name: Q :exec\n-- param: flag bool\nWITH d AS (SELECT 1)\nDELETE FROM t WHERE {\n  {flag} x\n}\n",
			"blog/queries/q.sql:4:15: WHERE { } of DELETE is empty when flag false, so it would touch every row"},
		{"-- name: Q :exec\n-- param: flag bool\nUPDATE t SET x = 1 FROM (SELECT 1) s WHERE {\n  {flag} y\n}\n",
			"blog/queries/q.sql:3:38: WHERE { } of UPDATE is empty when flag false, so it would touch every row"},
		{"-- name: Q :exec\n-- param: flag bool\nMERGE INTO t USING u ON true WHERE {\n  {flag} y\n}\n",
			"blog/queries/q.sql:3:30: WHERE { } of this statement is empty when flag false, so it would touch every row"},
		{"-- name: Q :exec\n-- param: flag bool\n-- param: id int64\nUPDATE t SET {\n  {flag} x = 1,\n}\nWHERE id = :id\n",
			"blog/queries/q.sql:4:10: SET { } is empty when flag false"},
		{opt + "SELECT :x, :y, :z\n{flag} AND 1\n",
			"blog/queries/q.sql:6:8: optional parameter x is used on a line that does not require {x}"},
		{opt + "SELECT 1\n{x | flag} AND :x, :y, :z\n",
			"blog/queries/q.sql:7:16: optional parameter x is used on a line that does not require {x}"},
		{opt + "SELECT 1\n{!x & flag} AND :x, :y, :z\n",
			"blog/queries/q.sql:7:17: optional parameter x is used on a line that does not require {x}"},
		{opt + "SELECT 1\n{x & y & flag} AND :x, :y, :z\n",
			"blog/queries/q.sql:7:28: optional parameter z is used on a line that does not require {z}"},
		{many.String(), "blog/queries/q.sql:1:10: query Q has 512 combinations of condition values, over 256"},
		{texts.String(), "blog/queries/q.sql:1:10: query Q has 128 distinct SQL texts, over 64"},
	} {
		_, _, got := variants(t, c.src)
		if first, _, _ := strings.Cut(got, "\n"); first != c.want {
			t.Errorf("%q:\ngot  %s\nwant %s", c.src, got, c.want)
		}
	}
	// A region is reported once, at the first combination that empties it.
	_, _, got := variants(t, "-- name: Q :exec\n-- param: a bool\n-- param: b bool\n-- param: c bool\n"+
		"DELETE FROM t WHERE {\n  {!a} x\n  {b & c} y\n}\n")
	if want := "blog/queries/q.sql:5:15: WHERE { } of DELETE is empty when a true, b false, c false, so it would touch every row\n" +
		"  fix: make one line of WHERE { } unconditional, or check in Go before calling the query\n"; got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	// Exactly at the limits is fine.
	var at256, at64 strings.Builder
	at256.WriteString("-- name: Q :many R\n")
	at64.WriteString("-- name: Q :many R\n")
	for i := range 8 {
		fmt.Fprintf(&at256, "-- param: b%d bool\n", i)
	}
	for i := range 6 {
		fmt.Fprintf(&at64, "-- param: b%d bool\n", i)
	}
	at256.WriteString("SELECT 1\n{b0 & b1 & b2 & b3 & b4 & b5 & b6 & b7} , 1\n")
	at64.WriteString("SELECT 1\n")
	for i := range 6 {
		fmt.Fprintf(&at64, "{b%d} , %d\n", i, i)
	}
	for _, src := range []string{at256.String(), at64.String()} {
		if _, _, got := variants(t, src); got != "" {
			t.Errorf("%q: unexpected errors:\n%s", src, got)
		}
	}
	// WHERE may empty out of a SELECT, whatever comes before it.
	for _, src := range []string{
		"-- name: Q :many R\n-- param: flag bool\nSELECT 1 FROM t WHERE {\n  {flag} x\n}\n",
		"-- name: Q :many R\n-- param: flag bool\nWITH d AS (DELETE FROM u)\nSELECT 1 FROM t WHERE {\n  {flag} x\n}\n",
		"-- name: Q :exec\n-- param: flag bool\nINSERT INTO t SELECT 1 FROM u WHERE {\n  {flag} x\n}\n",
		"-- name: Q :exec\n-- param: flag bool\n-- param: id int64\nDELETE FROM t WHERE {\n  {flag} x\n  AND id = :id\n}\n",
		"-- name: Q :exec\n-- param: x? int64\n-- param: y? int64\n-- param: flag bool\nSELECT 1\n{x & y & flag} AND :x, :y\n",
	} {
		if _, _, got := variants(t, src); got != "" {
			t.Errorf("%q: unexpected errors:\n%s", src, got)
		}
	}
}

// A string that spans lines moves the lines after it.
func TestPositionAfterMultilineString(t *testing.T) {
	_, vs, errs := variants(t, "-- name: Q :many R\nSELECT 'a\nb', nope\n")
	if errs != "" {
		t.Fatal(errs)
	}
	text := vs[0].SQL(project.Postgres, 1)
	char := utf8.RuneCountInString(text[:strings.Index(text, "nope")]) + 1
	if p := vs[0].Position(project.Postgres, 1, char); p.Line != 3 || p.Column != 5 {
		t.Errorf("nope: got %d:%d, want 3:5", p.Line, p.Column)
	}
}

// A character of a variant's text maps to its place in the query file.
func TestPosition(t *testing.T) {
	src, err := os.ReadFile("../testdata/orders/queries/search_orders.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, vs, errs := variants(t, string(src))
	if errs != "" {
		t.Fatal(errs)
	}
	v := vs[6] // status present, from and to present, sort=latest
	text := v.SQL(project.Postgres, 1)
	for _, c := range []struct {
		find string
		line int
		col  int
	}{
		{"SELECT", 7, 1},
		{"created_at BETWEEN", 10, 25},
		{"AND $3", 10, 50}, // SQL after a placeholder
		{"$3", 10, 54},     // the placeholder is where :to is
		{"status = $1", 9, 25},
		{"LIMIT", 16, 1},
		{"  AND created_at", 10, 1},    // indentation
		{"\n  AND created_at", 10, 25}, // a line break, at the line it starts
	} {
		char := utf8.RuneCountInString(text[:strings.Index(text, c.find)]) + 1
		if p := v.Position(project.Postgres, 1, char); p.Line != c.line || p.Column != c.col {
			t.Errorf("%q at character %d: got %d:%d, want %d:%d", c.find, char, p.Line, p.Column, c.line, c.col)
		}
	}
}
