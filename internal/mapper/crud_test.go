package mapper

import (
	"fmt"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/golden"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/model/modeltest"
	"github.com/maago-web/maago/internal/project"
)

func crudText(cs []CRUD) string {
	var b strings.Builder
	for _, c := range cs {
		fmt.Fprintf(&b, "%s\n", c.Model.Name)
		for _, s := range []struct {
			name string
			st   *Statement
		}{{"Get", &c.Get}, {"Insert", &c.Insert}, {"Update", c.Update}, {"Delete", &c.Delete}} {
			if s.st == nil {
				fmt.Fprintf(&b, "  %s: none\n", s.name)
				continue
			}
			fmt.Fprintf(&b, "  %s: %s\n    args %v\n", s.name, s.st.SQL, s.st.Args)
		}
	}
	return b.String()
}

// The shared example has FKs, NULL, defaults, a reserved word (order) and
// auto times.
func TestCRUDs(t *testing.T) {
	apps, _ := modeltest.Example(t, "../testdata/example")
	for _, d := range []project.Dialect{project.SQLite, project.Postgres} {
		golden.Check(t, "testdata/crud."+string(d)+".golden", []byte(crudText(CRUDs(apps[0], d))))
	}
}

// A model of nothing but ID is inserted with its defaults and has no update.
func TestCRUDOnlyID(t *testing.T) {
	m := &model.Model{Name: "Tick", Table: "ticks", Fields: []*model.Field{{Name: "ID", Column: "id", Type: model.Int64}}}
	got := crudText(CRUDs(&model.App{Models: []*model.Model{m}}, project.Postgres))
	want := "Tick\n" +
		"  Get: SELECT \"id\" FROM \"ticks\" WHERE \"id\" = $1\n    args [ID]\n" +
		"  Insert: INSERT INTO \"ticks\" DEFAULT VALUES RETURNING \"id\"\n    args []\n" +
		"  Update: none\n" +
		"  Delete: DELETE FROM \"ticks\" WHERE \"id\" = $1\n    args [ID]\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
