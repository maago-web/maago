package main

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/golden"
	"github.com/maago-web/maago/internal/mapper"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/practicedb"
	"github.com/maago-web/maago/internal/project"
)

// describeQueries reads the query files of app blog and describes each on
// db, all in one practice database, as a run of maago check does.
func describeQueries(t *testing.T, db *practicedb.DB, files map[string]string) ([][]practicedb.Column, []diag.Diagnostic) {
	t.Helper()
	root := t.TempDir()
	var list []string
	if err := os.MkdirAll(filepath.Join(root, "blog", "queries"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range files {
		p := "blog/queries/" + name + ".sql"
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(p)), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		list = append(list, p)
	}
	sort.Strings(list)
	apps, ds := mapper.Read(root, list, nil)
	if ds != nil {
		t.Fatalf("read:\n%s", text(ds))
	}
	var all []diag.Diagnostic
	var cols [][]practicedb.Column
	for _, q := range apps[0].Queries {
		axes, vs, ds := q.Variants()
		if ds != nil {
			t.Fatalf("%s:\n%s", q.File, text(ds))
		}
		c, ds, fatal := mapper.Describe(ctx(t), db, q, axes, vs)
		if fatal {
			t.Fatalf("%s: the connection failed:\n%s", q.File, text(ds))
		}
		all = append(all, ds...)
		cols = append(cols, c)
	}
	return cols, all
}

// Every query's errors come out of one run, each at its place and with
// the combination that shows it.
func TestDescribeQueryErrors(t *testing.T) {
	files := map[string]string{
		"missing": "-- name: Missing :many R\nSELECT id, nope FROM posts\n",
		"syntax":  "-- name: Syntax :many R\nSELEC id FROM posts\n",
		"one_combo": "-- name: OneCombo :many R\n-- param: flag bool\nSELECT id FROM posts\nWHERE {\n" +
			"  {flag} AND nope = 1\n  AND n > 0\n}\n",
		"same_error": "-- name: SameError :many R\n-- param: a bool\n-- param: b bool\n" +
			"SELECT id, nope FROM posts\n{a} WHERE n > 0\n{b} ORDER BY id\n",
		// Only the SQL error: whether columns differ is not asked of a
		// query that does not prepare.
		"differ_and_fail": "-- name: DifferAndFail :many R\n-- param: sort latest/title/bad\nSELECT id,\n" +
			"  {sort=latest} n\n  {sort=title}  title\n  {sort=bad}    nope\nFROM posts\n",
		// A slice where a scalar goes fails only with two elements.
		"slice_as_scalar": "-- name: SliceAsScalar :many R\n-- param: ids []int64\nSELECT id FROM posts WHERE id = :ids\n",
		// Same name, another source: the columns differ.
		"other_source": "-- name: OtherSource :many R\n-- param: sort latest/title\nSELECT\n" +
			"  {sort=latest} p.id AS x\n  {sort=title}  b.id AS x\nFROM posts p JOIN boards b ON b.id = p.board_id\n",
		"choice": "-- name: Choice :many R\n-- param: sort latest/title\nSELECT id FROM posts WHERE n = :sort\n",
		"korean": "-- name: Korean :many R\nSELECT '한글', nope FROM posts\n",
		"differ": "-- name: Differ :many R\n-- param: sort latest/title\nSELECT id,\n" +
			"  {sort=latest} n\n  {sort=title}  title\nFROM posts\n",
		"returning": "-- name: Returning :exec\n-- param: title string\n" +
			"INSERT INTO posts (board_id, title) VALUES (1, :title) RETURNING id\n",
		// Parameter types: checked for PostgreSQL only.
		"varchar": "-- name: Varchar :many R\n-- param: title int64\nSELECT id FROM posts WHERE title = :title\n",
		"int32":   "-- name: Int32 :many R\n-- param: n int64\nSELECT id FROM posts WHERE n = :n\n",
		"slice":   "-- name: Slice :many R\n-- param: ids []int32\nSELECT id FROM posts WHERE id IN (:ids)\n",
	}
	// In PostgreSQL a wrong parameter type leaves the columns good: they
	// come with the error, so the result type is checked in the same run.
	// SQLite does not check parameter types, so these give no error there.
	const withColumns = "choice int32 slice varchar"
	for _, d := range []project.Dialect{project.SQLite, project.Postgres} {
		t.Run(string(d), func(t *testing.T) {
			cols, ds := describeQueries(t, describeDB(t, d), files)
			golden.Check(t, "testdata/describe_errors."+string(d)+".golden", []byte(text(ds)))
			var got []string
			for i, name := range slices.Sorted(maps.Keys(files)) {
				if cols[i] != nil {
					got = append(got, name)
				}
			}
			if s := strings.Join(got, " "); s != withColumns {
				t.Errorf("queries with columns: %s, want %s", s, withColumns)
			}
		})
	}
}

// A failed connection ends the description at once, and says so.
func TestDescribeFatal(t *testing.T) {
	db := describeDB(t, project.Postgres)
	q := "-- name: Q :many R\nSELECT id FROM posts\n"
	db.Close()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "blog", "queries", "q.sql"), q)
	apps, ds := mapper.Read(root, []string{"blog/queries/q.sql"}, nil)
	if ds != nil {
		t.Fatal(text(ds))
	}
	axes, vs, _ := apps[0].Queries[0].Variants()
	if cols, ds, fatal := mapper.Describe(ctx(t), db, apps[0].Queries[0], axes, vs); !fatal || cols != nil || len(ds) != 1 {
		t.Errorf("fatal %v, cols %v:\n%s", fatal, cols, text(ds))
	}
}

