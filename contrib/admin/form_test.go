package admin

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/maago-web/maago"
)

// The helpers of admin_gen.go: the kind of each input, and a value as its
// input draws it and reads it back.
func TestInputs(t *testing.T) {
	str := maago.Field[string]{Name: "title", Label: "Title", Raw: "x", Error: "bad"}
	choice := maago.Field[string]{Name: "status", Choices: []string{"draft", "published"}}
	for _, c := range []struct {
		in   Input
		kind string
	}{
		{In(&str, true, ""), "text"}, {In(&choice, true, ""), "select"},
		{In(&maago.Field[int64]{}, true, "blog.Author"), "integer"}, {In(&maago.Field[int32]{}, true, ""), "integer"},
		{In(&maago.Field[float64]{}, false, ""), "number"}, {In(&maago.Field[bool]{}, false, ""), "checkbox"},
		{In(&maago.Field[time.Time]{}, false, ""), "datetime"}, {Show("Slug", "s"), "readonly"},
	} {
		if c.in.Kind != c.kind {
			t.Errorf("%+v: kind %s, want %s", c.in, c.in.Kind, c.kind)
		}
	}
	if in := In(&str, true, "blog.Author"); in.Name != "title" || in.Label != "Title" || in.Raw != "x" || in.Error != "bad" || !in.Required || in.FK != "blog.Author" {
		t.Errorf("In: %+v", in)
	}
	if in := Show("Body", []byte("ab")); in.Raw != "(2 bytes)" || in.Label != "Body" {
		t.Errorf("Show: %+v", in)
	}

	at := time.Date(2026, 10, 13, 9, 30, 15, 123456000, time.UTC)
	yes, s := true, "s"
	for _, c := range []struct {
		v    any
		want string
	}{
		{"s", "s"}, {int64(-3), "-3"}, {int32(7), "7"}, {1e6, "1000000"}, {0.1, "0.1"}, {true, "on"}, {false, ""},
		{at, "2026-10-13T09:30:15.123456"}, {&s, "s"}, {(*string)(nil), ""}, {&at, "2026-10-13T09:30:15.123456"},
		{&yes, "yes"}, {new(bool), "no"}, {(*bool)(nil), ""}, {(*int64)(nil), ""},
	} {
		if got := Raw(c.v); got != c.want {
			t.Errorf("Raw(%#v) = %q, want %q", c.v, got, c.want)
		}
	}

	// What Raw draws, BindTime reads back to the microsecond.
	var tf maago.Field[time.Time]
	maago.BindTime(context.Background(), url.Values{"t": {Raw(at)}}, &tf, maago.Rule{Name: "t"})
	if !tf.Value.Equal(at) {
		t.Errorf("time round trip: %v, want %v", tf.Value, at)
	}

	if p := Ptr(maago.Field[string]{}); p != nil {
		t.Errorf("Ptr of nothing: %v", *p)
	}
	if p := Ptr(maago.Field[string]{Value: "", Given: true, Raw: ""}); p == nil {
		t.Error("Ptr of a given value is nil")
	}
	for raw, want := range map[string]string{"": "nil", "yes": "true", "no": "false"} {
		got := "nil"
		if b := Bool(maago.Field[string]{Value: raw, Given: raw != ""}); b != nil {
			got = map[bool]string{true: "true", false: "false"}[*b]
		}
		if got != want {
			t.Errorf("Bool(%q) = %s, want %s", raw, got, want)
		}
	}
}

// post is a form of one title, and an fk field to blog.Author: it saves
// when the title is given, and records the ids and values it was given.
type post struct {
	got   []string
	saved map[string]string
}

