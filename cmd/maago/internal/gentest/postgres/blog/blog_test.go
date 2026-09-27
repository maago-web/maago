package blog

import (
	"bytes"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/maago-web/maago/cmd/maago/internal/gentest"
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
