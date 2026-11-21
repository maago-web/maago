package site

import (
	"context"
	"database/sql"
	"errors"
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

func PostDetail(ctx context.Context, db maago.DB, in PostDetailIn) (PostPage, error) {
	p, err := GetPost(ctx, db, in.ID)
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

func PostCreate(ctx context.Context, tx maago.Tx, in struct{}) (maago.Result[PostPage], error) {
	p, err := InsertPost(ctx, tx, NewPostInput{Title: "new"})
	if err != nil {
		return maago.Result[PostPage]{}, err
	}
	return maago.SeeOther[PostPage](PostDetailURL(p.ID)), nil
}

func APIPost(ctx context.Context, db maago.DB, in PostDetailIn) (Post, error) {
	return GetPost(ctx, db, in.ID)
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

// PostForm draws the form.
func PostForm(ctx context.Context, in PostFormIn) (PostFormPage, error) {
	return PostFormPage{in.Form}, nil
}

// PostFormSave saves the form. A taken title is a check that needs the
// database, so the view makes it (D-119), and the field error it sets
// makes the form not Valid.
func PostFormSave(ctx context.Context, tx maago.Tx, in PostFormIn) (maago.Result[PostFormPage], error) {
	f := in.Form
	if f.Valid() {
		taken, err := TitleTaken(ctx, tx, TitleTakenParams{Title: f.Title.Value})
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
	p, err := InsertPost(ctx, tx, NewPostInput{Title: f.Title.Value})
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
func APIPostCreate(ctx context.Context, tx maago.Tx, in PostFormIn) (Post, error) {
	apiCalls++
	f := in.Form
	taken, err := TitleTaken(ctx, tx, TitleTakenParams{Title: f.Title.Value})
	if err != nil {
		return Post{}, err
	}
	if taken.Taken {
		return Post{}, maago.Invalid(&f.Title, "that title is taken")
	}
	return InsertPost(ctx, tx, NewPostInput{Title: f.Title.Value})
}

// exportGate, when a test sets it, holds PostExport and Stream between their two rows,
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

// StreamIn says what Stream does.
type StreamIn struct {
	How string `maago:"path=how"`
}

// Stream writes the response itself (D-157): the user, the database and
// In come as a typed view's, w and r after them. It sets a cookie, which
// an error before writing drops. How "gone", "refuse" and "fail" return
// an error before writing; "late" one after the first row; any other
// streams two rows, the first flushed before the second is written.
func Stream(ctx context.Context, u Account, db maago.DB, in StreamIn, w http.ResponseWriter, r *http.Request) error {
	maago.SetCookie(ctx, &http.Cookie{Name: "seen", Value: u.Name})
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM site_post`).Scan(&n); err != nil {
		return err
	}
	switch in.How {
	case "gone":
		return fmt.Errorf("stream: %w", sql.ErrNoRows)
	case "refuse":
		return maago.Status(http.StatusForbidden, errors.New("not yours"))
	case "fail":
		return errors.New("stream failed")
	}
	w.Header().Set("Content-Type", "text/csv")
	fmt.Fprintf(w, "user,%s\n", u.Name)
	w.(http.Flusher).Flush()
	if in.How == "late" {
		return errors.New("stream failed late")
	}
	if exportGate != nil {
		select {
		case <-exportGate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	fmt.Fprintf(w, "posts,%d\n", n)
	return nil
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

// Secret requires login: u is an Account, not a pointer (D-149).
func Secret(ctx context.Context, u Account, in struct{}) (SecretPage, error) {
	return SecretPage{u.Name}, nil
}

// Mine takes the user and the database in its signature (D-149): the
// route requires login because u is an Account, not a pointer.
func Mine(ctx context.Context, u Account, db maago.DB, in struct{}) (SecretPage, error) {
	if _, err := TitleTaken(ctx, db, TitleTakenParams{Title: u.Name}); err != nil {
		return SecretPage{}, err
	}
	return SecretPage{u.Name}, nil
}

// SaveTitleIn is the title SaveTitle writes.
type SaveTitleIn struct {
	Title string `maago:"path=title"`
}

// committed are the titles whose AfterCommit ran.
var committed []string

// SaveTitle writes in the request's transaction (D-150): an error, a
// panic, or a page that fails to draw keeps nothing, and AfterCommit's
// work runs only after the commit.
func SaveTitle(ctx context.Context, tx maago.Tx, in SaveTitleIn) (maago.Result[BrokenPage], error) {
	p, err := InsertPost(ctx, tx, NewPostInput{Title: in.Title})
	if err != nil {
		return maago.Result[BrokenPage]{}, err
	}
	maago.AfterCommit(ctx, func(context.Context) { committed = append(committed, in.Title) })
	switch in.Title {
	case "bad":
		return maago.Result[BrokenPage]{}, errors.New("bad")
	case "boom":
		panic("boom")
	case "draw":
		return maago.Show(BrokenPage{}), nil // its template reads .Post.Title of a nil Post
	}
	return maago.SeeOther[BrokenPage](PostDetailURL(p.ID)), nil
}

// Greet takes a user who may be missing.
func Greet(ctx context.Context, u *Account, in struct{}) (SecretPage, error) {
	if u == nil {
		return SecretPage{"nobody"}, nil
	}
	return SecretPage{u.Name}, nil
}

// TagForm is a model form of every field of Tag.
type TagForm struct {
	maago.Form `maago:"model=Tag"`
	Name       maago.Field[string]
	Kind       maago.Field[string]
	Code       maago.Field[string]
}

type TagIn struct {
	Form TagForm `maago:"form"`
}

type TagPage struct {
	Form  TagForm
	First string // the first tag's name, read after a refused insert
}

// TagSave inserts a tag. A value a unique constraint refuses is the
// form's, by Fail; the transaction goes on to read (D-154).
func TagSave(ctx context.Context, tx maago.Tx, in TagIn) (maago.Result[TagPage], error) {
	f := in.Form
	if f.Valid() {
		_, err := InsertTag(ctx, tx, NewTagInput{Name: f.Name.Value, Kind: f.Kind.Value, Code: f.Code.Value})
		if err == nil {
			return maago.SeeOther[TagPage](TagSaveURL()), nil
		}
		if !f.Fail(err) {
			return maago.Result[TagPage]{}, err
		}
	}
	first, err := GetTag(ctx, tx, 1)
	if err != nil {
		return maago.Result[TagPage]{}, err
	}
	return maago.Show(TagPage{f, first.Name}), nil
}

// APITagSave inserts a tag from JSON, in a transaction, and returns the
// error as it is: a unique refusal is maago's 422. A tag named 409 gives
// its own code instead.
func APITagSave(ctx context.Context, tx maago.Tx, in TagIn) (Tag, error) {
	f := in.Form
	t, err := InsertTag(ctx, tx, NewTagInput{Name: f.Name.Value, Kind: f.Kind.Value, Code: f.Code.Value})
	return t, conflict(f.Name.Value, err)
}

type TagNameIn struct {
	Name string `maago:"path=name"`
}

// APITagName inserts a tag of the name in the path, with no form and no
// transaction: a taken name is maago's 422 all the same.
func APITagName(ctx context.Context, db maago.DB, in TagNameIn) (Tag, error) {
	t, err := InsertTag(ctx, db, NewTagInput{Name: in.Name, Kind: "path", Code: in.Name})
	return t, conflict(in.Name, err)
}

// conflict is err, as a 409 for a tag named 409.
func conflict(name string, err error) error {
	if err != nil && name == "409" {
		return maago.Status(http.StatusConflict, err)
	}
	return err
}

type KeyIn struct {
	Key maago.UUID  `maago:"path=key"`
	Ref *maago.UUID `maago:"query=ref"`
}

type KeyPage struct{ Key, Ref maago.UUID }

// KeyShow reads a UUID from the path and one from the query: one that
// does not parse is a 404 or a 400.
func KeyShow(ctx context.Context, in KeyIn) (KeyPage, error) {
	p := KeyPage{Key: in.Key}
	if in.Ref != nil {
		p.Ref = *in.Ref
	}
	return p, nil
}

// PriceForm reads a UUID and a price, in cents.
type PriceForm struct {
	maago.Form
	Key   maago.Field[maago.UUID]
	Price maago.Field[int64] `maago:"decimal=2,min_value=0.01"`
}

type PriceIn struct {
	Form PriceForm `maago:"form"`
}

type Price struct {
	Key   maago.UUID
	Cents int64
}

func APIPrice(ctx context.Context, tx maago.Tx, in PriceIn) (Price, error) {
	return Price{in.Form.Key.Value, in.Form.Price.Value}, nil
}
