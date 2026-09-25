package postgres

import (
	"go/token"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect/sqlite"
	"github.com/maago-web/maago/internal/golden"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/model/modeltest"
)

// The golden file is checked by hand and applied to a real PostgreSQL with
// testdata/smoke.sh; this test keeps it from changing unnoticed.
func TestDDL(t *testing.T) {
	apps, fset := modeltest.Example(t, "../../testdata/example")
	ddl, ds := DDL(fset, apps)
	if len(ds) > 0 {
		t.Fatal(ds)
	}
	golden.Check(t, "testdata/example.sql", []byte(ddl))
}

// Names over 63 bytes are PostgreSQL's limit, not SQLite's (D-31).
func TestLongNames(t *testing.T) {
	fset := token.NewFileSet()
	f := fset.AddFile("blog/models.go", -1, 100)
	f.SetLines([]int{0, 10, 20, 30})
	long := strings.Repeat("t", 64)
	apps := []*model.App{{Dir: "blog", Models: []*model.Model{
		{Name: "Plain", Pos: f.LineStart(1), Table: strings.Repeat("p", 63)}, // fits; its _pkey does not
		{Name: "Long", Pos: f.LineStart(2), Table: long},                     // no constraints at all
		{Name: "Wide", Pos: f.LineStart(3), Table: "w", Fields: []*model.Field{{Name: "X", Column: long, Pos: f.LineStart(3)}}},
		{Name: "Clash", Pos: f.LineStart(3), Table: "a_b_idx"},
		{Name: "Other", Pos: f.LineStart(3), Table: "a", Indexes: [][]string{{"b"}}},
	}}}
	var b strings.Builder
	_, ds := DDL(fset, apps)
	diag.Write(&b, ds)
	out := b.String()
	for _, want := range []string{
		`blog/models.go:1:1: name "` + strings.Repeat("p", 63) + `_pkey" of Plain is longer than 63 bytes`,
		`blog/models.go:2:1: name "` + long + `" of Long is longer than 63 bytes`,
		`blog/models.go:3:1: column "` + long + `" of Wide.X is longer than 63 bytes`,
		`name "a_b_idx" of Clash is also the name of another table or index`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if _, ds := sqlite.DDL(fset, apps); len(ds) > 0 {
		t.Errorf("SQLite must accept these names: %v", ds)
	}
}
