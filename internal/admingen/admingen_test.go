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
