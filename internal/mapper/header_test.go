package mapper

import (
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/golden"
	"github.com/maago-web/maago/internal/model"
)

const file = "blog/queries/q.sql"

func text(ds []diag.Diagnostic) string {
	var b strings.Builder
	diag.Write(&b, ds)
	return b.String()
}

func at(line, col, off int) token.Position {
	return token.Position{Filename: file, Line: line, Column: col, Offset: off}
}

func TestParse(t *testing.T) {
	for _, c := range []struct {
		src  string
		want Query // File and Src are filled in
	}{
		{
			src: "-- name: SearchOrders :many OrderSummary\n" +
				"-- Search orders, newest first.\n" +
				"\n" +
				"--param: status?    string\n" +
				"-- param: ids []int64\n" +
				"-- param:\tsort latest/amount\n" +
				"  -- column: n int64\n" +
				"-- column: author_name? string\n" +
				"-- param: data? []byte\n" +
				"-- param: blobs [][]byte\n" +
				"-- column: flag? bool\n" +
				"SELECT 1\n",
			want: Query{
				HeaderEnd: 274,
				Name:      "SearchOrders", NamePos: at(1, 10, 9), Kind: Many, Result: "OrderSummary",
				Params: []Param{
					{"status", at(4, 10, 83), true, Type{Base: model.String}},
					{"ids", at(5, 11, 111), false, Type{Base: model.Int64, Slice: true}},
					{"sort", at(6, 11, 133), false, Type{Choices: []string{"latest", "amount"}}},
					{"data", at(9, 11, 214), true, Type{Base: model.Bytes}},
					{"blobs", at(10, 11, 237), false, Type{Base: model.Bytes, Slice: true}},
				},
				Columns: []Column{
					{"n", at(7, 14, 165), false, model.Int64},
					{"author_name", at(8, 12, 184), true, model.String},
					{"flag", at(11, 12, 263), true, model.Bool}, // only a bool param cannot be optional
				},
			},
		},
		{
			src:  "-- name: Get :one Post\r\n-- param: id int64\r\nSELECT *\r\n",
			want: Query{HeaderEnd: 44, Name: "Get", NamePos: at(1, 10, 9), Kind: One, Result: "Post", Params: []Param{{"id", at(2, 11, 34), false, Type{Base: model.Int64}}}},
		},
		{
			src:  "-- name: Touch :exec\nUPDATE t SET x = 1",
			want: Query{HeaderEnd: 21, Name: "Touch", NamePos: at(1, 10, 9), Kind: Exec},
		},
		{
			src:  "-- name: Empty :exec",
			want: Query{HeaderEnd: 20, Name: "Empty", NamePos: at(1, 10, 9), Kind: Exec},
		},
	} {
		got, ds := parse(file, c.src)
		if ds != nil {
			t.Errorf("%q: unexpected errors:\n%s", c.src, text(ds))
			continue
		}
		c.want.File, c.want.Src = file, c.src
		if !reflect.DeepEqual(*got, c.want) {
			t.Errorf("%q:\ngot  %+v\nwant %+v", c.src, *got, c.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	const name = "-- name: Q :many R\n"
	var all string
	for _, c := range []struct{ src, want string }{
		{"", `1:1: query has no -- name: line`},
		{"SELECT 1", `1:1: query has no -- name: line`},
		{"/* x */\n-- name: Q :exec\n", `1:1: query has no -- name: line`},
		{"-- name:\n", `1:9: -- name: line has no query name`},
		{"-- name: searchOrders :one R\n", `1:10: query name "searchOrders" is not an exported Go name`},
		{"-- name: 한글 :one R\n", `1:10: query name "한글" is not an exported Go name`},
		{"-- name: Q\n", `1:11: query Q has no kind`},
		{"-- name: Q :mnay R\n", `1:12: unknown query kind ":mnay"` + "\n  fix: did you mean \":many\"?"},
		{"-- name: Q many R\n", `1:12: query kind "many" has no colon` + "\n  fix: write :many"},
		{"-- name: Q :exec R\n", `1:18: :exec returns no rows, so it has no result type`},
		{"-- name: Q :many\n", `1:17: query Q :many has no result type`},
		{"-- name: Q :one r\n", `1:17: result type "r" is not an exported Go name`},
		{"-- name: Q :one R S\n", `1:19: unexpected "S" after the result type`},
		{"-- name : Q :one R\n", `1:8: space between name and its colon` + "\n  fix: write -- name: with no space before the colon"},
		{name + "-- name: P :exec\n", `2:4: second -- name: line`},
		{name + "-- parm: x int64\n", `2:4: unknown header key "parm"` + "\n  fix: did you mean \"param\"?"},
		{name + "-- Note: see below\n", `2:4: unknown header key "Note"`},
		{name + "-- https://example.com\n", `2:4: unknown header key "https"`},
		{name + "-- maago: allow-drop\n", `2:4: unknown header key "maago"`},
		{name + "-- param:\n", `2:10: -- param: line has no name`},
		{name + "-- param: Status string\n", `2:11: param name "Status" is not lowercase snake_case`},
		{name + "-- param: x\n", `2:12: param x has no type`},
		{name + "-- param: ? int64\n", `2:11: -- param: line has no name`},
		{name + "-- param: x ? int64\n", `2:13: space between x and ?`},
		{name + "-- param: x int64 y\n", `2:19: unexpected "y" after the type of x`},
		{name + "-- param: x int\n", `2:13: unknown type "int"` + "\n  fix: did you mean \"int64\"?"},
		{name + "-- param: x []int\n", `2:13: unknown type "[]int"` + "\n  fix: did you mean \"[]int64\"?"},
		{name + "-- param: x *int64\n", `2:13: pointer type *int64 in a header` + "\n  fix: write x? int64 instead"},
		{name + "-- param: x? bool\n", `2:11: a bool cannot be optional`},
		{name + "-- param: x? []int64\n", `2:11: a slice cannot be optional`},
		{name + "-- param: x? a/b\n", `2:11: choices cannot be optional`},
		{name + "-- param: x latest\n", `2:13: unknown type "latest"`},
		{name + "-- param: x a//b\n", `2:15: choice "" is not lowercase snake_case`},
		{name + "-- param: x a/B\n", `2:15: choice "B" is not lowercase snake_case`},
		{name + "-- param: x a/\n", `2:15: choice "" is not lowercase snake_case`},
		{name + "-- param: x a/b/a\n", `2:17: choice a appears twice`},
		{name + "-- param: x int64\n-- param: x string\n", `3:11: parameter x is declared twice`},
		{name + "-- column: n int64\n-- column: n int64\n", `3:12: column n is declared twice`},
		{name + "-- column: n []int64\n", `2:14: unknown type "[]int64"`},
		{name + "-- column: n a/b\n", `2:14: unknown type "a/b"`},
		{"\t -- name: Q :mnay R\n", `1:14: unknown query kind ":mnay"`},
	} {
		q, ds := parse(file, c.src)
		out := text(ds)
		if q != nil || len(ds) != 1 || !strings.HasPrefix(out, file+":"+c.want) {
			t.Errorf("%q:\ngot  %s\nwant one error starting %s:%s", c.src, out, file, c.want)
		}
		all += fmt.Sprintf("%q\n%s\n", c.src, out)
	}
	// The fixes are what an agent follows, so all of them are checked.
	golden.Check(t, "testdata/errors.golden", []byte(all))
}

// Every line is read, so independent errors are all reported.
func TestParseSeveralErrors(t *testing.T) {
	q, ds := parse(file, "-- name: Q :mnay R\n-- param: x int\n-- param: y bool\n")
	want := file + `:1:12: unknown query kind ":mnay"` + "\n" +
		`  fix: did you mean ":many"? allowed: :one, :many, :exec` + "\n" +
		file + `:2:13: unknown type "int"` + "\n" +
		`  fix: did you mean "int64"? allowed: int64, int32, bool, float64, string, time.Time, []byte; or a slice of one, like []int64, or choices, like latest/amount` + "\n"
	if q != nil || text(ds) != want {
		t.Errorf("got %v\n%s\nwant nil\n%s", q, text(ds), want)
	}
}

func TestRead(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"blog/queries/a.sql": "-- name: Q :exec\nDELETE FROM t WHERE false\n",
		"blog/queries/b.sql": "-- name: Q :exec\nDELETE FROM t WHERE false\n",
		"blog/queries/c.sql": "-- name: Q :exec\nDELETE FROM t WHERE false\n",
		"blog/queries/d.sql": "-- name: R :exec\nDELETE FROM t WHERE false\n",
		"blog/queries/e.sql": "-- name: bad :exec\nDELETE FROM t WHERE false\n",
		"shop/queries/a.sql": "-- name: Q :exec\nDELETE FROM t WHERE false\n",
		"none/queries/a.sql": "-- name: bad :exec\nDELETE FROM t WHERE false\n",
		"queries/x.sql":      "-- name: X :exec\nDELETE FROM t WHERE false\n",
	}
	var list []string
	for p, src := range files {
		f := filepath.Join(root, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(f), 0o755)
		if err := os.WriteFile(f, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		list = append(list, p)
	}

	list = append(list, "gone/queries/a.sql") // cannot be read; the rest still is

	apps, ds := Read(root, list)

	want := "blog/queries/b.sql:1:10: query name Q is also the name of blog/queries/a.sql\n" +
		"  fix: rename one of them; each query becomes a Go function of the package\n" +
		"blog/queries/c.sql:1:10: query name Q is also the name of blog/queries/a.sql\n" +
		"  fix: rename one of them; each query becomes a Go function of the package\n" +
		"blog/queries/e.sql:1:10: query name \"bad\" is not an exported Go name\n" +
		"  fix: start it with an uppercase ASCII letter, followed by letters, digits or _\n" +
		"gone/queries/a.sql: cannot read file: open " + filepath.Join(root, "gone", "queries", "a.sql") + ": no such file or directory\n" +
		"  fix: make sure the file can be read\n" +
		"none/queries/a.sql:1:10: query name \"bad\" is not an exported Go name\n" +
		"  fix: start it with an uppercase ASCII letter, followed by letters, digits or _\n"
	if got := text(ds); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	var got []string
	if len(apps) != 3 {
		t.Errorf("got %d apps, want 3: none/ and gone/ have no query", len(apps))
	}
	for _, a := range apps {
		for _, q := range a.Queries {
			got = append(got, a.Dir+" "+q.File+" "+q.Name)
		}
	}
	wantQ := []string{"blog blog/queries/a.sql Q", "blog blog/queries/d.sql R", ". queries/x.sql X", "shop shop/queries/a.sql Q"}
	if strings.Join(got, "\n") != strings.Join(wantQ, "\n") {
		t.Errorf("queries:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(wantQ, "\n"))
	}
}