func TestDescribeQueries(t *testing.T) {
	search, err := os.ReadFile("../../internal/testdata/orders/queries/search_orders.sql")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"search_orders": string(search),
		"touch":         "-- name: Touch :exec\n-- param: id int64\nUPDATE posts SET n = n + 1 WHERE id = :id\n",
		"insert":        "-- name: Insert :one R\n-- param: title string\nINSERT INTO posts (board_id, title) VALUES (1, :title) RETURNING id\n",
	}
	for _, d := range []project.Dialect{project.SQLite, project.Postgres} {
		t.Run(string(d), func(t *testing.T) {
			db := describeDB(t, d)
			if d == project.SQLite {
				mustExec(t, db, "CREATE TABLE orders (id INTEGER PRIMARY KEY, status TEXT NOT NULL, amount INTEGER NOT NULL, created_at TEXT NOT NULL)")
			} else {
				mustExec(t, db, "CREATE TABLE orders (id bigint PRIMARY KEY, status varchar(20) NOT NULL, amount bigint NOT NULL, created_at timestamptz NOT NULL)")
			}
			cols, ds := describeQueries(t, db, files)
			if ds != nil {
				t.Fatal(text(ds))
			}
			var got []string
			for _, cs := range cols {
				var names []string
				for _, c := range cs {
					names = append(names, c.Name+"="+c.Table+"."+c.Origin)
				}
				got = append(got, strings.Join(names, " "))
			}
			// insert, search_orders, touch: in file order
			want := "id=posts.id | id=orders.id status=orders.status amount=orders.amount created_at=orders.created_at | "
			if s := strings.Join(got, " | "); s != want {
				t.Errorf("got  %s\nwant %s", s, want)
			}
		})
	}
}

// A policy is prepared alone: what the database finds wrong in it is at
// its place in policies.sql, a :viewer_ column's type too.
func TestDescribePolicyErrors(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "blog", "policies.sql"), "-- name: typo Post p\n-- get: GetPost\np.titel = 'x'\n\n"+
		"-- name: untyped Post p\np.id > 0 OR :viewer_id IS NULL\n\n"+
		"-- name: typed Post p\n-- param: n string\np.n = :n AND p.title = :viewer_id\n")
	pols, ds := mapper.ReadPolicies(root, []string{"blog/policies.sql"})
	if ds != nil {
		t.Fatal(text(ds))
	}
	field := func(name, col string, typ model.Type) *model.Field {
		return &model.Field{Name: name, Column: col, Type: typ}
	}
	post := &model.Model{Name: "Post", Table: "posts", Fields: []*model.Field{field("ID", "id", model.Int64)}}
	user := &model.Model{Name: "User", Table: "users", Fields: []*model.Field{field("ID", "id", model.Int64)}}
	mapper.CheckPolicies([]*model.App{{Dir: "blog", Models: []*model.Model{post, user}}}, pols, nil, mapper.Viewer{Expr: "User", Model: user}) // finds the model; "used by no query" is beside the point
	for _, d := range []project.Dialect{project.SQLite, project.Postgres} {
		t.Run(string(d), func(t *testing.T) {
			db := describeDB(t, d)
			var all []diag.Diagnostic
			for _, p := range pols["blog"] {
				q, v := p.Probe()
				_, ds, fatal := mapper.Describe(ctx(t), db, q, nil, []mapper.Variant{v})
				if fatal {
					t.Fatal(text(ds))
				}
				all = append(all, ds...)
			}
			golden.Check(t, "testdata/describe_policy_errors."+string(d)+".golden", []byte(text(all)))
		})
	}
}
