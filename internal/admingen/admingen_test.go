package admingen

import (
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/project"
)

// An ordering by the ID is not cut by the ID again, and an FK to a model
// of another app names that app.
func TestIDOrderAndOtherApp(t *testing.T) {
	item := &model.Model{Name: "Item", Table: "shop_item", Fields: []*model.Field{{Name: "ID", Column: "id", Type: model.Int64}}}
	note := &model.Model{Name: "Note", Table: "notes", Ordering: []model.Order{{Field: "ID"}}, Fields: []*model.Field{
		{Name: "ID", Column: "id", Type: model.Int64},
		{Name: "ItemID", Column: "item_id", Type: model.Int64, FK: "shop_item"},
	}}
	if got := Statements(note, project.SQLite, nil).List; !strings.HasSuffix(got, `ORDER BY "id" LIMIT ?1 OFFSET ?2`) {
		t.Errorf("list: %s", got)
	}
	apps := []*model.App{{Dir: "notes", Name: "notes", Models: []*model.Model{note}}, {Dir: "shop", Name: "shop", Models: []*model.Model{item}}}
	if got := string(File(project.SQLite, apps[0], apps)); !strings.Contains(got, `{Name: "ItemID", Label: "ItemID", FK: "shop.Item"}`) {
		t.Errorf("admin_gen.go:\n%s", got)
	}
}

// A field named Password without admin=hidden is in the admin, and its
// list (D-49, M6's condition 2: nothing is hidden by its name).
func TestPassword(t *testing.T) {
	user := &model.Model{Name: "User", Table: "auth_user", Fields: []*model.Field{
		{Name: "ID", Column: "id", Type: model.Int64},
		{Name: "Password", Column: "password", Type: model.String},
	}}
	app := &model.App{Dir: "auth", Name: "auth", Models: []*model.Model{user}}
	got := string(File(project.SQLite, app, []*model.App{app}))
	if !strings.Contains(got, `{Name: "Password", Label: "Password"}`) || !strings.Contains(got, `List: []string{"ID", "Password"}`) {
		t.Errorf("admin_gen.go:\n%s", got)
	}
}

// A field the form leaves out stops adding when it would be a zero value
// (D-132): hidden, read-only or bytes, unless it is the ID, auto, a
// pointer or has a default.
func TestBlocks(t *testing.T) {
	def := "x"
	for _, c := range []struct {
		f    model.Field
		want string
	}{
		{model.Field{Name: "Code", Type: model.String, Admin: "hidden"}, "M: no adding: Code is admin=hidden, with no default"},
		{model.Field{Name: "Slug", Type: model.String, Admin: "readonly"}, "M: no adding: Slug is admin=readonly, with no default"},
		{model.Field{Name: "Body", Type: model.Bytes}, "M: no adding: Body is bytes, which the admin does not edit, with no default"},
		{model.Field{Name: "Code", Type: model.String, Admin: "hidden", Pointer: true}, ""},
		{model.Field{Name: "Code", Type: model.String, Admin: "readonly", Default: &def}, ""},
		{model.Field{Name: "At", Type: model.Time, Admin: "readonly", Auto: "create"}, ""},
		{model.Field{Name: "Body", Type: model.Bytes, Pointer: true}, ""},
		{model.Field{Name: "Title", Type: model.String}, ""},
		{model.Field{Name: "Code", Type: model.String, Admin: "hidden"}, "M: no adding: Code is admin=hidden; Body is bytes, which the admin does not edit, with no default"},
	} {
		m := &model.Model{Name: "M", Fields: []*model.Field{{Name: "ID", Type: model.Int64, Admin: "readonly"}, &c.f}}
		if strings.Contains(c.want, "; Body") {
			m.Fields = append(m.Fields, &model.Field{Name: "Body", Type: model.Bytes})
		}
		if got := blocks(m); got != c.want {
			t.Errorf("%+v: %q, want %q", c.f, got, c.want)
		}
	}
}

