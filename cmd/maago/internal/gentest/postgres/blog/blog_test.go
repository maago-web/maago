package blog

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/maago-web/maago"
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
	got.Order, got.CreatedAt = 3, time.Time{}
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

// input is the input of f named name.
func input(t *testing.T, f admin.Form, name string) admin.Input {
	t.Helper()
	for _, in := range f.Inputs {
		if in.Name == name {
			return in
		}
	}
	t.Fatalf("no input %s in %+v", name, f.Inputs)
	return admin.Input{}
}

// sent is what a browser sends of f drawn: each input's text, a checkbox
// only when checked.
func sent(f admin.Form) url.Values {
	vals := url.Values{}
	for _, in := range f.Inputs {
		if in.Kind != "readonly" && (in.Kind != "checkbox" || in.Raw != "") {
			vals.Set(in.Name, in.Raw)
		}
	}
	if !f.Version.IsZero() { // the hidden input
		vals.Set("_version", strconv.FormatInt(f.Version.UnixMicro(), 10))
	}
	return vals
}

// The admin's forms of the shared example (D-132): a form sent back
// untouched keeps every value, a time to the microsecond; a change writes
// only the fields the form changes, whatever else the request sends; an
// fk number of no row is the field's error; neither model has adding.
func TestAdminForm(t *testing.T) {
	ctx, db := gentest.Open(t, "internal/testdata/example")
	post, author := Admin[0], Admin[1]
	if post.Add || author.Add || !post.Edit || !author.Edit {
		t.Errorf("Add %t %t, Edit %t %t", post.Add, author.Add, post.Edit, author.Edit)
	}
	a, err := InsertAuthor(ctx, db, NewAuthorInput{Name: "kim", EditorID: 1, Code: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 28, 9, 0, 0, 123_456_000, time.UTC)
	p, err := InsertPost(ctx, db, NewPostInput{AuthorID: a.ID, Title: "t", Slug: "s", Body: []byte{}, PublishedAt: &at, Score: maago.Set(maago.Ptr(0.1))})
	if err != nil {
		t.Fatal(err)
	}

	f, err := post.Form(ctx, db, p.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if in := input(t, f, "published_at"); in.Raw != "2026-09-28T09:00:00.123456" || in.Kind != "datetime" {
		t.Errorf("published_at: %+v", in)
	}
	vals := sent(f)
	vals.Set("slug", "hacked") // readonly: not the form's
	if f, err := post.Form(ctx, db, p.ID, vals); err != nil || !f.Saved {
		t.Fatalf("untouched: %v %+v", err, f)
	}
	got, err := GetPost(ctx, db, p.ID)
	if err != nil || got.Slug != "s" || got.PublishedAt == nil || !got.PublishedAt.Equal(at) || *got.Score != 0.1 || got.Title != "t" {
		t.Errorf("after an untouched form: %+v %v", got, err)
	}

	vals.Set("author_id", "999")
	if f, err := post.Form(ctx, db, p.ID, vals); err != nil || f.Saved || input(t, f, "author_id").Error != "Select a row that exists." {
		t.Errorf("an fk of no row: %v %+v", err, f)
	}
	vals.Set("author_id", "x")
	if f, err := post.Form(ctx, db, p.ID, vals); err != nil || f.Saved || input(t, f, "author_id").Error != "Enter a number." {
		t.Errorf("an fk that is not a number: %v %+v", err, f)
	}

	af, err := author.Form(ctx, db, a.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	vals = sent(af)
	vals.Set("name", "lee")
	vals.Set("code", "hacked") // hidden
	vals.Set("created_at", "2000-01-01T00:00")
	if f, err := author.Form(ctx, db, a.ID, vals); err != nil || !f.Saved {
		t.Fatalf("author: %v %+v", err, f)
	}
	ga, err := GetAuthor(ctx, db, a.ID)
	if err != nil || ga.Name != "lee" || ga.Code != "c1" || !ga.CreatedAt.Equal(a.CreatedAt) || !ga.UpdatedAt.After(a.UpdatedAt) {
		t.Errorf("author after the form: %+v %v, was %+v", ga, err, a)
	}
	for _, in := range af.Inputs {
		if in.Name == "code" || in.Label == "Code" {
			t.Errorf("the hidden Code is in the form: %+v", in)
		}
	}

	if _, err := post.Form(ctx, db, p.ID+100, nil); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("no row: %v", err)
	}
	if _, err := post.Form(ctx, db, p.ID+100, vals); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("no row, sent: %v", err)
	}
}

// What keeps a row from being deleted in the admin (D-135): the rows of
// each field that refers to it, but not the row itself; and a row that
// is not there.
func TestAdminDelete(t *testing.T) {
	ctx, db := gentest.Open(t, "internal/testdata/example")
	post, author := Admin[0], Admin[1]
	// Each edits itself (EditorID is unique, ids start at 1); b is a's
	// parent and c's reviewer; c writes two posts.
	var as []Author
	for i, name := range []string{"a", "b", "c"} {
		x, err := InsertAuthor(ctx, db, NewAuthorInput{Name: name, EditorID: int64(i + 1), Code: name})
		if err != nil || x.ID != int64(i+1) {
			t.Fatalf("author %s: %+v %v", name, x, err)
		}
		as = append(as, x)
	}
	a, b, c := as[0], as[1], as[2]
	a.ParentID, c.ReviewerID = &b.ID, &b.ID
	for _, x := range []*Author{&a, &c} {
		if err := UpdateAuthor(ctx, db, x); err != nil {
			t.Fatal(err)
		}
	}
	var posts []int64
	for _, slug := range []string{"p1", "p2"} {
		p, err := InsertPost(ctx, db, NewPostInput{AuthorID: c.ID, Title: "t", Slug: slug, Body: []byte{}})
		if err != nil {
			t.Fatal(err)
		}
		posts = append(posts, p.ID)
	}
	counts := func(id int64) string {
		refs, err := author.Refs(ctx, db, id)
		if err != nil {
			t.Fatal(err)
		}
		var s []string
		for _, r := range refs {
			s = append(s, fmt.Sprintf("%s.%s=%d", r.Model, r.Field, r.N))
		}
		return strings.Join(s, " ")
	}
	for _, c := range []struct {
		id   int64
		want string
	}{
		{a.ID, "blog.Post.AuthorID=0 blog.Author.ParentID=0 blog.Author.EditorID=0 blog.Author.ReviewerID=0"},
		{b.ID, "blog.Post.AuthorID=0 blog.Author.ParentID=1 blog.Author.EditorID=0 blog.Author.ReviewerID=1"},
		{c.ID, "blog.Post.AuthorID=2 blog.Author.ParentID=0 blog.Author.EditorID=0 blog.Author.ReviewerID=0"},
	} {
		if got := counts(c.id); got != c.want {
			t.Errorf("author %d: %s, want %s", c.id, got, c.want)
		}
	}
	if _, err := author.Refs(ctx, db, c.ID+100); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("no author: %v", err)
	}
	if refs, err := post.Refs(ctx, db, posts[0]); err != nil || refs != nil {
		t.Errorf("post refs: %v %v", refs, err)
	}
	// a, which refers only to itself, is deleted, as its Refs said.
	if err := author.Delete(ctx, db, a.ID); err != nil {
		t.Errorf("an author that edits itself: %v", err)
	}
	if err := post.Delete(ctx, db, posts[0]); err != nil {
		t.Fatal(err)
	}
	if err := post.Delete(ctx, db, posts[0]); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("deleted twice: %v", err)
	}
}

