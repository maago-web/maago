package site

import (
	"bufio"
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maago-web/maago"
	_ "modernc.org/sqlite"
)

// handler serves the app on a new database with the project's migration.
func handler(t *testing.T) (http.Handler, *sql.DB, *[]error) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ddl, err := os.ReadFile("../migrations/0001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(ddl)); err != nil {
		t.Fatal(err)
	}
	var reported []error
	h, err := maago.Handler(maago.Config{DB: db, Apps: []maago.App{App}, Auth: Auth, LoginURL: "/login",
		OnError: func(_ context.Context, _ *http.Request, err error) { reported = append(reported, err) }})
	if err != nil {
		t.Fatal(err)
	}
	return h, db, &reported
}

func TestRequests(t *testing.T) {
	h, db, reported := handler(t)
	p, err := InsertPost(context.Background(), db, NewPostInput{Title: "hello <b>"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		method, path string
		code         int
		body         string // what the body holds
	}{
		{"GET", PostDetailURL(p.ID), 200, "<h1>hello &lt;b&gt;</h1>"},
		{"GET", "/posts/abc", 404, "Not Found"},
		{"GET", PostDetailURL(p.ID + 1), 404, "Not Found"},       // sql.ErrNoRows
		{"GET", PageURL(7), 200, "<p>7 at 2026-10-01 00:30</p>"}, // UTC: no Config.TimeZone
		{"GET", "/pages/99999999999", 404, "Not Found"},          // past int32
		{"GET", "/list?page=2", 200, "page 2, draft unset"},
		{"GET", "/list?page=2&draft=true", 200, "page 2, draft yes"},
		{"GET", "/list?page=abc", 400, "Bad Request"},
		{"GET", "/list", 400, "Bad Request"},               // page is required
		{"GET", "/list?page=1&page=2", 400, "Bad Request"}, // twice
		{"GET", "/list?page=1&draft", 400, "Bad Request"},  // a bool needs a value
		{"GET", "/list?page=1&x=%zz", 400, "Bad Request"},  // malformed
		{"GET", "/list?page=1&other=1", 200, "page 1"},     // keys of no field are ignored
		{"GET", "/list/rows?page=3", 200, "page 3"},
		{"GET", ApiPostURL(p.ID), 200, `"Title":"hello \u003cb\u003e"`},
		{"GET", ApiPostURL(p.ID + 1), 404, `"status":404`},
		{"GET", "/broken", 500, "Internal Server Error"},
		{"GET", PageURL(7), 200, strings.Join([]string{PostDetailURL(7), PageURL(7), PostListURL(), PostRowsURL(), PostCreateURL(), ApiPostURL(7), BrokenURL(), PostExportURL("csv")}, " ")}, // {{url}} is XxxURL
		{"GET", PostDetailURL(p.ID), 200, "<!doctype html>"}, // through base.html
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(c.method, c.path, nil))
		if w.Code != c.code || !strings.Contains(w.Body.String(), c.body) {
			t.Errorf("%s %s: got %d %q, want %d with %q", c.method, c.path, w.Code, w.Body, c.code, c.body)
		}
		if c.path == "/list/rows?page=3" && w.Body.String() != "<p>page 3, draft unset</p>" {
			t.Errorf("a block route sent more than the block: %q", w.Body)
		}
		if c.path == "/broken" && strings.Contains(w.Body.String(), "before") {
			t.Errorf("a failed render wrote part of the page: %q", w.Body)
		}
	}
	if len(*reported) != 1 {
		t.Errorf("OnError heard %v, want the broken render once", *reported)
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", PostCreateURL(), nil))
	if w.Code != 303 || !strings.HasPrefix(w.Header().Get("Location"), "/posts/") {
		t.Errorf("POST: %d, Location %q", w.Code, w.Header().Get("Location"))
	}
}

// TestForm posts the form of D-119: each field's three results, the text
// sent kept and escaped, a form error, a check the view makes with the
// database, and what is not a form at all.
func TestForm(t *testing.T) {
	h, db, _ := handler(t)
	if _, err := InsertPost(context.Background(), db, NewPostInput{Title: "old"}); err != nil {
		t.Fatal(err)
	}
	send := func(method, ct, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, PostNewURL()+"?title=fromquery", strings.NewReader(body))
		if ct != "" {
			r.Header.Set("Content-Type", ct)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	const form = "application/x-www-form-urlencoded"
	for _, c := range []struct {
		name, method, ct, body string
		code                   int
		has, hasNot            []string
	}{
		{"drawn empty", "GET", "", "", 200, []string{`<label for="title">제목</label><input name="title" value="">`}, []string{"error", "fromquery"}},
		{"count empty", "POST", form, "title=a&count=", 200, []string{`<input name="count" value=""><p class="error">This field is required.</p>`}, nil},
		{"count not a number", "POST", form, "title=a&count=abc", 200, []string{`<input name="count" value="abc"><p class="error">Enter a number.</p>`}, nil},
		{"count past int64", "POST", form, "title=a&count=99999999999999999999", 200,
			[]string{`<input name="count" value="99999999999999999999"><p class="error">Enter 9223372036854775807 or less.</p>`}, nil},
		{"count past max_value", "POST", form, "title=a&count=101", 200, []string{`<p class="error">Enter 100 or less.</p>`}, nil},
		{"one field's error keeps the others", "POST", form, "title=" + url.QueryEscape(`<b>&"`) + "&count=abc&agree=on", 200,
			[]string{`value="&lt;b&gt;&amp;&#34;"`, `<input type="checkbox" name="agree" checked>`}, []string{"<b>"}},
		{"title too long", "POST", form, "title=" + strings.Repeat("가", 11) + "&count=1", 200, []string{"Enter at most 10 characters."}, nil},
		{"form error", "POST", form, "title=forbidden&count=1", 200, []string{`<p class="form-error">that title is forbidden</p>`}, nil},
		{"a field error Validate sets", "POST", form, "title=nope&count=1", 200, []string{`<p class="error">nope is not a title</p>`}, nil},
		{"no Validate while a field is wrong", "POST", form, "title=forbidden&count=abc", 200, []string{"Enter a number."}, []string{"forbidden</p>"}},
		{"taken, as the view finds", "POST", form, "title=old&count=1", 200, []string{`value="old"><p class="error">that title is taken</p>`}, nil},
		{"not a form body", "POST", "text/plain", "title=a&count=1", 400, nil, nil},
		{"a key twice", "POST", form, "title=a&title=b&count=1", 400, nil, nil},
		{"saved", "POST", form, "title=new&count=1", 303, nil, nil},
	} {
		w := send(c.method, c.ct, c.body)
		body := w.Body.String()
		if w.Code != c.code {
			t.Errorf("%s: %d, want %d: %s", c.name, w.Code, c.code, body)
		}
		for _, s := range c.has {
			if !strings.Contains(body, s) {
				t.Errorf("%s: no %q in %s", c.name, s, body)
			}
		}
		for _, s := range c.hasNot {
			if strings.Contains(body, s) {
				t.Errorf("%s: %q in %s", c.name, s, body)
			}
		}
	}
}

// Config.Messages replaces what forms say; what it leaves empty stays.
func TestFormMessages(t *testing.T) {
	_, db, _ := handler(t)
	h, err := maago.Handler(maago.Config{DB: db, Apps: []maago.App{App}, Auth: Auth, LoginURL: "/login", Messages: maago.Messages{Required: "필수 항목입니다."}})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", PostNewURL(), strings.NewReader("title=&count=x"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if body := w.Body.String(); !strings.Contains(body, "필수 항목입니다.") || !strings.Contains(body, "Enter a number.") {
		t.Errorf("messages: %s", body)
	}
	r = httptest.NewRequest("POST", ApiPostCreateURL(), strings.NewReader(`{"count":1}`))
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if body := w.Body.String(); !strings.Contains(body, `"detail":"필수 항목입니다."`) {
		t.Errorf("messages in a 422: %s", body)
	}
}

// TestAPIForm posts the form as JSON (D-120): a 422 with the errors in
// order and the view not called, 400 for what is no JSON object, a 422
// the view returns, and the created post.
func TestAPIForm(t *testing.T) {
	h, db, _ := handler(t)
	if _, err := InsertPost(context.Background(), db, NewPostInput{Title: "old"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, body string
		code       int
		answer     string // what the answer holds
		calls      int    // of the view
	}{
		{"count empty", `{"title":"a","count":null}`, 422, `"errors":[{"field":"count","detail":"This field is required."}]`, 0},
		{"count not a number", `{"title":"a","count":"abc"}`, 422, `"errors":[{"field":"count","detail":"Enter a number."}]`, 0},
		{"count past int64", `{"title":"a","count":99999999999999999999}`, 422, `"errors":[{"field":"count","detail":"Enter 9223372036854775807 or less."}]`, 0},
		{"two fields, in order", `{"count":"x"}`, 422, `"errors":[{"field":"title","detail":"This field is required."},{"field":"count","detail":"Enter a number."}]`, 0},
		{"Validate", `{"title":"forbidden","count":1}`, 422, `"errors":[{"detail":"that title is forbidden"}]`, 0},
		{"broken JSON", `{"title":`, 400, `"status":400`, 0},
		{"taken, as the view finds", `{"title":"old","count":1}`, 422, `"errors":[{"field":"title","detail":"that title is taken"}]`, 1},
		{"created", `{"title":"new","count":"1","agree":true}`, 200, `"Title":"new"`, 1},
	} {
		before := apiCalls
		r := httptest.NewRequest("POST", ApiPostCreateURL(), strings.NewReader(c.body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.code || !strings.Contains(w.Body.String(), c.answer) || apiCalls-before != c.calls {
			t.Errorf("%s: %d %s, view called %d times; want %d with %s, %d calls", c.name, w.Code, w.Body, apiCalls-before, c.code, c.answer, c.calls)
		}
		if c.code >= 400 && w.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("%s: Content-Type %q", c.name, w.Header().Get("Content-Type"))
		}
	}
}

// TestExportStreams shows an escape view streams (D-41): through a real
// server, the first row arrives while the view still holds the second.
// Everything is bounded, so a broken stream fails rather than hangs.
func TestExportStreams(t *testing.T) {
	h, _, reported := handler(t)
	srv := httptest.NewServer(h)
	gate := make(chan struct{})
	exportGate = gate
	open := sync.OnceFunc(func() { close(gate) })
	t.Cleanup(func() { open(); srv.Close(); exportGate = nil }) // the gate first: Close waits for the view
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	get := func(path string) (*http.Response, *bufio.Reader) {
		req, err := http.NewRequestWithContext(ctx, "GET", srv.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp, bufio.NewReader(resp.Body)
	}

	resp, body := get(PostExportURL("csv"))
	first, err := body.ReadString('\n')
	if err != nil || first != "format,csv\n" {
		t.Fatalf("first row before the second is written: %q %v", first, err)
	}
	open() // only now may the view write the second row
	rest, err := io.ReadAll(body)
	if err != nil || string(rest) != "posts,0\n" {
		t.Errorf("rest: %q %v", rest, err)
	}
	if ct, cd := resp.Header.Get("Content-Type"), resp.Header.Get("Content-Disposition"); ct != "text/csv" || cd != `attachment; filename="posts.csv"` {
		t.Errorf("headers: %q %q", ct, cd)
	}

	// A panic after the first row: the status stays, nothing is added, and
	// OnError hears it.
	resp, body = get(PostExportURL("panic"))
	all, _ := io.ReadAll(body)
	if resp.StatusCode != 200 || string(all) != "format,panic\n" {
		t.Errorf("panic after a row: %d %q", resp.StatusCode, all)
	}
	if len(*reported) != 1 || !strings.HasPrefix((*reported)[0].Error(), "panic: export failed") {
		t.Errorf("OnError heard %v", *reported)
	}
}

// TestLogin: maago.LoginRequired in the route table, with the fake Auth
// (D-121).
func TestLogin(t *testing.T) {
	h, _, _ := handler(t)
	for _, c := range []struct {
		name, path, user string
		code             int
		has, location    string
	}{
		{"page, nobody", SecretURL() + "?x=1", "", 303, "", "/login?next=%2Fsecret%3Fx%3D1"},
		{"API, nobody", ApiSecretURL(), "", 401, `"status":401`, ""},
		{"page, ann", SecretURL(), "ann", 200, "<p>secret for ann</p>", ""},
		{"API, ann", ApiSecretURL(), "ann", 200, `{"Name":"ann"}`, ""},
	} {
		r := httptest.NewRequest("GET", c.path, nil)
		if c.user != "" {
			r.Header.Set("X-User", c.user)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.code || !strings.Contains(w.Body.String(), c.has) || w.Header().Get("Location") != c.location {
			t.Errorf("%s: %d %q Location %q", c.name, w.Code, w.Body, w.Header().Get("Location"))
		}
	}
}

// TestPrefix serves the app under /app (D-122) behind http.StripPrefix, as
// a proxy that keeps /app needs: every path the app makes has /app once,
// and requests to those paths reach their routes.
func TestPrefix(t *testing.T) {
	_, db, _ := handler(t)
	seoul, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatal(err)
	}
	h, err := maago.Handler(maago.Config{DB: db, Apps: []maago.App{App}, Auth: Auth, LoginURL: "/app/login", Prefix: "/app", TimeZone: seoul})
	if err != nil {
		t.Fatal(err)
	}
	if got := PostDetailURL(7); got != "/app/posts/7" {
		t.Errorf("PostDetailURL: %q", got)
	}
	sh := http.StripPrefix("/app", h) // what main does when the proxy keeps /app
	do := func(method, path, ct, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if ct != "" {
			r.Header.Set("Content-Type", ct)
		}
		w := httptest.NewRecorder()
		sh.ServeHTTP(w, r)
		return w
	}
	page := do("GET", PageURL(7), "", "").Body.String()
	for _, s := range []string{"<p>7 at 2026-10-01 09:30</p>", "/app/posts/7 /app/pages/7 /app/list /app/list/rows /app/posts /app/api/posts/7 /app/broken /app/export/csv"} {
		if !strings.Contains(page, s) {
			t.Errorf("no %q in %s", s, page)
		}
	}
	if loc := do("GET", SecretURL(), "", "").Header().Get("Location"); loc != "/app/login?next=%2Fapp%2Fsecret" {
		t.Errorf("login: %q", loc)
	}
	w := do("POST", PostNewSaveURL(), "application/x-www-form-urlencoded", "title=prefixed&count=1")
	loc := w.Header().Get("Location")
	if w.Code != 303 || !strings.HasPrefix(loc, "/app/posts/") {
		t.Fatalf("SeeOther: %d %q", w.Code, loc)
	}
	if got := do("GET", loc, "", ""); got.Code != 200 || !strings.Contains(got.Body.String(), "prefixed") { // a doubled prefix would be a 404
		t.Errorf("following SeeOther: %d %s", got.Code, got.Body)
	}
}
