package mapper

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/golden"
	"github.com/maago-web/maago/internal/model/modeltest"
	"github.com/maago-web/maago/internal/practicedb"
)

func TestJoins(t *testing.T) {
	for _, c := range []struct{ body, want string }{
		{"SELECT 1 FROM posts p LEFT JOIN boards b ON b.id = p.board_id", "null=[boards]"},
		{"SELECT 1 FROM posts p LEFT OUTER JOIN ONLY s.\"Boards\" AS x ON true", "null=[boards]"},
		{"SELECT 1 FROM posts p RIGHT JOIN boards b ON true", "all"},
		{"SELECT 1 FROM posts p FULL OUTER JOIN boards b ON true", "all"},
		{"WITH x AS (SELECT b.name FROM posts p LEFT JOIN boards b ON true) SELECT name FROM x", "null=[boards]"},
		{"SELECT 'LEFT JOIN boards' FROM posts /* LEFT JOIN boards */", "null=[]"},
		{"SELECT title FROM posts UNION SELECT name FROM boards", "null=[] noSource"},
		{"SELECT 1 FROM (SELECT id FROM posts p JOIN boards b ON true) s", "null=[]"},
		{"SELECT 1 FROM posts p JOIN LATERAL (SELECT 1) x ON true", "null=[]"},
		{"SELECT 1 FROM posts NATURAL JOIN boards", "21: cannot tell which result columns may be NULL: a NATURAL join (D-104)"},
		{"SELECT 1 FROM posts p LEFT JOIN (SELECT 1) x ON true", "33: cannot tell which result columns may be NULL: a LEFT JOIN of a subquery (D-104)"},
		{"SELECT 1 FROM posts p LEFT JOIN LATERAL (SELECT 1) x ON true", "33: cannot tell which result columns may be NULL: a LEFT JOIN LATERAL (D-104)"},
		{"WITH s AS MATERIALIZED (SELECT name FROM boards) SELECT s.name FROM posts p LEFT JOIN s ON true", "87: cannot tell which result columns may be NULL: a LEFT JOIN of the CTE s, whose columns come from other tables (D-104)"},
		{"WITH RECURSIVE t AS (SELECT 1) SELECT 1 FROM posts p LEFT JOIN t ON true", "64: cannot tell which result columns may be NULL: a LEFT JOIN of the CTE t, whose columns come from other tables (D-104)"},
		{"WITH x(a) AS (SELECT 1), y AS (SELECT 2) SELECT 1 FROM posts p LEFT JOIN y ON true", "74: cannot tell which result columns may be NULL: a LEFT JOIN of the CTE y, whose columns come from other tables (D-104)"},
		{"SELECT 1 FROM (posts p LEFT JOIN boards b ON true)", "15: cannot tell which result columns may be NULL: a join in parentheses (D-104)"},
	} {
		q, out := readBody(t, "-- name: Q :many R\n"+c.body+"\n")
		if out != "" {
			t.Fatalf("%s: %s", c.body, out)
		}
		j, ds := q.joins()
		got := strings.TrimPrefix(text(ds), file+":2:")
		if ds == nil {
			var null []string
			for n := range j.null {
				null = append(null, n)
			}
			slices.Sort(null)
			got = fmt.Sprintf("null=%v", null)
			if j.all {
				got = "all"
			}
			if j.noSource {
				got += " noSource"
			}
		}
		if first, _, _ := strings.Cut(got, "\n"); first != c.want {
			t.Errorf("%s:\ngot  %s\nwant %s", c.body, got, c.want)
		}
	}
}

// col is a result column as Describe gives it.
func col(name, source string, oid uint32) practicedb.Column {
	table, origin, _ := strings.Cut(source, ".")
	return practicedb.Column{Name: name, Table: table, Origin: origin, Type: oid}
}

var postColumns = []practicedb.Column{
	col("id", "posts.id", 0), col("board_id", "posts.board_id", 0), col("title", "posts.title", 0),
	col("author_name", "posts.author_name", 0), col("published_at", "posts.published_at", 0), col("url", "posts.url", 0),
}

func results(t *testing.T, srcs []string, cols [][]practicedb.Column) ([]*Result, string) {
	t.Helper()
	apps, _ := modeltest.Example(t, "testdata/example")
	var qs []*Query
	for i, src := range srcs {
		q, ds := parse(fmt.Sprintf("blog/queries/q%d.sql", i+1), src)
		if ds == nil {
			ds = q.body()
		}
		if ds != nil {
			t.Fatalf("%s: %s", src, text(ds))
		}
		qs = append(qs, q)
	}
	// PostgreSQL's names of the OIDs the cases use
	names := map[uint32]string{20: "bigint", 1700: "numeric"}
	rs, ds := Results(apps, "blog", qs, cols, func(oid uint32) string { return names[oid] })
	return rs, text(ds)
}