// The admin's search and filters (D-134), on data that tells a mutation
// apart: the escapes, the case, the parentheses of the search, each
// filter and the count with it, a NULL left out, and bad values.
func TestAdminSearch(t *testing.T) {
	ctx, db := gentest.Open(t, "internal/testdata/example")
	post, author := Admin[0], Admin[1]
	a, err := InsertAuthor(ctx, db, NewAuthorInput{Name: "a", EditorID: 1, Code: "a"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := InsertAuthor(ctx, db, NewAuthorInput{Name: "b", EditorID: 2, ParentID: &a.ID, Code: "b"})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for _, p := range []struct {
		author      int64
		title, slug string
		status      string
		public      bool
	}{
		{a.ID, "a%b", "s1", "draft", true}, {a.ID, "axb", "s2", "draft", true}, {a.ID, "x_y", "s3", "published", false},
		{b.ID, "xzy", "s4", "published", true}, {b.ID, `a\b`, "s5", "draft", false}, {b.ID, "hello", "s6", "published", true},
		{a.ID, "other", "hello-slug", "draft", true},
	} {
		o, err := InsertPost(ctx, db, NewPostInput{AuthorID: p.author, Title: p.title, Slug: p.slug, Status: maago.Set(p.status), Public: maago.Set(p.public), Body: []byte{}})
		if err != nil {
			t.Fatal(err)
		}
		ids[p.title] = o.ID
	}
	list := func(m admin.Model, q admin.Query) ([]string, error) {
		if q.Limit == 0 {
			q.Limit = 50
		}
		n, err := m.Count(ctx, db, q)
		if err != nil {
			return nil, err
		}
		rows, err := m.Rows(ctx, db, q)
		if err != nil {
			return nil, err
		}
		var got []string
		for _, r := range rows {
			got = append(got, fmt.Sprint(r.Cells[0]))
		}
		slices.Sort(got)
		if q.Offset == 0 && int64(len(got)) != n {
			t.Errorf("%+v: %d rows, count %d", q, len(got), n)
		}
		return got, nil
	}
	f := func(kv ...string) map[string]string {
		m := map[string]string{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	all := []string{`a\b`, "a%b", "axb", "hello", "other", "x_y", "xzy"}
	for _, c := range []struct {
		q    admin.Query
		want []string
	}{
		{admin.Query{Search: "a%b"}, []string{"a%b"}},
		{admin.Query{Search: "x_y"}, []string{"x_y"}},
		{admin.Query{Search: `a\b`}, []string{`a\b`}},
		{admin.Query{Search: "HELLO"}, []string{"hello", "other"}},
		{admin.Query{Search: "HELLO", Filters: f("status", "published")}, []string{"hello"}},
		{admin.Query{Search: "  "}, all},
		{admin.Query{Filters: f("status", "")}, all},
		{admin.Query{Filters: f("status", "draft")}, []string{`a\b`, "a%b", "axb", "other"}},
		{admin.Query{Filters: f("public", "no")}, []string{`a\b`, "x_y"}},
		{admin.Query{Filters: f("author_id", fmt.Sprint(b.ID))}, []string{`a\b`, "hello", "xzy"}},
		{admin.Query{Filters: f("status", "draft"), Limit: 2, Offset: 2}, []string{"axb", "other"}},
	} {
		got, err := list(post, c.q)
		if slices.Sort(c.want); err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%+v: %q %v, want %q", c.q, got, err, c.want)
		}
	}
	if got, err := list(author, admin.Query{Filters: f("parent_id", fmt.Sprint(a.ID))}); err != nil || !slices.Equal(got, []string{fmt.Sprint(b.ID)}) {
		t.Errorf("parent filter: %q %v", got, err) // a's NULL parent is left out
	}
	for _, bad := range []map[string]string{f("status", "nope"), f("public", "maybe"), f("author_id", "x"), f("author_id", "1.5")} {
		if _, err := post.Count(ctx, db, admin.Query{Filters: bad}); err == nil {
			t.Errorf("%v: no error", bad)
		}
		if _, err := post.Rows(ctx, db, admin.Query{Limit: 50, Filters: bad}); err == nil {
			t.Errorf("%v: no error from Rows", bad)
		}
	}
}