func (p *post) model(add, edit bool) Model {
	m := plain("blog", "Post")
	m.Add, m.Edit = add, edit
	m.Form = func(ctx context.Context, _ maago.DB, id int64, vals url.Values) (Form, error) {
		if id == 404 {
			return Form{}, sql.ErrNoRows
		}
		var title, status maago.Field[string]
		var author maago.Field[int64]
		var public maago.Field[bool]
		var at maago.Field[time.Time]
		maago.BindString(ctx, vals, &title, maago.Rule{Name: "title", Label: "Title", Max: 5})
		maago.BindInt64(ctx, vals, &author, maago.Rule{Name: "author_id", Label: "Author", Optional: true})
		maago.BindString(ctx, vals, &status, maago.Rule{Name: "status", Label: "Status", Choices: []string{"draft", "published"}, Optional: true})
		maago.BindBool(ctx, vals, &public, maago.Rule{Name: "public", Label: "Public"})
		maago.BindTime(ctx, vals, &at, maago.Rule{Name: "at", Label: "At", Optional: true})
		if vals == nil && id != 0 {
			title.Raw, author.Raw, status.Raw, public.Raw = "old", "3", "published", "on"
			at.Raw = Raw(time.Date(2026, 10, 13, 9, 30, 15, 123456000, time.UTC))
		}
		p.got = append(p.got, strings.TrimSpace(Raw(id)+" "+vals.Encode()))
		var in []Input
		if id != 0 {
			in = append(in, Show("ID", id))
		}
		in = append(in, In(&title, true, ""), In(&author, false, "blog.Author"), In(&status, true, ""), In(&public, false, ""), In(&at, false, ""))
		if vals == nil || title.Error != "" {
			return Form{Inputs: in}, nil
		}
		p.saved = map[string]string{"title": title.Value}
		return Form{Inputs: in, Saved: true}, nil
	}
	return m
}

