package admin

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/maago-web/maago"
)

// txDriver is a database that only begins, commits and rolls back.
type txDriver struct{ commits, rollbacks *int }

func (d txDriver) Connect(context.Context) (driver.Conn, error) { return txConn(d), nil }
func (txDriver) Driver() driver.Driver                          { return nil }

type txConn txDriver

func (txConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("no statements") }
func (txConn) Close() error                        { return nil }
func (c txConn) Begin() (driver.Tx, error)         { return txTx(c), nil }

type txTx txConn

func (t txTx) Commit() error   { *t.commits++; return nil }
func (t txTx) Rollback() error { *t.rollbacks++; return nil }

// Deleting rows, all of them or none (D-135): what refers to each, a row
// that is not there, one transaction, and who may.
func TestDelete(t *testing.T) {
	var commits, rollbacks int
	var deleted []int64
	var inTx []bool
	m := plain("blog", "Post")
	m.Refs = func(_ context.Context, _ maago.DB, id int64) ([]Ref, error) {
		switch id {
		case 2:
			return []Ref{{Model: "blog.Comment", Field: "PostID", N: 3}, {Model: "blog.Tag", Field: "PostID", N: 1}, {Model: "blog.Post", Field: "ParentID", Self: true}}, nil
		case 404:
			return nil, sql.ErrNoRows
		case 9:
			return nil, errors.New("refs down")
		}
		return []Ref{{Model: "blog.Comment", Field: "PostID"}}, nil
	}
	m.Delete = func(_ context.Context, db maago.DB, id int64) error {
		_, tx := db.(*sql.Tx)
		inTx = append(inTx, tx)
		if id == 5 {
			return errors.New("delete down")
		}
		deleted = append(deleted, id)
		return nil
	}
	m.Form = (&post{}).model(true, true).Form
	var asked []string
	cfg := Config{Models: [][]Model{{m}}, Can: func(ctx context.Context, app, model, action string) (bool, error) {
		u, _ := maago.UserFrom[string](ctx)
		asked = append(asked, action)
		return u == "root" || u == "viewer" && action == View, nil
	}}
	h := handlerOn(t, sql.OpenDB(txDriver{&commits, &rollbacks}), cfg, "", nil)
	ids := func(ns ...int) url.Values {
		v := url.Values{}
		for _, n := range ns {
			v.Add("id", strconv.Itoa(n))
		}
		return v
	}

	body := get(h, "root", "/admin/blog/Post/delete/?id=1&id=2").Body.String()
	if !strings.Contains(body, "3 rows of blog.Comment (PostID) refer to it") || !strings.Contains(body, "1 row of blog.Tag (PostID) refers to it") || strings.Contains(body, "blog.Post (ParentID)") ||
		strings.Contains(body, "<button") || !strings.Contains(body, "Nothing is deleted") || !slices.Contains(asked, Delete) {
		t.Errorf("GET of a row others refer to:\n%s", body)
	}
	body = get(h, "root", "/admin/blog/Post/delete/?id=1&id=1").Body.String()
	if strings.Count(body, `<input type="hidden" name="id" value="1">`) != 1 || !strings.Contains(body, `<button type="submit">Delete</button>`) {
		t.Errorf("GET of a row twice:\n%s", body)
	}
	if body := get(h, "root", "/admin/blog/Post/delete/?id=404").Body.String(); !strings.Contains(body, "no such row") || strings.Contains(body, "<button") {
		t.Errorf("GET of no row:\n%s", body)
	}

	if w := send(h, "root", "/admin/blog/Post/delete/", ids(1, 2)); w.Code != 200 || deleted != nil || commits != 0 {
		t.Errorf("POST with a row others refer to: %d, deleted %v", w.Code, deleted)
	}
	if w := send(h, "root", "/admin/blog/Post/delete/", ids(1, 404)); w.Code != 200 || deleted != nil {
		t.Errorf("POST with no row: %d, deleted %v", w.Code, deleted)
	}
	if rollbacks != 2 || commits != 0 { // the POSTs above counted in a transaction, and deleted nothing
		t.Errorf("commits %d, rollbacks %d", commits, rollbacks)
	}
	rollbacks = 0
	if w := send(h, "root", "/admin/blog/Post/delete/", ids(3, 5)); w.Code != 500 || commits != 0 || rollbacks != 1 {
		t.Errorf("a delete that fails: %d, commits %d, rollbacks %d", w.Code, commits, rollbacks)
	}
	deleted, inTx = nil, nil
	if w := send(h, "root", "/admin/blog/Post/delete/", ids(1, 3)); w.Code != 303 || w.Header().Get("Location") != "/admin/blog/Post/" ||
		!slices.Equal(deleted, []int64{1, 3}) || !slices.Equal(inTx, []bool{true, true}) || commits != 1 {
		t.Errorf("POST: %d %q, deleted %v in a tx %v, commits %d", w.Code, w.Header().Get("Location"), deleted, inTx, commits)
	}

	deleted = nil
	for _, c := range []struct {
		user, method, target string
		form                 url.Values
		code                 int
	}{
		{"root", "GET", "/admin/blog/Post/delete/", nil, 303}, // Delete selected with nothing selected: back to the list
		{"root", "POST", "/admin/blog/Post/delete/", url.Values{}, 400},
		{"root", "GET", "/admin/blog/Post/delete/?id=x", nil, 400},
		{"root", "GET", "/admin/blog/Post/delete/?id=9", nil, 500},
		{"viewer", "GET", "/admin/blog/Post/delete/?id=1", nil, 403},
		{"viewer", "POST", "/admin/blog/Post/delete/", ids(1), 403},
		{"viewer", "GET", "/admin/blog/Nope/delete/?id=1", nil, 403},
		{"root", "GET", "/admin/blog/Nope/delete/?id=1", nil, 404},
	} {
		var code int
		if c.method == "GET" {
			code = get(h, c.user, c.target).Code
		} else {
			code = send(h, c.user, c.target, c.form).Code
		}
		if code != c.code {
			t.Errorf("%s %s %s: %d, want %d", c.user, c.method, c.target, code, c.code)
		}
	}
	many := make([]int, pageSize+1)
	for i := range many {
		many[i] = i + 1
	}
	if w := send(h, "root", "/admin/blog/Post/delete/", ids(many...)); w.Code != 400 || deleted != nil {
		t.Errorf("%d rows: %d", len(many), w.Code)
	}
	if w := send(h, "root", "/admin/blog/Post/delete/", ids(1), "Origin", "https://evil.example", "Sec-Fetch-Site", "cross-site"); w.Code != 403 || deleted != nil {
		t.Errorf("cross-origin POST: %d %v", w.Code, deleted)
	}

	// The list's boxes and the row's link, for whoever may delete only.
	if body := get(h, "root", "/admin/blog/Post/").Body.String(); !strings.Contains(body, `<form method="get" action="/admin/blog/Post/delete/">`) ||
		!strings.Contains(body, `<input type="checkbox" name="id" value="1">`) || !strings.Contains(body, "Delete selected") {
		t.Errorf("root's list:\n%s", body)
	}
	if body := get(h, "viewer", "/admin/blog/Post/").Body.String(); strings.Contains(body, "checkbox") || strings.Contains(body, "Delete") {
		t.Errorf("viewer's list:\n%s", body)
	}
	if body := get(h, "root", "/admin/blog/Post/7/").Body.String(); !strings.Contains(body, `<a href="/admin/blog/Post/delete/?id=7">Delete</a>`) {
		t.Errorf("root's row:\n%s", body)
	}
	if body := get(h, "viewer", "/admin/blog/Post/7/").Body.String(); strings.Contains(body, "Delete") {
		t.Errorf("viewer's row:\n%s", body)
	}
}
