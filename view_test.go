package maago

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"
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
	tmpl := template.Must(template.New("p.html").Parse(`<b>{{.Title}}</b>{{.Missing.X}}`))
	w := httptest.NewRecorder()
	if err := Render(w, tmpl, "p.html", struct{ Title string }{"t"}); err == nil || w.Body.Len() > 0 {
		t.Errorf("failed render: %v, wrote %q", err, w.Body)
	}
	// No tag in the page: Content-Type must be set, not sniffed.
	plain := template.Must(template.New("p.html").Parse(`{{.Title}}`))
	w = httptest.NewRecorder()
	if err := Render(w, plain, "p.html", struct{ Title string }{"<t>"}); err != nil || w.Body.String() != "&lt;t&gt;" || w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("render: %v %q", err, w.Body)
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
