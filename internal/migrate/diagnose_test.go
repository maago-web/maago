package migrate

import (
	"go/token"
	"testing"

	"github.com/maago-web/maago/internal/schema"
)

func TestDiagnose(t *testing.T) {
	from, to := post(), post()
	// A dropped field takes its check and index along; an added one
	// brings its own. A changed check on a kept column still shows, and the dropped
	// table has no place in the models.
	from.Columns = append(from.Columns, schema.Column{Name: "old", Type: "TEXT"})
	from.Indexes = append(from.Indexes, schema.Index{Name: "post_old_idx", Columns: []string{"old"}})
	from.Checks = append(from.Checks, schema.Check{Column: "old", Expr: "1"})
	to.Columns = append(to.Columns, schema.Column{Name: "editor_id", Type: "INTEGER"})
	to.FKs = append(to.FKs, schema.FK{Columns: []string{"editor_id"}, RefTable: "people", RefColumns: []string{"id"}, OnDelete: "NO ACTION", OnUpdate: "NO ACTION"})
	to.Checks[0].Expr = "2"
	gone := &schema.Table{Name: "gone", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}}
	cs := Diff(&schema.Schema{Tables: []*schema.Table{from, gone}}, &schema.Schema{Tables: []*schema.Table{to}})

	// The models have post and its fields other than old, each on its own line.
	lines := map[string]int{"": 10, "id": 11, "title": 12, "views": 13, "author_id": 14, "editor_id": 15}
	pos := func(table, column string) token.Position {
		if table != "post" {
			return token.Position{}
		}
		line, ok := lines[column]
		if !ok {
			line = lines[""]
		}
		return token.Position{Filename: "blog/models.go", Line: line, Column: 2}
	}
	want := `migrations do not match models: table "gone" is not in the models
  fix: run maago makemigrations
blog/models.go:10:2: migrations do not match models: column "old" of "post" is not in the models
  fix: run maago makemigrations
blog/models.go:12:2: migrations do not match models: check (2) of "post" is missing
  fix: run maago makemigrations
blog/models.go:12:2: migrations do not match models: check (LENGTH ("title") <= 200) of "post" is not in the models
  fix: run maago makemigrations
blog/models.go:15:2: migrations do not match models: column "editor_id" of "post" is missing
  fix: run maago makemigrations
`
	if got := text(Diagnose(cs, pos)); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