func send(h http.Handler, user, target string, form url.Values, header ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", target, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("X-User", user)
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// Adding and changing a row: what Can is asked, the form drawn and read
// again with its errors, a valid one saved and sent to the list, and a
// user without Change reads only (D-131, D-133).
func TestForms(t *testing.T) {
	p := &post{}
	var asked []string
	author := plain("blog", "Author")
	cfg := Config{Models: [][]Model{{p.model(true, true), author}},
		Can: func(ctx context.Context, app, model, action string) (bool, error) {
			u, _ := maago.UserFrom[string](ctx)
			asked = append(asked, u+" "+app+"."+model+" "+action)
			return u == "root" || u == "viewer" && action == View && model == "Post", nil
		}}
	h := handler(t, cfg, "", nil)

	body := get(h, "root", "/admin/blog/Post/add/").Body.String()
	if !slices.Equal(p.got, []string{"0"}) || !strings.Contains(body, `<input type="text" id="title" name="title" value="" required>`) ||
		!strings.Contains(body, `<button type="submit">Save</button>`) || strings.Contains(body, ">ID<") {
		t.Errorf("add GET: %q\n%s", p.got, body)
	}
	if !slices.Contains(asked, "root blog.Post add") {
		t.Errorf("Can was asked %q", asked)
	}

	p.got = nil
	w := send(h, "root", "/admin/blog/Post/add/", url.Values{"title": {`"><b>x`}, "author_id": {"x"}})
	if body := w.Body.String(); w.Code != 200 || !strings.Contains(body, `value="&#34;&gt;&lt;b&gt;x"`) || strings.Contains(body, "<b>") || !strings.Contains(body, "Enter at most 5 characters.") ||
		!strings.Contains(body, "Enter a number.") || p.saved != nil {
		t.Errorf("add with errors: %d\n%s", w.Code, body)
	}
	if w := send(h, "root", "/admin/blog/Post/add/", url.Values{"title": {"hi", "evil"}}); w.Code != http.StatusBadRequest || p.saved != nil {
		t.Errorf("a key sent twice: %d %v", w.Code, p.saved)
	}
	w = send(h, "root", "/admin/blog/Post/add/", url.Values{"title": {"hi"}})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/admin/blog/Post/" || p.saved["title"] != "hi" {
		t.Errorf("add: %d %q %v", w.Code, w.Header().Get("Location"), p.saved)
	}

	asked, p.saved = nil, nil
	body = get(h, "root", "/admin/blog/Post/7/").Body.String()
	for _, want := range []string{`<option>draft</option><option selected>published</option>`,
		`<input type="checkbox" id="public" name="public" checked>`,
		`<input type="text" id="at" name="at" value="2026-10-13T09:30:15.123456" placeholder="2006-01-02T15:04:05">`,
		`<input type="number" id="author_id" name="author_id" value="3" step="1">`} {
		if !strings.Contains(body, want) {
			t.Errorf("row GET lacks %s:\n%s", want, body)
		}
	}
	emptyChoice := `<select id="status" name="status" required>` + "\n" + `<option value=""></option>`
	if strings.Contains(body, emptyChoice) {
		t.Errorf("a required select with a value has an empty choice:\n%s", body)
	}
	if add := get(h, "root", "/admin/blog/Post/add/").Body.String(); !strings.Contains(add, emptyChoice) {
		t.Errorf("a required select with no value has no empty choice:\n%s", add)
	}
	if !strings.Contains(body, `value="old"`) || !strings.Contains(body, `<span class="label">ID</span> <span>7</span>`) ||
		!strings.Contains(body, `<a href="/admin/blog/Author/3/">blog.Author 3</a>`) || !strings.Contains(body, "Save") {
		t.Errorf("row GET:\n%s", body)
	}
	w = send(h, "root", "/admin/blog/Post/7/", url.Values{"title": {"new"}})
	if w.Code != http.StatusSeeOther || p.saved["title"] != "new" || !slices.Contains(asked, "root blog.Post change") {
		t.Errorf("row POST: %d %v, asked %q", w.Code, p.saved, asked)
	}

	// The viewer may view Post, but not change it, nor view Author.
	body = get(h, "viewer", "/admin/blog/Post/7/").Body.String()
	if !strings.Contains(body, `value="old" required disabled>`) || strings.Contains(body, "Save") || strings.Contains(body, "/admin/blog/Author/3/") {
		t.Errorf("viewer's row:\n%s", body)
	}
	for _, c := range []struct {
		user, method, target string
		code                 int
	}{
		{"viewer", "POST", "/admin/blog/Post/7/", 403},
		{"viewer", "GET", "/admin/blog/Post/add/", 403},
		{"viewer", "POST", "/admin/blog/Post/add/", 403},
		{"root", "GET", "/admin/blog/Post/404/", 404},
		{"root", "GET", "/admin/blog/Post/x/", 404},
		{"root", "GET", "/admin/blog/Nope/7/", 404},
		{"viewer", "GET", "/admin/blog/Nope/7/", 403},
	} {
		var w *httptest.ResponseRecorder
		if c.method == "GET" {
			w = get(h, c.user, c.target)
		} else {
			w = send(h, c.user, c.target, url.Values{"title": {"x"}})
		}
		if w.Code != c.code {
			t.Errorf("%s %s %s: %d, want %d", c.user, c.method, c.target, w.Code, c.code)
		}
	}

	// A POST from another origin is refused before the admin sees it.
	p.saved = nil
	w = send(h, "root", "/admin/blog/Post/7/", url.Values{"title": {"evil"}}, "Origin", "https://evil.example", "Sec-Fetch-Site", "cross-site")
	if w.Code != http.StatusForbidden || p.saved != nil {
		t.Errorf("cross-origin POST: %d %v", w.Code, p.saved)
	}

	// The list links each row, and Add.
	if body := get(h, "root", "/admin/blog/Post/").Body.String(); !strings.Contains(body, `<a href="/admin/blog/Post/add/">Add</a>`) || !strings.Contains(body, `<a href="/admin/blog/Post/1/">1</a>`) {
		t.Errorf("list:\n%s", body)
	}
	if body := get(h, "viewer", "/admin/blog/Post/").Body.String(); strings.Contains(body, "Add") {
		t.Errorf("viewer's list offers Add:\n%s", body)
	}
}

// A model without adding has no add page nor link, and one with nothing
// to change is read only (D-132).
func TestNoAddNoEdit(t *testing.T) {
	p := &post{}
	all := func(context.Context, string, string, string) (bool, error) { return true, nil }
	h := handler(t, Config{Models: [][]Model{{p.model(false, false)}}, Can: all}, "", nil)
	if w := get(h, "root", "/admin/blog/Post/add/"); w.Code != 404 {
		t.Errorf("add: %d", w.Code)
	}
	if body := get(h, "root", "/admin/blog/Post/").Body.String(); strings.Contains(body, "Add") {
		t.Errorf("list offers Add:\n%s", body)
	}
	if body := get(h, "root", "/admin/blog/Post/7/").Body.String(); strings.Contains(body, "Save") || !strings.Contains(body, "disabled") {
		t.Errorf("row:\n%s", body)
	}
	if w := send(h, "root", "/admin/blog/Post/7/", url.Values{"title": {"x"}}); w.Code != 403 || p.saved != nil {
		t.Errorf("POST: %d %v", w.Code, p.saved)
	}
}