func fields(r *Result) string {
	var fs []string
	for _, f := range r.Fields {
		fs = append(fs, f.Column+"→"+fieldString(f))
	}
	return strings.Join(fs, ", ")
}

func TestModelResult(t *testing.T) {
	const post = "-- name: Q :many Post\n"
	// The columns in another order fill the fields in that order.
	rs, errs := results(t, []string{post + "SELECT title, id, url, board_id, published_at, author_name FROM posts\n"},
		[][]practicedb.Column{{postColumns[2], postColumns[0], postColumns[5], postColumns[1], postColumns[4], postColumns[3]}})
	want := "title→Title string, id→ID int64, url→URL string, board_id→BoardID int64, published_at→PublishedAt *time.Time, author_name→AuthorName *string"
	if errs != "" || rs[0].Model == nil || rs[0].Model.Name != "Post" || fields(rs[0]) != want {
		t.Errorf("got %v %s\nwant model Post: %s", rs, errs, want)
	}
	var all string
	for _, c := range []struct {
		body string
		cols []practicedb.Column
	}{
		// Completion condition 3 of M3, in the words of TUTORIAL-FUTURE §6.
		{"SELECT id, board_id, title, author_name, url FROM posts", append(slices.Clone(postColumns[:4]), postColumns[5])},
		{"SELECT id, board_id, title, author_name, published_at, url, 1 AS n FROM posts",
			append(slices.Clone(postColumns), col("n", "", 0))},
		{"SELECT id, name FROM boards", []practicedb.Column{col("id", "boards.id", 0), col("name", "boards.name", 0)}},
		// Everything at once.
		{"SELECT p.id, b.id, COALESCE(title, '') AS title, p.author_name, p.author_name FROM posts p JOIN boards b ON true",
			[]practicedb.Column{col("id", "posts.id", 0), col("id", "boards.id", 0), col("title", "", 0),
				col("author_name", "posts.author_name", 0), col("author_name", "posts.author_name", 0)}},
		{"SELECT p.id, p.board_id, p.title, p.author_name, p.published_at, p.url FROM posts p RIGHT JOIN boards b ON true", postColumns},
		{"SELECT * FROM posts UNION SELECT * FROM posts", postColumns},
		{"SELECT p.id, p.board_id, p.title, p.author_name, p.published_at, p.url FROM boards b LEFT JOIN posts p ON true", postColumns},
	} {
		_, errs := results(t, []string{post + c.body + "\n"}, [][]practicedb.Column{c.cols})
		all += c.body + "\n" + errs + "\n"
	}
	_, errs = results(t, []string{post + "-- column: n int64\nSELECT 1\n"}, [][]practicedb.Column{postColumns})
	all += "-- column: with a model\n" + errs
	golden.Check(t, "testdata/model_result_errors.golden", []byte(all))
}

