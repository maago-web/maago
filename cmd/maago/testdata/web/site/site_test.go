package site

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
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
