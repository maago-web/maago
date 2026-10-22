package mapper

import (
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/golden"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/model/modeltest"
	"github.com/maago-web/maago/internal/project"
)

func TestPolicyFile(t *testing.T) {
	const file = "board/policies.sql"
	for _, c := range []struct{ src, want string }{
		{"-- name: a Post p\nNOT p.hidden\n-- name: b Post q\n-- get: GetPost\n-- param: now time.Time\nq.at < :now", ""},
		{"", "1:1: policies.sql has no policy"},
		{"NOT hidden\n-- name: a Post p\nx", "1:1: SQL before the first -- name: line"},
		{"-- param: x int64\n-- name: a Post p\nx", "1:4: -- param: before the first -- name: line"},
		{"-- name: a Post\nx", "1:16: -- name: line of a policy has 2 words, not 3"},
		{"-- name: Visible Post p\nx", `1:10: policy name "Visible" is not lowercase snake_case`},
		{"-- name: none Post p\nx", "1:10: policy name none is taken"},
		{"-- name: a post p\nx", `1:12: "post" is not a model's name`},
		{"-- name: a Post P\nx", `1:17: alias "P" is not lowercase snake_case`},
		{"-- name: a Post p\nx\n-- name: a Post p\nx", "3:10: policy a is also at line 1"},
		{"-- name: a Post p\n-- policy: a p\nx", `2:4: unknown header key "policy" in policies.sql`},
		{"-- name: a Post p\n-- nam: x\nx", `2:4: unknown header key "nam" in policies.sql`},
		{"-- name: a Post p\n", "1:10: policy a has no SQL"},
		{"-- name: a Post p\np.x = 1;\n-- name: b Post p\np.y", "2:8: ; in policies.sql"},
		{"-- name: a Post p\n{x} AND p.x", "2:1: { in a policy"},
		{"-- name: a Post p\np.x = ?", "2:7: placeholder ? in a policy"},
		{"-- name: a Post p\np.x = $1", "2:7: placeholder $1 in a policy"},
		{"-- name: a Post p\np.x = :y", "2:7: parameter y is not declared"},
		{"-- name: a Post p\n-- param: y int64\np.x", "2:11: parameter y is not used"},
		{"-- name: a Post p\n-- param: viewer_id int64\np.x = :viewer_id", "2:11: parameter viewer_id is the user's"},
		{"-- name: a Post p\n-- param: y? int64\np.x = :y", "2:11: a policy's parameter y is optional, a slice or choices"},
		{"-- name: a Post p\n-- param: y []int64\np.x IN (:y)", "2:11: a policy's parameter y is optional, a slice or choices"},
		{"-- name: a Post p\n-- param: y a/b\np.x = :y", "2:11: a policy's parameter y is optional, a slice or choices"},
		{"-- name: a Post p\n-- param: y int64\n-- param: y int64\np.x = :y", "3:11: parameter y is declared twice"},
		{"-- name: a Post p\n-- get: GetBoard\np.x", "2:8: -- get: names the GetX of the policy's model, GetPost"},
		{"-- name: a Post p\n-- get: GetPost\n-- get: GetPost\np.x", "3:4: second -- get: line in policy a"},
		{"-- name: a Post p\n-- param: id int64\np.x = :id", "2:11: parameter id is a name GetX takes"},
	} {
		_, ds := parsePolicies(file, c.src)
		got := strings.TrimSpace(strings.ReplaceAll(text(ds), file+":", ""))
		if i := strings.Index(got, "\n"); i >= 0 {
			got = got[:i]
		}
		if got != c.want {
			t.Errorf("%q:\n got %s\nwant %s", c.src, got, c.want)
		}
	}
}

func TestPolicyParts(t *testing.T) {
	ps, ds := parsePolicies("board/policies.sql", "-- name: v Post p\n-- param: now time.Time\nNOT p.hidden -- a comment\n  AND p.at <= :now OR p.author_id = :viewer_id")
	if ds != nil {
		t.Fatal(text(ds))
	}
	var b strings.Builder
	for _, part := range ps[0].expand(`"X"`, at(9, 1, 0)) {
		if part.Param != "" {
			b.WriteString(":" + part.Param)
			continue
		}
		b.WriteString(part.SQL)
	}
	// The comment goes, and the line break: a use may be in the middle of
	// a line.
	if want := `(NOT "X".hidden AND "X".at <= :now OR "X".author_id = :viewer_id)`; b.String() != want {
		t.Errorf("got  %s\nwant %s", b.String(), want)
	}
	if got := ps[0].Params; len(got) != 2 || got[1].Name != "viewer_id" || !got[1].Viewer {
		t.Errorf("params: %+v", got)
	}
}

