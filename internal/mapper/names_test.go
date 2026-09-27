package mapper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/golden"
	"github.com/maago-web/maago/internal/model/modeltest"
	"github.com/maago-web/maago/internal/pipeline"
	"github.com/maago-web/maago/internal/project"
)

// Each way a generated name is declared twice, and names that do not
// count: a generated file's and an external test's.
func TestNames(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/app\n",
		"orders/models.go": "package orders\n\nimport \"github.com/maago-web/maago\"\n\n" +
			"type Order struct {\n\tmaago.Model\n\tID int64\n}\n",
		"orders/extra.go":              "package orders\n\nfunc Bar() {}\n\nvar fmt = 1\n\nfunc (Order) Baz() {}\n",
		"orders/extra_test.go":         "package orders\n\nfunc scanOrder() {}\n\ntype NewOrderInput struct{}\n",
		"orders/_scratch.go":           "package orders\n\nfunc DeleteOrder() {}\n",
		"orders/x_test.go":             "package orders_test\n\nfunc Baz() {}\n",
		"orders/old_gen.go":            pipeline.Marker + "\n\npackage orders\n\nfunc Qux() {}\n",
		"orders/queries/get_order.sql": "-- name: GetOrder :exec\nDELETE FROM orders\nWHERE id = 1\n",
		"orders/queries/foo.sql":       "-- name: Foo :many Foo\nSELECT id FROM orders\n",
		"orders/queries/list.sql": "-- name: List :many R\n-- param: sort_by a/b\nSELECT id FROM orders ORDER BY\n" +
			"  {sort_by=a} id\n  {sort_by=b} id DESC\n",
		"orders/queries/list_sort.sql": "-- name: ListSort :many R\n-- param: by a/b\nSELECT id FROM orders ORDER BY\n" +
			"  {by=a} id\n  {by=b} id DESC\n",
		"orders/queries/bar.sql": "-- name: Bar :exec\nDELETE FROM orders\nWHERE id = 1\n",
		"orders/queries/baz.sql": "-- name: Baz :exec\nDELETE FROM orders\nWHERE id = 2\n",
		"orders/queries/qux.sql": "-- name: Qux :exec\nDELETE FROM orders\nWHERE id = 3\n",
	}
	for p, src := range files {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	apps, fset := modeltest.Example(t, root)
	qfiles, _ := project.QueryFiles(root)
	qapps, ds := Read(root, qfiles)
	if ds != nil {
		t.Fatal(ds)
	}
	var qs []Checked
	r := &Result{Name: "R"} // one type for both, as Results gives
	for _, q := range qapps[0].Queries {
		c := Checked{Query: q}
		switch q.Name {
		case "Foo":
			c.Result = &Result{Name: "Foo", Pos: q.NamePos}
		case "List", "ListSort":
			c.Result = r
		}
		qs = append(qs, c)
	}
	var b strings.Builder
	diag.Write(&b, Names(fset, root, "orders", apps[0], qs))
	golden.Check(t, "testdata/names.golden", []byte(b.String()))
}
