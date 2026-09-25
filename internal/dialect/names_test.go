package dialect

import (
	"go/token"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/model"
)

func TestCheckNames(t *testing.T) {
	fset := token.NewFileSet()
	f := fset.AddFile("blog/models.go", -1, 100)
	f.SetLines([]int{0, 10, 20, 30, 40, 50})
	at := func(line int) token.Pos { return f.LineStart(line) }
	long := strings.Repeat("t", 60)
	apps := []*model.App{
		{Dir: "blog", Models: []*model.Model{
			// a_b + c and a + b_c both give x_a_b_c_key.
			{Name: "X", Pos: at(1), Table: "x", Uniques: [][]string{{"a_b", "c"}, {"a", "b_c"}}},
			// Index and FK names count too, and across apps.
			{Name: "Y", Pos: at(2), Table: "y", Indexes: [][]string{{"k"}}, Fields: []*model.Field{{Name: "PID", Column: "p_id", FK: "Y"}}},
			{Name: "Long", Pos: at(3), Table: long, Indexes: [][]string{{"c"}}},
		}},
		{Dir: "shop", Models: []*model.Model{
			{Name: "Z", Pos: at(4), Table: "y", Indexes: [][]string{{"k"}}, Fields: []*model.Field{{Name: "PID", Column: "p_id", FK: "Z"}}},
		}},
	}
	text := func(ds []diag.Diagnostic) string {
		var b strings.Builder
		diag.Write(&b, ds)
		return b.String()
	}

	out := text(CheckNames(fset, apps, 63))
	for _, want := range []string{
		`blog/models.go:1:1: constraint name "x_a_b_c_key" of X is also used by X`,
		`blog/models.go:3:1: constraint name "` + long + `_c_idx" of Long is longer than 63 bytes`,
		`blog/models.go:4:1: constraint name "y_k_idx" of Z is also used by Y`,
		`blog/models.go:4:1: constraint name "y_p_id_fkey" of Z is also used by Y`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if n := strings.Count(out, "\n  fix:"); n != 4 {
		t.Errorf("want 4 errors, got %d:\n%s", n, out)
	}
	if out := text(CheckNames(fset, apps, 0)); strings.Contains(out, "longer than") {
		t.Errorf("maxLen 0 must not limit length:\n%s", out)
	}
}
