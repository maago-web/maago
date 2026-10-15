package site

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/maago-web/maago"
)

type PostDetailIn struct {
	ID int64 `maago:"path=id"`
}

type PostPage struct {
	Post Post
}

func PostDetail(ctx context.Context, in PostDetailIn) (PostPage, error) {
	p, err := GetPost(ctx, maago.DBFrom(ctx), in.ID)
	return PostPage{Post: p}, err
}

type PageIn struct {
	N int32 `maago:"path=n"`
}

type NumberPage struct {
	N  int32
	At time.Time // shown in Config.TimeZone
}

func Page(ctx context.Context, in PageIn) (NumberPage, error) {
	return NumberPage{N: in.N, At: time.Date(2026, 10, 1, 0, 30, 0, 0, time.UTC)}, nil
}

type ListIn struct {
	Page  int32 `maago:"query=page"`
	Draft *bool `maago:"query=draft"`
}

type ListPage struct {
	Page  int32
	Draft string
}

func PostList(ctx context.Context, in ListIn) (ListPage, error) {
	draft := "unset"
	if in.Draft != nil {
		draft = map[bool]string{true: "yes", false: "no"}[*in.Draft]
	}
	return ListPage{Page: in.Page, Draft: draft}, nil
}

func PostCreate(ctx context.Context, in struct{}) (maago.Result[PostPage], error) {
	p, err := InsertPost(ctx, maago.DBFrom(ctx), NewPostInput{Title: "new"})
	if err != nil {
		return maago.Result[PostPage]{}, err
	}
	return maago.SeeOther[PostPage](PostDetailURL(p.ID)), nil
}

func APIPost(ctx context.Context, in PostDetailIn) (Post, error) {
	return GetPost(ctx, maago.DBFrom(ctx), in.ID)
}

type BrokenPage struct {
	Post *Post // nil: the template fails on it
}

func Broken(ctx context.Context, in struct{}) (BrokenPage, error) { return BrokenPage{}, nil }

// NewPostForm is a plain form: an int64 for the three results of a
// number, a checkbox, and a check of the form as a whole (D-119).
type NewPostForm struct {
	maago.Form
	Title maago.Field[string] `maago:"max=10,label='제목'"`
	Count maago.Field[int64]  `maago:"max_value=100"`
	Agree maago.Field[bool]
}

func (f *NewPostForm) Validate() {
	if f.Title.Value == "forbidden" {
		f.AddError("that title is forbidden")
	}
	if f.Title.Value == "nope" {
		f.Title.Error = "nope is not a title"
	}
}

type PostFormIn struct {
	Form NewPostForm `maago:"form"`
}

type PostFormPage struct {
	Form NewPostForm
}

// PostForm draws the form on GET and saves it on POST. A taken title is
// a check that needs the database, so the view makes it (D-119), and the
// field error it sets makes the form not Valid.
func PostForm(ctx context.Context, in PostFormIn) (maago.Result[PostFormPage], error) {
	f := in.Form
	db := maago.DBFrom(ctx)
	if f.Valid() {
		taken, err := TitleTaken(ctx, db, TitleTakenParams{Title: f.Title.Value})
		if err != nil {
			return maago.Result[PostFormPage]{}, err
		}
		if taken.Taken {
			f.Title.Error = "that title is taken"
		}
	}
	if !f.Valid() {
		return maago.Show(PostFormPage{f}), nil
	}
	p, err := InsertPost(ctx, db, NewPostInput{Title: f.Title.Value})
	if err != nil {
		return maago.Result[PostFormPage]{}, err
	}
	return maago.SeeOther[PostFormPage](PostDetailURL(p.ID)), nil
}

// apiCalls counts the calls of APIPostCreate: an invalid form must not
// reach it (D-120).
var apiCalls int

// APIPostCreate takes the same form as JSON. What only the database can
// say comes back as maago.Invalid, a 422.
func APIPostCreate(ctx context.Context, in PostFormIn) (Post, error) {
	apiCalls++
	f := in.Form
	db := maago.DBFrom(ctx)
	taken, err := TitleTaken(ctx, db, TitleTakenParams{Title: f.Title.Value})
	if err != nil {
		return Post{}, err
	}
	if taken.Taken {
		return Post{}, maago.Invalid(&f.Title, "that title is taken")
	}
	return InsertPost(ctx, db, NewPostInput{Title: f.Title.Value})
}

// exportGate, when a test sets it, holds PostExport between its two rows,
// so the test sees the first arrive alone. Nil, it holds nothing.
var exportGate chan struct{}

// PostExport is an escape view (D-41) that streams a CSV: the first row,
// flushed, then the rest. Format "panic" fails after the first row.
func PostExport(w http.ResponseWriter, r *http.Request) {
	var n int
	if err := maago.DBFrom(r.Context()).QueryRowContext(r.Context(), `SELECT count(*) FROM site_post`).Scan(&n); err != nil {
		http.Error(w, "export failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="posts.csv"`)
	fmt.Fprintf(w, "format,%s\n", r.PathValue("format"))
	w.(http.Flusher).Flush()
	if r.PathValue("format") == "panic" {
		panic("export failed")
	}
	if exportGate != nil {
		select {
		case <-exportGate:
		case <-r.Context().Done():
			return
		}
	}
	fmt.Fprintf(w, "posts,%d\n", n)
}

// Account is the fixture's user: who a request says it is.
type Account struct{ Name string }

// Auth is a fake Config.Auth (D-121): a request is from the account its
// X-User header names. M5 brings sessions and tokens.
func Auth(r *http.Request) (any, error) {
	if name := r.Header.Get("X-User"); name != "" {
		return Account{name}, nil // a value: a view takes Account, or *Account for one who may be missing
	}
	return nil, nil
}

type SecretPage struct {
	Name string
}

// Secret requires login; the route table says so.
func Secret(ctx context.Context, in struct{}) (SecretPage, error) {
	a, _ := maago.UserFrom[Account](ctx)
	return SecretPage{a.Name}, nil
}

// Mine takes the user and the database in its signature (D-149): the
// route requires login because u is an Account, not a pointer.
func Mine(ctx context.Context, u Account, db maago.DB, in struct{}) (SecretPage, error) {
	if _, err := TitleTaken(ctx, db, TitleTakenParams{Title: u.Name}); err != nil {
		return SecretPage{}, err
	}
	return SecretPage{u.Name}, nil
}

// Greet takes a user who may be missing.
func Greet(ctx context.Context, u *Account, in struct{}) (SecretPage, error) {
	if u == nil {
		return SecretPage{"nobody"}, nil
	}
	return SecretPage{u.Name}, nil
}
