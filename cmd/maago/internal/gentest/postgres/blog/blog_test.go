package blog

import (
	"bytes"
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/maago-web/maago/cmd/maago/internal/gentest"
	"github.com/maago-web/maago/contrib/admin"
)

// The shared example's CRUD: a reserved word (order), []byte, defaults of
// every kind, fields that are not columns, and both auto times.
func TestCRUD(t *testing.T) {
	ctx, db := gentest.Open(t, "internal/testdata/example")
	a, err := InsertAuthor(ctx, db, NewAuthorInput{Name: "kim", EditorID: 1, Code: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := GetAuthor(ctx, db, a.ID)
	if err != nil || got.Order != 7 || got.Admin || got.Name != "kim" || got.EditorID != 1 || got.ParentID != nil ||
		!got.CreatedAt.Equal(a.CreatedAt) || !got.UpdatedAt.Equal(a.CreatedAt) {
		t.Errorf("author: %+v %v, inserted %+v", got, err, a)
	}

	p, err := InsertPost(ctx, db, NewPostInput{AuthorID: a.ID, Title: "t", Slug: "s", Body: []byte("body")})
	if err != nil {
		t.Fatal(err)
	}
	gp, err := GetPost(ctx, db, p.ID)
	if err != nil || gp.Status != "draft" || gp.Views != 0 || gp.Score == nil || *gp.Score != 1.5 || !gp.Public ||
		!bytes.Equal(gp.Body, []byte("body")) || gp.PublishedAt != nil {
		t.Errorf("post: %+v %v", gp, err)
	}

	// A nullable time with a value goes in and comes back.
	at := time.Date(2026, 9, 28, 9, 0, 0, 123_456_000, time.UTC)
	p2, err := InsertPost(ctx, db, NewPostInput{AuthorID: a.ID, Title: "t2", Slug: "s2", Body: []byte{}, PublishedAt: &at})
	if err != nil {
		t.Fatal(err)
	}
	if gp, err := GetPost(ctx, db, p2.ID); err != nil || gp.PublishedAt == nil || !gp.PublishedAt.Equal(at) {
		t.Errorf("PublishedAt: %v %v, want %v", gp.PublishedAt, err, at)
	}

	// Update keeps CreatedAt, which only Insert writes, and sets UpdatedAt.
	got.Order, got.CreatedAt, got.UpdatedAt = 3, time.Time{}, time.Time{}
	if err := UpdateAuthor(ctx, db, &got); err != nil {
		t.Fatal(err)
	}
	after, err := GetAuthor(ctx, db, a.ID)
	if err != nil || after.Order != 3 || !after.CreatedAt.Equal(a.CreatedAt) || !after.UpdatedAt.Equal(got.UpdatedAt) || got.UpdatedAt.IsZero() {
		t.Errorf("after update: %+v %v, o %+v", after, err, got)
	}

	if err := DeletePost(ctx, db, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := DeletePost(ctx, db, p.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("second delete: %v", err)
	}
}

// The admin's list of posts (D-132, D-134): the columns of admin_list,
// the newest PublishedAt first and NULL last in both dialects, then the
// Title, then the newest ID; pages, and a page past the end; and Author
// without its hidden Code.
func TestAdmin(t *testing.T) {
	ctx, db := gentest.Open(t, "internal/testdata/example")
	post, author := Admin[0], Admin[1]
	a, err := InsertAuthor(ctx, db, NewAuthorInput{Name: "kim", EditorID: 1, Code: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	later := at.Add(time.Hour)
	var ids []int64
	for i, p := range []struct {
		title string
		at    *time.Time
	}{{"a", nil}, {"x", &at}, {"b", nil}, {"y", &later}, {"x", &at}} {
		o, err := InsertPost(ctx, db, NewPostInput{AuthorID: a.ID, Title: p.title, Slug: string(rune('a' + i)), Body: []byte{}, PublishedAt: p.at})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, o.ID)
	}
	want := []int64{ids[3], ids[4], ids[1], ids[0], ids[2]}
	for _, page := range []struct {
		offset int
		want   []int64
	}{{0, want[:2]}, {2, want[2:4]}, {4, want[4:]}, {10, []int64{}}} {
		rows, err := post.Rows(ctx, db, admin.Query{Limit: 2, Offset: page.offset})
		var got []int64
		for _, r := range rows {
			got = append(got, r.ID)
		}
		if err != nil || rows == nil || !slices.Equal(got, page.want) {
			t.Errorf("offset %d: %v %v (nil: %t), want %v", page.offset, got, err, rows == nil, page.want)
		}
	}
	rows, err := post.Rows(ctx, db, admin.Query{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if c := rows[0].Cells; len(c) != 3 || c[0] != "y" || c[1] != "draft" || !c[2].(*time.Time).Equal(later) {
		t.Errorf("cells: %v", c)
	}
	if p := rows[4].Cells[2].(*time.Time); p != nil {
		t.Errorf("a NULL PublishedAt is %v", p)
	}
	if n, err := post.Count(ctx, db, admin.Query{}); err != nil || n != 5 {
		t.Errorf("count: %d %v", n, err)
	}

	for _, f := range author.Fields {
		if f.Name == "Code" {
			t.Error("Author.Code is admin=hidden, but the admin has it")
		}
	}
	rows, err = author.Rows(ctx, db, admin.Query{Limit: 1})
	if err != nil || len(rows) != 1 || len(rows[0].Cells) != len(author.List) || slices.Contains(author.List, "Code") {
		t.Errorf("author: %v %v, list %v", rows, err, author.List)
	}
}
