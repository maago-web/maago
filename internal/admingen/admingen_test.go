package admingen

import (
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
	if got := Statements(note, project.SQLite).List; !strings.HasSuffix(got, `ORDER BY "id" LIMIT ? OFFSET ?`) {
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
	if st := Statements(item, project.SQLite); st.Update != "" {
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
	if got := string(File(project.SQLite, app, []*model.App{app})); !strings.Contains(got, `maago.BindString(ctx, vals, &f.Password, maago.Rule{Name: "password", Label: "Password"})`) {
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
	for _, want := range []string{"On maago.Field[string]", `maago.BindString(ctx, vals, &f.On, maago.Rule{Name: "on", Label: "On", Choices: []string{"yes", "no"}, Optional: true})`,
		`f.On.Raw = "yes"`, "On: maago.Set(admin.Bool(f.On))", "admin.Bool(f.On), id"} {
		if !strings.Contains(got, want) {
			t.Errorf("admin_gen.go lacks %s:\n%s", want, got)
		}
	}
}
