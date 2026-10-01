package site

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	h, err := maago.Handler(maago.Config{DB: db, Apps: []maago.App{App},
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
		{"GET", PostDetailURL(p.ID + 1), 404, "Not Found"}, // sql.ErrNoRows
		{"GET", PageURL(7), 200, "<p>7</p>"},
		{"GET", "/pages/99999999999", 404, "Not Found"}, // past int32
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
		{"GET", PageURL(7), 200, strings.Join([]string{PostDetailURL(7), PageURL(7), PostListURL(), PostRowsURL(), PostCreateURL(), ApiPostURL(7), BrokenURL()}, " ")}, // {{url}} is XxxURL
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
	h, err := maago.Handler(maago.Config{DB: db, Apps: []maago.App{App}, Messages: maago.Messages{Required: "필수 항목입니다."}})
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
}
