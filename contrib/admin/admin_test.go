package admin

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/rt"
)

// This package's tests make Handlers, which set the process's Prefix
// (D-122), so none of them is parallel.

// noDriver is a connector that is never connected: the models of these
// tests read no database.
type noDriver struct{}

func (noDriver) Connect(context.Context) (driver.Conn, error) { return nil, errors.New("no driver") }
func (noDriver) Driver() driver.Driver                        { return nil }

// rows is a model whose n rows are made by cells, and which records the
// limit and offset of each Query Rows was given.
func rows(app, name string, n int, fields []Field, cells func(i int) []any, got *[][2]int) Model {
	var list []string
	for _, f := range fields {
		list = append(list, f.Name)
	}
	return Model{App: app, Name: name, Fields: fields, List: list,
		Rows: func(_ context.Context, _ maago.DB, q Query) ([]Row, error) {
			if got != nil {
				*got = append(*got, [2]int{q.Limit, q.Offset})
			}
			out := []Row{}
			for i := q.Offset; i < min(q.Offset+q.Limit, n); i++ {
				out = append(out, Row{ID: int64(i + 1), Cells: cells(i)})
			}
			return out, nil
		},
		Count: func(context.Context, maago.DB, Query) (int64, error) { return int64(n), nil },
	}
}

func plain(app, name string) Model {
	return rows(app, name, 1, []Field{{Name: "ID", Label: "ID"}}, func(i int) []any { return []any{int64(i + 1)} }, nil)
}

// handler serves the admin of cfg, and an API app beside it. The user is
// the X-User header, and can tells what each user may view.
func handler(t *testing.T, cfg Config, prefix string, can map[string][]string) http.Handler {
	t.Helper()
	return handlerOn(t, sql.OpenDB(noDriver{}), cfg, prefix, can)
}

