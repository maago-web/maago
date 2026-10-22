package policy

import (
	"database/sql"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/cmd/maago/internal/gentest"
	"github.com/maago-web/maago/contrib/admin"
)

// The same queries and GetPost read other rows for nobody, another user,
// the author, a teammate and staff: what the policies let each read.
func TestPolicies(t *testing.T) {
	ctx, db := gentest.Open(t, "internal/testdata/policy")
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	team := int64(1)
	user := func(name string, staff bool, team *int64) *User {
		u, err := InsertUser(ctx, db, NewUserInput{Name: name, IsStaff: staff, TeamID: team, JoinedAt: now.Add(-24 * time.Hour)})
		if err != nil {
			t.Fatal(err)
		}
		return &u
	}
	other, author, mate, staff := user("other", false, nil), user("author", false, nil), user("mate", false, &team), user("staff", true, nil)
	later, before := now.Add(time.Hour), now.Add(-time.Hour)
	post := func(title, status string, hidden bool, by *User, team *int64, at *time.Time) int64 {
		p, err := InsertPost(ctx, db, NewPostInput{Title: title, Status: maago.Set(status), Hidden: hidden, AuthorID: &by.ID, TeamID: team, PublishAt: at})
		if err != nil {
			t.Fatal(err)
		}
		return p.ID
	}
	pub := post("pub", "published", false, other, nil, nil)
	draft := post("draft", "draft", false, author, nil, nil)
	hidden := post("hidden", "published", true, author, nil, nil)
	soon := post("soon", "published", false, author, nil, &later)
	teams := post("teams", "draft", false, staff, &team, &before)
	for _, id := range []int64{pub, draft} {
		if _, err := InsertComment(ctx, db, NewCommentInput{PostID: id, Text: "on " + strconvI(id)}); err != nil {
			t.Fatal(err)
		}
	}

	for _, c := range []struct {
		name string
		u    *User
		read []int64 // by GetPost
	}{
		{"nobody", nil, []int64{pub}},
		{"other", other, []int64{pub}},
		{"author", author, []int64{pub, draft, soon}},
		{"mate", mate, []int64{pub, teams}},
		{"staff", staff, []int64{pub, draft, soon, teams}},
	} {
		var got []int64
		for _, id := range []int64{pub, draft, hidden, soon, teams} {
			switch p, err := GetPost(ctx, c.u, db, id, now); {
			case err == nil:
				got = append(got, p.ID)
			case !errors.Is(err, sql.ErrNoRows):
				t.Fatal(err)
			}
			d, err := PostDetail(ctx, c.u, db, PostDetailParams{ID: id, Now: now})
			if read := slices.Contains(c.read, id); err == nil != read || read && (d.Author == nil || d.ID != id) {
				t.Errorf("%s: PostDetail(%d) = %+v, %v; may read: %v", c.name, id, d, err, read)
			}
		}
		if !slices.Equal(got, c.read) {
			t.Errorf("%s: GetPost reads %v, want %v", c.name, got, c.read)
		}
		// The author's posts are what the user may read of them.
		mine, err := MyPosts(ctx, c.u, db, MyPostsParams{AuthorID: author.ID, Now: now})
		var ids []int64
		for _, p := range mine {
			ids = append(ids, p.ID)
		}
		want := slices.DeleteFunc(slices.Clone(c.read), func(id int64) bool { return id == pub || id == teams })
		if err != nil || !slices.Equal(ids, want) {
			t.Errorf("%s: MyPosts = %v, %v, want %v", c.name, ids, err, want)
		}
		// A policy inside EXISTS: the comments on what the user may read.
		cs, err := ReadableComments(ctx, c.u, db, ReadableCommentsParams{Now: now})
		var on []string
		for _, cm := range cs {
			on = append(on, cm.Text)
		}
		wantOn := []string{"on " + strconvI(pub)}
		if slices.Contains(c.read, draft) {
			wantOn = append(wantOn, "on "+strconvI(draft))
		}
		if err != nil || !slices.Equal(on, wantOn) {
			t.Errorf("%s: ReadableComments = %v, %v, want %v", c.name, on, err, wantOn)
		}
		// The list is what is out, for everybody alike, in WHERE { } with a
		// condition beside it.
		list, err := PostList(ctx, c.u, db, PostListParams{Now: now})
		if err != nil || len(list) != 1 || list[0].ID != pub {
			t.Errorf("%s: PostList = %+v, %v", c.name, list, err)
		}
		q := "nothing"
		if list, err := PostList(ctx, c.u, db, PostListParams{Q: &q, Now: now}); err != nil || len(list) != 0 {
			t.Errorf("%s: PostList(q) = %+v, %v", c.name, list, err)
		}
	}
	// Once published, the post is out by the time it says.
	if _, err := GetPost(ctx, nil, db, soon, later); err != nil {
		t.Errorf("GetPost(soon) at its time: %v", err)
	}

	// What reads every row: GetPostIgnoringPolicies, a query with none,
	// and the admin, which opens the hidden post to change it.
	if p, err := GetPostIgnoringPolicies(ctx, db, hidden); err != nil || p.Title != "hidden" {
		t.Errorf("GetPostIgnoringPolicies = %+v, %v", p, err)
	}
	if n, err := EveryPost(ctx, db); err != nil || n != 5 {
		t.Errorf("EveryPost = %d, %v", n, err)
	}
	f, err := Admin[1].Form(ctx, db, hidden, nil)
	if err != nil || !slices.ContainsFunc(f.Inputs, func(in admin.Input) bool { return in.Name == "title" && in.Raw == "hidden" }) {
		t.Errorf("admin form of the hidden post: %+v, %v", f, err)
	}
}

func strconvI(n int64) string { return strconv.FormatInt(n, 10) }