func TestNewResult(t *testing.T) {
	const head = "-- name: Q :many PostRow\n"
	for _, c := range []struct {
		body string
		cols []practicedb.Column
		want string
	}{
		{"-- column: n int64\nSELECT p.author_name, b.name AS board_name, p.board_id AS parent_id, count(*) AS n FROM posts p JOIN boards b ON true",
			[]practicedb.Column{col("author_name", "posts.author_name", 0), col("board_name", "boards.name", 0),
				col("parent_id", "posts.board_id", 0), col("n", "", 20)},
			"author_name→AuthorName *string, board_name→BoardName string, parent_id→ParentID int64, n→N int64"},
		{"-- column: name? string\nSELECT b.name FROM posts p LEFT JOIN boards b ON true",
			[]practicedb.Column{col("name", "boards.name", 0)}, "name→Name *string"},
		// The model's field name, unless the column is renamed.
		{"SELECT url, url AS link_url FROM posts",
			[]practicedb.Column{col("url", "posts.url", 0), col("link_url", "posts.url", 0)}, "url→URL string, link_url→LinkUrl string"},
	} {
		rs, errs := results(t, []string{head + c.body + "\n"}, [][]practicedb.Column{c.cols})
		if errs != "" || rs[0].Model != nil || fields(rs[0]) != c.want {
			t.Errorf("%s:\ngot  %v %s\nwant %s", c.body, fields(rs[0]), errs, c.want)
		}
	}
	var all string
	for _, c := range []struct {
		body string
		cols []practicedb.Column
	}{
		{"SELECT count(*) FROM posts", []practicedb.Column{col("count(*)", "", 0)}},
		{"SELECT p.id, b.id FROM posts p JOIN boards b ON true", []practicedb.Column{col("id", "posts.id", 0), col("id", "boards.id", 0)}},
		{"SELECT count(*) AS n FROM posts", []practicedb.Column{col("n", "", 0)}},
		{"SELECT b.name FROM posts p LEFT JOIN boards b ON true", []practicedb.Column{col("name", "boards.name", 0)}},
		{"SELECT name FROM hand_made", []practicedb.Column{col("name", "hand_made.name", 0)}},
		{"SELECT title FROM posts UNION SELECT name FROM boards", []practicedb.Column{col("title", "posts.title", 0)}},
		{"-- column: title string\nSELECT title FROM posts", []practicedb.Column{col("title", "posts.title", 0)}},
		{"-- column: nope int64\nSELECT title FROM posts", []practicedb.Column{col("title", "posts.title", 0)}},
		// PostgreSQL gives the type; SQLite (0) does not.
		{"-- column: n int32\nSELECT count(*) AS n FROM posts", []practicedb.Column{col("n", "", 20)}},
		{"-- column: n int64\nSELECT sum(id) AS n FROM posts", []practicedb.Column{col("n", "", 1700)}},
		{"-- column: n int32\nSELECT count(*) AS n FROM posts", []practicedb.Column{col("n", "", 0)}},
		{"-- column: n string\nSELECT sum(id) AS n FROM posts", []practicedb.Column{col("n", "", 1700)}},
		// The source's type is known, so a declaration must agree with it, in SQLite too.
		{"-- column: name? int64\nSELECT b.name FROM posts p LEFT JOIN boards b ON true", []practicedb.Column{col("name", "boards.name", 0)}},
		{"-- column: a_1_b int64\n-- column: a1_b int64\nSELECT 1 AS a_1_b, 2 AS a1_b", []practicedb.Column{col("a_1_b", "", 0), col("a1_b", "", 0)}},
	} {
		_, errs := results(t, []string{head + c.body + "\n"}, [][]practicedb.Column{c.cols})
		all += c.body + "\n" + errs + "\n"
	}
	golden.Check(t, "testdata/new_result_errors.golden", []byte(all))
}

// Queries naming the same new type share it, if they give it one shape.
func TestSharedResult(t *testing.T) {
	a := "-- name: A :many Row\nSELECT id, title FROM posts\n"
	b := "-- name: B :one Row\nSELECT id, title FROM posts WHERE id = 1\n"
	c := "-- name: C :many Row\nSELECT title, id FROM posts\n"
	cols := []practicedb.Column{col("id", "posts.id", 0), col("title", "posts.title", 0)}
	rs, errs := results(t, []string{a, b, c}, [][]practicedb.Column{cols, cols, {cols[1], cols[0]}})
	if rs[0] == nil || rs[0] != rs[1] || rs[2] != nil {
		t.Errorf("A and B should share one result, C have none: %v", rs)
	}
	want := "blog/queries/q3.sql:1:10: result type Row has another shape in blog/queries/q1.sql: field 1 is ID int64 there, Title string here\n" +
		"  fix: select the same columns in the same order as there, or give this result another name\n"
	if errs != want {
		t.Errorf("got\n%s\nwant\n%s", errs, want)
	}
	// In an app without models, a model's name is a new type.
	apps, _ := modeltest.Example(t, "testdata/example")
	q, _ := readBody(t, "-- name: Z :many Post\nSELECT id FROM posts\n")
	rs, ds := Results(apps, "other", []*Query{q}, [][]practicedb.Column{{col("id", "posts.id", 0)}}, nil)
	if ds != nil || rs[0].Model != nil || fields(rs[0]) != "id→ID int64" {
		t.Errorf("got %v %s, want a new type Post", rs[0], text(ds))
	}
	// A query Describe reported (no columns) is skipped, and an :exec has no result.
	rs, errs = results(t, []string{"-- name: X :many Post\nSELECT 1\n", "-- name: Y :exec\nDELETE FROM posts\n"}, [][]practicedb.Column{nil, nil})
	if errs != "" || rs[0] != nil || rs[1] != nil {
		t.Errorf("got %v %s, want nothing", rs, errs)
	}
}