// A model with nothing to change has no UPDATE, and its form only reads;
// a Password field without admin=hidden is in the form.
func TestNoEditAndPasswordForm(t *testing.T) {
	item := &model.Model{Name: "Item", Table: "shop_item", Fields: []*model.Field{{Name: "ID", Column: "id", Type: model.Int64}}}
	if st := Statements(item, project.SQLite, nil); st.Update != "" {
		t.Errorf("update: %q", st.Update)
	}
	app := &model.App{Dir: "shop", Name: "shop", Models: []*model.Model{item}}
	if got := string(File(project.SQLite, app, []*model.App{app})); strings.Contains(got, "Edit: true") || strings.Contains(got, "UPDATE") || !strings.Contains(got, "Add: true") ||
		!strings.Contains(got, "_, err := InsertItem(ctx, db, NewItemInput{})") {
		t.Errorf("admin_gen.go:\n%s", got)
	}
	user := &model.Model{Name: "User", Table: "auth_user", Fields: []*model.Field{
		{Name: "ID", Column: "id", Type: model.Int64},
		{Name: "Password", Column: "password", Type: model.String},
	}}
	app = &model.App{Dir: "auth", Name: "auth", Models: []*model.Model{user}}
	if got := string(File(project.SQLite, app, []*model.App{app})); !strings.Contains(got, `rt.BindString(ctx, vals, &f.Password, rt.Rule{Name: "password", Label: "Password"})`) {
		t.Errorf("admin_gen.go:\n%s", got)
	}
}

// A *bool is a select of yes and no, which can say NULL; its default is
// drawn as the choice.
func TestNullBool(t *testing.T) {
	yes := "true"
	m := &model.Model{Name: "Flag", Table: "flags", Fields: []*model.Field{
		{Name: "ID", Column: "id", Type: model.Int64},
		{Name: "On", Column: "on", Type: model.Bool, Pointer: true, Default: &yes},
	}}
	app := &model.App{Dir: "f", Name: "f", Models: []*model.Model{m}}
	got := string(File(project.SQLite, app, []*model.App{app}))
	for _, want := range []string{"On maago.Field[string]", `rt.BindString(ctx, vals, &f.On, rt.Rule{Name: "on", Label: "On", Choices: []string{"yes", "no"}, Optional: true})`,
		`f.On.Raw = "yes"`, "On: maago.Set(admin.Bool(f.On))", "admin.Bool(f.On), id"} {
		if !strings.Contains(got, want) {
			t.Errorf("admin_gen.go lacks %s:\n%s", want, got)
		}
	}
}

// A string whose default is ” may be left empty: that is its value.
func TestEmptyDefault(t *testing.T) {
	empty := ""
	m := &model.Model{Name: "Note", Table: "notes", Fields: []*model.Field{
		{Name: "ID", Column: "id", Type: model.Int64},
		{Name: "Nick", Column: "nick", Type: model.String, Default: &empty},
	}}
	app := &model.App{Dir: "n", Name: "n", Models: []*model.Model{m}}
	got := string(File(project.SQLite, app, []*model.App{app}))
	if !strings.Contains(got, `rt.Rule{Name: "nick", Label: "Nick", Optional: true}`) || !strings.Contains(got, `admin.In(&f.Nick, false, "")`) {
		t.Errorf("admin_gen.go:\n%s", got)
	}
}

// The rows that refer to a model: from any app, and from the same table
// leaving the row itself out (D-135).
func TestRefs(t *testing.T) {
	item := &model.Model{Name: "Item", Table: "shop_item", Fields: []*model.Field{
		{Name: "ID", Column: "id", Type: model.Int64},
		{Name: "ParentID", Column: "parent_id", Type: model.Int64, Pointer: true, FK: "shop_item"},
	}}
	note := &model.Model{Name: "Note", Table: "notes", AdminHidden: true, Fields: []*model.Field{
		{Name: "ID", Column: "id", Type: model.Int64},
		{Name: "ItemID", Column: "item_id", Type: model.Int64, FK: "shop_item"},
	}}
	apps := []*model.App{{Dir: "notes", Name: "notes", Models: []*model.Model{note}}, {Dir: "shop", Name: "shop", Models: []*model.Model{item}}}
	got := Statements(item, project.Postgres, apps).Refs
	want := []Ref{
		{Model: "notes.Note", Field: "ItemID", SQL: `SELECT COUNT(*) FROM "notes" WHERE "item_id" = $1`},
		{Model: "shop.Item", Field: "ParentID", SQL: `SELECT COUNT(*) FROM "shop_item" WHERE "parent_id" = $1 AND "id" <> $2`, Self: true},
	}
	if !slices.Equal(got, want) {
		t.Errorf("refs:\n%+v\nwant\n%+v", got, want)
	}
	if file := string(File(project.Postgres, apps[1], apps)); !strings.Contains(file, `{Model: "notes.Note", Field: "ItemID", SQL: `+"`"+`SELECT COUNT(*) FROM "notes" WHERE "item_id" = $1`+"`"+`}`) ||
		!strings.Contains(file, "Delete: DeleteItem,") {
		t.Errorf("admin_gen.go:\n%s", file)
	}
}