// handlerOn is handler on db.
func handlerOn(t *testing.T, db *sql.DB, cfg Config, prefix string, can map[string][]string) http.Handler {
	t.Helper()
	if cfg.Can == nil {
		cfg.Can = func(ctx context.Context, app, model, action string) (bool, error) {
			u, _ := maago.UserFrom[string](ctx)
			return action == View && (slices.Contains(can[u], "*") || slices.Contains(can[u], app+"."+model)), nil
		}
	}
	api := rt.NewApp("api", nil, rt.AppRoute{Method: "GET", Path: "/api/x", Name: "api_x", API: true, LoginRequired: true,
		Serve: func(w http.ResponseWriter, r *http.Request) error { return nil }})
	h, err := maago.Handler(maago.Config{DB: db, Apps: []maago.App{App(cfg), api}, LoginURL: "/login", Prefix: prefix,
		Auth: func(r *http.Request) (any, error) {
			if u := r.Header.Get("X-User"); u != "" {
				return u, nil
			}
			return nil, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { // back to no Prefix
		x := rt.NewApp("x", nil, rt.AppRoute{Method: "GET", Path: "/x", Name: "x", Serve: func(http.ResponseWriter, *http.Request) error { return nil }})
		if _, err := maago.Handler(maago.Config{DB: sql.OpenDB(noDriver{}), Apps: []maago.App{x}}); err != nil {
			t.Error(err)
		}
	})
	return h
}

func get(h http.Handler, user, target string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", target, nil)
	if user != "" {
		r.Header.Set("X-User", user)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// Without a user, the admin's pages send the browser to log in, and the
// API beside them answers 401 (M6's condition 3).
func TestLogin(t *testing.T) {
	h := handler(t, Config{Models: [][]Model{{plain("blog", "Post")}}}, "", nil)
	for _, target := range []string{"/admin/", "/admin/blog/Post/?page=2"} {
		w := get(h, "", target)
		u, err := url.Parse(w.Header().Get("Location"))
		if w.Code != http.StatusSeeOther || err != nil || u.Path != "/login" || u.Query().Get("next") != target {
			t.Errorf("%s: %d %q", target, w.Code, w.Header().Get("Location"))
		}
	}
	if w := get(h, "", "/api/x"); w.Code != http.StatusUnauthorized {
		t.Errorf("api: %d", w.Code)
	}
}

// The index has the models Can lets the user view, and none is 403; a
// list Can refuses is 403 whether the model exists or not, so the user
// learns nothing of the models; a model that is not there is 404 to a
// user who may view anything (D-131, D-133, M6's condition 4).
func TestCan(t *testing.T) {
	var asked []string
	cfg := Config{Models: [][]Model{{plain("blog", "Post"), plain("blog", "Tag")}, {plain("auth", "User")}},
		Can: func(ctx context.Context, app, model, action string) (bool, error) {
			u, _ := maago.UserFrom[string](ctx)
			asked = append(asked, app+"."+model+" "+action)
			return u == "root" || u == "ed" && app+"."+model == "blog.Tag", nil
		}}
	h := handler(t, cfg, "", nil)
	if w := get(h, "nobody", "/admin/"); w.Code != http.StatusForbidden {
		t.Errorf("index with no model: %d", w.Code)
	}
	if want := []string{"blog.Post view", "blog.Tag view", "auth.User view"}; !slices.Equal(asked, want) {
		t.Errorf("Can was asked %q, want %q", asked, want)
	}
	w := get(h, "ed", "/admin/")
	if body := w.Body.String(); w.Code != 200 || !strings.Contains(body, `href="/admin/blog/Tag/"`) || strings.Contains(body, "Post") || strings.Contains(body, "auth") {
		t.Errorf("ed's index: %d\n%s", w.Code, body)
	}
	if body := get(h, "root", "/admin/").Body.String(); strings.Count(body, "<h2>blog</h2>") != 1 || strings.Count(body, "<h2>auth</h2>") != 1 ||
		!strings.Contains(body, `href="/admin/blog/Post/">Post</a></li><li><a href="/admin/blog/Tag/">Tag</a>`) {
		t.Errorf("root's index:\n%s", body)
	}
	for _, c := range []struct {
		user, target string
		code         int
	}{
		{"ed", "/admin/blog/Tag/", 200},
		{"ed", "/admin/blog/Post/", 403},
		{"ed", "/admin/blog/Nope/", 403},
		{"root", "/admin/blog/Nope/", 404},
		{"root", "/admin/blog/post/", 404},
		{"root", "/admin/blog/Post/1/x/", 404},
	} {
		if w := get(h, c.user, c.target); w.Code != c.code {
			t.Errorf("%s %s: %d, want %d", c.user, c.target, w.Code, c.code)
		}
	}
	// ServeMux's redirect: 301 before Go 1.26, 307 from it.
	if w := get(h, "root", "/admin"); w.Code/100 != 3 || w.Header().Get("Location") != "/admin/" {
		t.Errorf("root /admin: %d %q", w.Code, w.Header().Get("Location"))
	}
}

// A list: the labels as its heads, each type of cell as text and escaped,
// and pages of 50 (D-134).
func TestList(t *testing.T) {
	at := time.Date(2026, 10, 13, 9, 30, 0, 0, time.UTC)
	s, n := "set", int64(7)
	fields := []Field{{Name: "Title", Label: "제목"}, {Name: "Note"}, {Name: "Num"}, {Name: "Public"}, {Name: "At"}, {Name: "Body"}, {Name: "Nil"}, {Name: "F"}, {Name: "Blob"}}
	for i := range fields {
		if fields[i].Label == "" {
			fields[i].Label = fields[i].Name
		}
	}
	var got [][2]int
	post := rows("blog", "Post", 51, fields, func(i int) []any {
		return []any{fmt.Sprintf("<b>%d</b>", i), &s, &n, true, at, []byte("abc"), (*string)(nil), 1e6, &[]byte{1}}
	}, &got)
	h := handler(t, Config{Models: [][]Model{{post}}}, "", map[string][]string{"ed": {"*"}})

	body := get(h, "ed", "/admin/blog/Post/").Body.String()
	for _, want := range []string{"<th>제목</th>", "<th>Note</th>", `<td><a href="/admin/blog/Post/1/">&lt;b&gt;0&lt;/b&gt;</a></td><td>set</td><td>7</td><td>yes</td><td>2026-10-13 09:30</td><td>(3 bytes)</td><td></td><td>1000000</td><td>(1 bytes)</td>`,
		"Rows: 51", "Page 1 of 2", `href="/admin/blog/Post/?page=2">Next`} {
		if !strings.Contains(body, want) {
			t.Errorf("page 1 lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "<b>") || strings.Contains(body, "Previous") || strings.Count(body, "<tr>") != 51 {
		t.Errorf("page 1:\n%s", body)
	}

	got = nil
	for _, c := range []struct {
		page, want string
		queried    [][2]int
	}{
		{"2", "Page 2 of 2", [][2]int{{50, 50}}},
		{"0", "Page 1 of 2", [][2]int{{50, 0}}},
		{"-1", "Page 1 of 2", [][2]int{{50, 0}}},
		{"3", "Page 3 of 2", nil},
		{"1000000000000000000", "No rows on this page.", nil},
	} {
		got = nil
		w := get(h, "ed", "/admin/blog/Post/?page="+c.page)
		if w.Code != 200 || !strings.Contains(w.Body.String(), c.want) || !slices.Equal(got, c.queried) {
			t.Errorf("page %s: %d, Rows got %v, want %v\n%s", c.page, w.Code, got, c.queried, w.Body)
		}
	}
	if w := get(h, "ed", "/admin/blog/Post/?page=5"); !strings.Contains(w.Body.String(), `href="/admin/blog/Post/?page=2">Previous`) {
		t.Errorf("past the last page, Previous is not the last:\n%s", w.Body)
	}
	if body := get(h, "ed", "/admin/blog/Post/?page=2").Body.String(); strings.Contains(body, "Next") || !strings.Contains(body, `href="/admin/blog/Post/?page=1">Previous`) {
		t.Errorf("the last page:\n%s", body)
	}
	fifty := rows("blog", "Fifty", 50, fields[:1], func(i int) []any { return []any{"x"} }, nil)
	empty := rows("blog", "Empty", 0, fields[:1], nil, nil)
	h2 := handler(t, Config{Models: [][]Model{{fifty, empty}}}, "", map[string][]string{"ed": {"*"}})
	if body := get(h2, "ed", "/admin/blog/Fifty/").Body.String(); !strings.Contains(body, "Page 1 of 1") || strings.Contains(body, "Next") {
		t.Errorf("50 rows:\n%s", body)
	}
	if body := get(h2, "ed", "/admin/blog/Empty/").Body.String(); !strings.Contains(body, "Page 1 of 1") || !strings.Contains(body, "No rows.") {
		t.Errorf("no rows:\n%s", body)
	}
	if w := get(h, "ed", "/admin/blog/Post/?page=x"); w.Code != http.StatusBadRequest {
		t.Errorf("page=x: %d", w.Code)
	}

	odd := rows("blog", "Odd", 1, []Field{{Name: "X", Label: "X"}}, func(int) []any { return []any{3} }, nil)
	h = handler(t, Config{Models: [][]Model{{odd}}}, "", map[string][]string{"ed": {"*"}})
	if w := get(h, "ed", "/admin/blog/Odd/"); w.Code != http.StatusInternalServerError {
		t.Errorf("a cell of an int: %d", w.Code)
	}
}

// A field named Password without admin=hidden is a column like any other
// (D-49, M6's condition 2: nothing is hidden by its name).
func TestPassword(t *testing.T) {
	user := rows("auth", "User", 1, []Field{{Name: "Email", Label: "Email"}, {Name: "Password", Label: "Password"}},
		func(int) []any { return []any{"a@b.c", "hunter2"} }, nil)
	h := handler(t, Config{Models: [][]Model{{user}}}, "", map[string][]string{"ed": {"*"}})
	if body := get(h, "ed", "/admin/auth/User/").Body.String(); !strings.Contains(body, "<th>Password</th>") || !strings.Contains(body, "<td>hunter2</td>") {
		t.Errorf("no Password:\n%s", body)
	}
}

// App refuses a Config that cannot work.
func TestConfig(t *testing.T) {
	can := func(context.Context, string, string, string) (bool, error) { return true, nil }
	for _, c := range []struct {
		cfg  Config
		want string
	}{
		{Config{Models: [][]Model{{plain("blog", "Post")}}}, "admin: Config.Can is nil; set it to the copied auth's Can"},
		{Config{Models: [][]Model{{plain("blog", "Post")}, {plain("blog", "Post")}}, Can: can}, "admin: model blog.Post is in Config.Models twice"},
		{Config{Models: [][]Model{{{App: "blog", Name: "Post", List: []string{"Nope"}}}}, Can: can}, "admin: blog.Post lists Nope, which is not among its Fields; run maago gen"},
		{Config{Models: [][]Model{{}, nil}, Can: can}, "admin: Config.Models has no model; add each app's Admin, such as board.Admin (a model with admin=hidden is not there)"},
	} {
		func() {
			defer func() {
				if got := fmt.Sprint(recover()); got != c.want {
					t.Errorf("panic %q, want %q", got, c.want)
				}
			}()
			App(c.cfg)
		}()
	}
}

// The links are under Config.Prefix, once (D-122).
func TestPrefix(t *testing.T) {
	h := handler(t, Config{Models: [][]Model{{rows("blog", "Post", 60, []Field{{Name: "ID", Label: "ID"}}, func(i int) []any { return []any{int64(i)} }, nil)}}},
		"/app", map[string][]string{"ed": {"*"}})
	if body := get(h, "ed", "/admin/").Body.String(); !strings.Contains(body, `href="/app/admin/blog/Post/"`) || !strings.Contains(body, `href="/app/admin/"`) {
		t.Errorf("index:\n%s", body)
	}
	if body := get(h, "ed", "/admin/blog/Post/").Body.String(); !strings.Contains(body, `href="/app/admin/blog/Post/?page=2"`) {
		t.Errorf("list:\n%s", body)
	}
}

// An error of Can answers 500 on every page that asks it, and OnError
// hears it.
func TestCanFails(t *testing.T) {
	p := &post{}
	down := errors.New("database down")
	cfg := Config{Models: [][]Model{{p.model(true, true)}}, Can: func(context.Context, string, string, string) (bool, error) { return false, down }}
	var heard []error
	h, err := maago.Handler(maago.Config{DB: sql.OpenDB(noDriver{}), Apps: []maago.App{App(cfg)}, LoginURL: "/login",
		Auth:    func(*http.Request) (any, error) { return "root", nil },
		OnError: func(_ context.Context, _ *http.Request, err error) { heard = append(heard, err) }})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/admin/", "/admin/blog/Post/", "/admin/blog/Post/7/", "/admin/blog/Post/add/"} {
		heard = nil
		if w := get(h, "root", target); w.Code != http.StatusInternalServerError || len(heard) != 1 || !errors.Is(heard[0], down) {
			t.Errorf("%s: %d, heard %v", target, w.Code, heard)
		}
	}
}