// policyFixture is the app of testdata/policy and its policies.
func policyFixture(t *testing.T) ([]*model.App, Policies) {
	t.Helper()
	apps, _ := modeltest.Example(t, "testdata/policy")
	pols, ds := ReadPolicies("testdata/policy", []string{"board/policies.sql"})
	if ds != nil {
		t.Fatal(text(ds))
	}
	return apps, pols
}

func TestPolicyCheck(t *testing.T) {
	apps, pols := policyFixture(t)
	user := apps[0].Models[slices.IndexFunc(apps[0].Models, func(m *model.Model) bool { return m.Name == "User" })]
	viewer := Viewer{Expr: "User", Model: user}
	const head = "-- name: Q :many R\n"
	for _, c := range []struct{ dir, src, want string }{
		// What passes.
		{"board", "-- policy: out p\nSELECT p.id FROM board_post p WHERE p.board_id = 1 AND out(p)", ""},
		{"board", "-- policy: out board_post\nSELECT id FROM board_post WHERE out(board_post)", ""},
		{"board", "-- policy: visible p\n-- param: now time.Time\nSELECT p.id FROM board_post p WHERE visible(p) ORDER BY p.publish_at < :now", ""},
		{"board", "-- policy: visible p\n-- policy: none q every post\nSELECT p.id, (SELECT count(*) FROM board_post q WHERE q.board_id = p.board_id) FROM board_post p WHERE visible(p)", ""},
		{"board", "-- policy: none p the admin's export\nSELECT id FROM board_post p", ""},
		{"board", "-- policy: out p\nSELECT 1 FROM board_board WHERE {\n  out(p)\n}", "4:3: out(p), but this SELECT reads no table as p"},
		{"board", "-- policy: out p\nSELECT b.id FROM board_board b WHERE EXISTS (SELECT 1 FROM board_post p WHERE p.board_id = b.id AND out(p))", ""},
		{"board", "-- policy: out p\nWITH ps AS (SELECT p.id FROM board_post p WHERE out(p)) SELECT id FROM ps", ""},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p WHERE true AND (out(p) AND p.id > 1)", ""},
		{"board", "-- policy: out p\nSELECT 1 FROM board_board b LEFT JOIN board_post p ON p.board_id = b.id AND out(p)", ""},
		{"board", "-- policy: out p\n-- param: x bool\nSELECT 1 FROM board_board b WHERE true\n{x} AND EXISTS (SELECT 1 FROM board_post p WHERE out(p))", ""},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p WHERE {\n  out(p)\n  AND p.id > 1\n}", ""},
		{"board", "-- policy: out p\nSELECT 1 FROM board_board b JOIN board_post p ON out(p) WHERE true", ""},
		{"board", "-- policy: out p\nSELECT p.out, p.id AS out FROM board_post p WHERE out(p)", ""},
		// What a missing policy is.
		{"board", "SELECT id FROM board_post", "2:16: query reads board_post board_post without a policy (rule 6)"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p WHERE out(p) AND EXISTS (SELECT 1 FROM board_post p WHERE p.id = 1)", "3:67: query reads board_post p without a policy (rule 6)"},
		{"board", "-- policy: out p\nSELECT (SELECT count(*) FROM board_post p) FROM board_post p WHERE out(p)", "3:30: query reads board_post p without a policy (rule 6)"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p WHERE out(p) UNION SELECT 1 FROM board_post p", "3:61: query reads board_post p without a policy (rule 6)"},
		{"board", "-- policy: out p\nWITH ps AS (SELECT p.id FROM board_post p) SELECT id FROM board_post p WHERE out(p)", "3:30: query reads board_post p without a policy (rule 6)"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p WHERE out(p) AND EXISTS (SELECT 1 FROM board_post q WHERE out(p))", "3:67: query reads board_post q without a policy (rule 6)"},
		{"board", "-- policy: out s\nSELECT 1 FROM (SELECT * FROM board_post p) s WHERE out(s)", "3:30: query reads board_post p without a policy (rule 6)"},
		{"board", "-- policy: out s\n-- policy: none p every post\nSELECT 1 FROM (SELECT * FROM board_post p) s WHERE out(s)", "4:52: out(s), but this SELECT reads no table as s"},
		{"board", "SELECT 1 FROM board_post, (a JOIN b ON true)", "2:27: cannot tell which tables the query reads: a join in parentheses"},
		{"other", "SELECT 1 FROM board_post p", "2:15: query of the app at other reads board_post, the table of model Post, whose policies are another app's"},
		// A use in the wrong place.
		{"board", "-- policy: out b\nSELECT 1 FROM board_board b WHERE out(b)", "3:35: b reads board_board, but policy out is for Post"},
		{"board", "-- policy: out p\n-- param: x bool\nSELECT 1 FROM board_post p WHERE true\n{x} AND out(p)", "5:9: policy out on a line with a {condition}"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p WHERE p.id = 1 OR out(p)", "3:46: out(p) follows \"OR\", so it may not narrow the rows"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p WHERE out(p) OR p.id = 1", "3:34: out(p) is followed by \"OR\", so it may not narrow the rows"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p WHERE (p.id = 1 OR out(p))", "3:47: out(p) follows \"OR\", so it may not narrow the rows"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p WHERE (out(p) AND p.id = 1) OR true", "3:35: out(p) is followed by \"OR\", so it may not narrow the rows"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p WHERE NOT out(p)", "3:38: out(p) follows \"NOT\", so it may not narrow the rows"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p WHERE NOT (out(p))", "3:39: out(p) follows \"NOT\", so it may not narrow the rows"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p WHERE CASE WHEN out(p) THEN true ELSE true END", "3:44: out(p) is followed by \"THEN\", so it may not narrow the rows"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p WHERE COALESCE(out(p), true)", "3:43: out(p) is followed by \",\", so it may not narrow the rows"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p WHERE out(p) IS NOT FALSE", "3:34: out(p) is followed by \"IS\", so it may not narrow the rows"},
		{"board", "-- policy: out p\nSELECT out(p) FROM board_post p", "3:8: out(p) follows \"SELECT\", so it may not narrow the rows"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p GROUP BY p.id HAVING count(*) > 0 AND out(p)", "3:66: out(p) is in HAVING, so it may not narrow the rows"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p GROUP BY p.id HAVING out(p)", "3:49: out(p) follows \"HAVING\", so it may not narrow the rows"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p LEFT JOIN board_board b ON out(p)", "3:55: out(p) is in the ON of a JOIN that reads another table, which keeps the rows of p it does not match, so it may not narrow the rows"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_board b RIGHT JOIN board_post p ON out(p)", "3:56: out(p) is in the ON of a RIGHT JOIN, which keeps the rows it does not match, so it may not narrow the rows"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p WHERE {\n  p.id = 1\n  OR out(p)\n}", "5:6: out(p) follows \"OR\", so it may not narrow the rows"},
		// The header.
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p", "2:12: policy out is not used"},
		{"board", "-- policy: out p\nSELECT 1 FROM board_post p WHERE out(q)", "3:38: no -- policy: out q"},
		{"board", "-- policy: hidden p\nSELECT 1 FROM board_post p", `2:12: no policy hidden in policies.sql of the app`},
		{"board", "-- policy: none p\nSELECT 1 FROM board_post p", "2:18: -- policy: none with no reason"},
		{"board", "-- policy: none p why\n-- policy: out p\nSELECT 1", "3:16: alias p has a -- policy: line already"},
		{"board", "-- policy: none x because\nSELECT 1 FROM board_post p WHERE false", "2:12: -- policy: none x, but the query reads no table as x"},
		{"board", "-- policy: none b because\nSELECT 1 FROM board_board b", "2:12: -- policy: none b, but board_board has no policy"},
		{"board", "-- policy: none p because\nSELECT 1 FROM board_post p UNION SELECT 1 FROM board_comment p", "3:48: -- policy: none p, but p is board_comment here and board_post at line 3"},
		{"board", "-- policy: none p because\nSELECT 1 FROM board_post p UNION SELECT 1 FROM board_board p", "3:48: -- policy: none p, but p is board_board here and board_post at line 3"},
		{"board", "-- policy: none p because\nSELECT 1 FROM board_post p WHERE EXISTS (SELECT 1 FROM board_post p WHERE true)", ""},
		{"board", "-- policy: none b because\nSELECT 1 FROM board_post, (a JOIN b ON true)", "3:27: cannot tell which tables the query reads: a join in parentheses"},
		{"board", "-- policy: visible Post p\nSELECT 1", `2:20: alias "Post" is not lowercase snake_case`},
		{"board", "-- policy: out p\n-- param: now int64\nSELECT 1 FROM board_post p WHERE out(p)", "3:11: parameter now is not the time.Time of policy out"},
		{"board", "-- policy: visible p\n-- param: viewer_id int64\nSELECT 1 FROM board_post p WHERE visible(p) AND :viewer_id = 1", "3:11: parameter viewer_id is the user's column in policy visible"},
		{"board", "-- policy: visible p\nSELECT 1 FROM board_post p WHERE visible(p) AND p.id = :viewer_id", "3:56: parameter viewer_id is not declared"},
	} {
		q, ds := parse("board/queries/q.sql", head+c.src)
		if ds == nil {
			if pds, ok := q.findPolicies(pols["board"]); !ok {
				ds = pds
			} else {
				ds = q.body()
			}
		}
		if ds == nil {
			ds = CheckPolicies(apps, pols, []*App{{Dir: c.dir, Queries: []*Query{q}}}, viewer)
		}
		got := strings.TrimSpace(strings.ReplaceAll(text(ds), "board/queries/q.sql:", ""))
		if i := strings.Index(got, "\n"); i >= 0 {
			got = got[:i]
		}
		if got != c.want {
			t.Errorf("%q:\n got %s\nwant %s", c.src, got, c.want)
		}
	}
}

func TestPolicyCheckPolicies(t *testing.T) {
	apps, _ := policyFixture(t)
	user := apps[0].Models[slices.IndexFunc(apps[0].Models, func(m *model.Model) bool { return m.Name == "User" })]
	viewer := Viewer{Expr: "User", Model: user}
	const file = "board/policies.sql"
	for _, c := range []struct {
		src    string
		viewer Viewer
		want   string
	}{
		{"-- name: a Post p\n-- get: GetPost\np.x = :viewer_id AND :viewer_team_id = 1 AND :viewer_joined_at < now()", viewer, ""},
		{"-- name: a Post p\np.x", viewer, "1:10: model Post has policies, but none says -- get: GetPost"},
		{"-- name: a Post p\n-- get: GetPost\np.x\n-- name: b Post p\n-- get: GetPost\np.y", viewer, "5:9: policies a and b both say -- get: GetPost"},
		{"-- name: a Post p\n-- get: GetPost\np.x\n-- name: b Post p\np.y", viewer, "4:10: policy b is used by no query"},
		{"-- name: a Pots p\np.x", viewer, `1:12: no model Pots in the app of this policies.sql`},
		{"-- name: a Post p\n-- get: GetPost\np.x = :viewer_idd", viewer, `3:7: User has no column idd`},
		{"-- name: a Post p\n-- get: GetPost\np.x", Viewer{}, "1:10: policies need the user's type, but no view takes the user"},
		{"-- name: a Post p\n-- get: GetPost\np.x = :viewer_id", Viewer{Expr: "Account"}, "3:7: the user type Account is not a model, so :viewer_id has no column"},
	} {
		ps, ds := parsePolicies(file, c.src)
		if ds == nil {
			ds = CheckPolicies(apps, Policies{"board": ps}, nil, c.viewer)
		}
		got := strings.TrimSpace(strings.ReplaceAll(text(ds), file+":", ""))
		if i := strings.Index(got, "\n"); i >= 0 {
			got = got[:i]
		}
		if got != c.want {
			t.Errorf("%q:\n got %s\nwant %s", c.src, got, c.want)
		}
	}
}

// A policy is expanded before the combinations: in WHERE { } its line is
// tidied like any other, and its parameters are numbered with the query's.
func TestPolicyVariants(t *testing.T) {
	_, pols := policyFixture(t)
	q, ds := parse("board/queries/q.sql", "-- name: Q :many R\n"+
		"-- param: q? string\n"+
		"-- param: board_id? int64\n"+
		"-- policy: visible p\n"+
		"SELECT p.id FROM board_post p\n"+
		"WHERE {\n"+
		"  {board_id} p.board_id = :board_id\n"+
		"  AND visible(p)\n"+
		"  {q} AND p.status = :q\n"+
		"}\n"+
		"ORDER BY p.publish_at < :now\n")
	if ds == nil {
		ds, _ = q.findPolicies(pols["board"])
	}
	if ds == nil {
		ds = q.body()
	}
	if ds != nil {
		t.Fatal(text(ds))
	}
	axes, vs, ds := q.Variants()
	if ds != nil {
		t.Fatal(text(ds))
	}
	golden.Check(t, "testdata/policy_variants.golden", []byte(render(axes, vs, project.Postgres)))
	// The policy's SQL is where policies.sql has it.
	i := slices.IndexFunc(vs[0].Pieces, func(p Piece) bool { return p.SQL == "author_id" })
	if p := vs[0].Pieces[i]; p.Pos.Filename != "board/policies.sql" || p.Pos.Line != 12 || p.Pos.Column != 8 {
		t.Errorf("author_id at %v", p.Pos)
	}
}

// Without a policy in the project nothing is checked: not even the shapes
// tables cannot read.
func TestPolicyCheckWithoutPolicies(t *testing.T) {
	apps, _ := policyFixture(t)
	q, ds := parse("board/queries/q.sql", "-- name: Q :many R\nSELECT 1 FROM board_post, (a JOIN b ON true)")
	if ds == nil {
		ds = q.body()
	}
	if ds == nil {
		ds = CheckPolicies(apps, Policies{}, []*App{{Dir: "board", Queries: []*Query{q}}}, Viewer{})
	}
	if ds != nil {
		t.Error(text(ds))
	}
}
