package maago

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// code is the status err answers with, as Handler decides it.
func code(err error) int {
	if err == nil {
		return 0
	}
	return status(err)
}

func TestQuery(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?n=7&s=&two=1&two=2&b&f=NaN&e=", nil)
	q, err := ParseQuery(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name     string
		got      func() (any, bool, error)
		want     any
		present  bool
		wantCode int
	}{
		{"int", func() (any, bool, error) { return Query(q, "n", ParseInt64, true) }, int64(7), true, 0},
		{"missing, optional", func() (any, bool, error) { return Query(q, "none", ParseInt64, false) }, int64(0), false, 0},
		{"missing, required", func() (any, bool, error) { return Query(q, "none", ParseInt64, true) }, int64(0), false, 400},
		{"empty string", func() (any, bool, error) { return Query(q, "s", ParseString, true) }, "", true, 0},
		{"empty number", func() (any, bool, error) { return Query(q, "e", ParseInt32, true) }, int32(0), false, 400},
		{"twice", func() (any, bool, error) { return Query(q, "two", ParseString, true) }, "", false, 400},
		{"bool without a value", func() (any, bool, error) { return Query(q, "b", ParseBool, true) }, false, false, 400},
		{"NaN", func() (any, bool, error) { return Query(q, "f", ParseFloat64, true) }, float64(0), false, 400},
	} {
		v, present, err := c.got()
		if v != c.want || present != c.present || code(err) != c.wantCode {
			t.Errorf("%s: got %v %v %v", c.name, v, present, err)
		}
	}
	if _, err := ParseQuery(httptest.NewRequest("GET", "/x?a=%zz", nil)); code(err) != 400 {
		t.Errorf("malformed query: %v", err)
	}
}

func TestPath(t *testing.T) {
	mux := http.NewServeMux()
	var got []any
	mux.HandleFunc("GET /a/{n}/{s}", func(w http.ResponseWriter, r *http.Request) {
		n, err := Path(r, "n", ParseInt32)
		s, _ := Path(r, "s", ParseString)
		got = append(got, n, s, code(err))
	})
	for _, p := range []string{"/a/7/x", "/a/99999999999/x", "/a/" + PathSegment("1") + "/" + PathSegment("a/b %?#")} {
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", p, nil))
	}
	want := []any{int32(7), "x", 0, int32(0), "x", 404, int32(1), "a/b %?#", 0}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
			break
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("PathSegment(\"\") did not panic")
		}
	}()
	PathSegment("")
}

func TestRender(t *testing.T) {
	type page struct {
		Title string
		Post  *struct{ X int }
	}
	render := func(fsys fstest.MapFS, block string, data page) (string, string, error) {
		sets, err := ParsePages(fsys, nil, "p.html")
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		err = Render(w, sets, "p.html", block, data)
		return w.Body.String(), w.Header().Get("Content-Type"), err
	}
	alone := fstest.MapFS{"templates/p.html": {Data: []byte(`{{.Title}}`)}}
	// No tag in the page: Content-Type must be set, not sniffed.
	if body, ct, err := render(alone, "", page{Title: "<t>"}); err != nil || body != "&lt;t&gt;" || ct != "text/html; charset=utf-8" {
		t.Errorf("page alone: %v %q %q", err, body, ct)
	}
	layout := fstest.MapFS{
		"templates/base.html": {Data: []byte(`<main>{{block "content" .}}{{end}}</main>`)},
		"templates/p.html":    {Data: []byte(`{{define "content"}}<p>{{.Title}}</p>{{block "rows" .}}<i>{{.Title}}</i>{{end}}{{end}}`)},
		"templates/q.html":    {Data: []byte(`{{define "content"}}q{{end}}`)}, // another page fills the same block
	}
	if body, _, err := render(layout, "", page{Title: "t"}); err != nil || body != "<main><p>t</p><i>t</i></main>" {
		t.Errorf("layout: %v %q", err, body)
	}
	if body, _, err := render(layout, "rows", page{Title: "t"}); err != nil || body != "<i>t</i>" {
		t.Errorf("block: %v %q", err, body)
	}
	sets, err := ParsePages(layout, nil, "p.html", "q.html")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	if err := Render(w, sets, "q.html", "", page{}); err != nil || w.Body.String() != "<main>q</main>" {
		t.Errorf("second page: %v %q", err, w.Body)
	}
	// A block route served before its page still renders the page right.
	url := fstest.MapFS{
		"templates/base.html": {Data: []byte(`<a href="/q?v={{template "rows" .}}">{{template "rows" .}}</a>`)},
		"templates/p.html":    {Data: []byte(`{{define "rows"}}{{.Title}}{{end}}`)},
	}
	pages := Pages(url, nil, "p.html")
	for _, c := range []struct{ block, want string }{{"rows", "x&amp;y"}, {"", `<a href="/q?v=x%26y">x&amp;y</a>`}} {
		w := httptest.NewRecorder()
		if err := Render(w, pages, "p.html", c.block, page{Title: "x&y"}); err != nil || w.Body.String() != c.want {
			t.Errorf("block %q first: %v %q, want %q", c.block, err, w.Body, c.want)
		}
	}
	broken := fstest.MapFS{"templates/p.html": {Data: []byte(`<b>{{.Title}}</b>{{.Post.X}}`)}}
	if body, _, err := render(broken, "", page{Title: "t"}); err == nil || body != "" {
		t.Errorf("failed render: %v, wrote %q", err, body)
	}

	w = httptest.NewRecorder()
	if err := JSON(w, map[string]int{"n": 1}); err != nil || w.Body.String() != "{\"n\":1}\n" || w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("JSON: %v %q", err, w.Body)
	}
	w = httptest.NewRecorder()
	if err := JSON(w, func() {}); err == nil || w.Body.Len() > 0 {
		t.Errorf("JSON of a func: %v %q", err, w.Body)
	}
}

func TestURLFunc(t *testing.T) {
	url := URLFunc(map[string]string{"post": "/posts/{id}", "tag": "/t/{tag}/{n}/{$}", "home": "/{$}"})
	for _, c := range []struct {
		name string
		args []any
		want string
		err  bool
	}{
		{"post", []any{int64(7)}, "/posts/7", false},
		{"post", []any{7}, "/posts/7", false},
		{"tag", []any{"a/b ?", int32(2)}, "/t/a%2Fb%20%3F/2/", false},
		{"home", nil, "/", false},
		{"nope", nil, "", true},
		{"post", nil, "", true},
		{"post", []any{1, 2}, "", true},
		{"post", []any{1.5}, "", true},
		{"tag", []any{"", 1}, "", true},
	} {
		got, err := url(c.name, c.args...)
		if got != c.want || (err != nil) != c.err {
			t.Errorf("url %s %v: %q %v", c.name, c.args, got, err)
		}
	}
}
