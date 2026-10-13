package maago

import (
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func staticFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

// The hashed name follows the content; the hashed name is kept for a
// year and the plain one asked again, with one ETag; a name of neither
// is 404 (D-136, D-137, M7's condition 3).
func TestStatic(t *testing.T) {
	t.Cleanup(func() { current.Store(nil) })
	s := Static("board", staticFS(map[string]string{"static/app.css": "body{}", "static/same.css": "body{}", "static/README": "hi"}), "app.css")
	if again := Static("board", staticFS(map[string]string{"static/app.css": "body{}"})); again.files["app.css"].hashed != s.files["app.css"].hashed {
		t.Error("the same content has another name")
	}
	changed := Static("board", staticFS(map[string]string{"static/app.css": "body{color:red}"}))
	hashed := s.files["app.css"].hashed
	if changed.files["app.css"].hashed == hashed || !strings.HasPrefix(hashed, "app.") || !strings.HasSuffix(hashed, ".css") || len(hashed) != len("app..css")+16 {
		t.Errorf("hashed names: %q, changed %q", hashed, changed.files["app.css"].hashed)
	}
	if h := s.files["README"].hashed; !strings.HasPrefix(h, "README.") || len(h) != len("README.")+16 {
		t.Errorf("README: %q", h)
	}
	if s.files["same.css"].hashed == hashed {
		t.Error("two names, one hashed name")
	}

	h, err := Handler(Config{DB: testDB, Apps: []App{NewApp("example.com/board", route("GET", "/x", false, ok)).WithStatic(s)}})
	if err != nil {
		t.Fatal(err)
	}
	get := func(method, target string, header ...string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, nil)
		for i := 0; i < len(header); i += 2 {
			r.Header.Set(header[i], header[i+1])
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := get("GET", "/static/board/"+hashed)
	etag := w.Header().Get("ETag")
	if w.Code != 200 || w.Body.String() != "body{}" || w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" ||
		w.Header().Get("Content-Type") != "text/css; charset=utf-8" || len(etag) != 66 {
		t.Errorf("hashed: %d %q %v", w.Code, w.Body, w.Header())
	}
	w = get("GET", "/static/board/app.css")
	if w.Code != 200 || w.Body.String() != "body{}" || w.Header().Get("Cache-Control") != "no-cache" || w.Header().Get("ETag") != etag {
		t.Errorf("plain: %d %q %v", w.Code, w.Body, w.Header())
	}
	if w = get("GET", "/static/board/app.css", "If-None-Match", etag); w.Code != 304 || w.Body.Len() != 0 {
		t.Errorf("If-None-Match: %d %q", w.Code, w.Body)
	}
	if w = get("GET", "/static/board/"+hashed, "Range", "bytes=1-2"); w.Code != 206 || w.Body.String() != "od" {
		t.Errorf("Range: %d %q", w.Code, w.Body)
	}
	if w = get("HEAD", "/static/board/"+hashed); w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("ETag") != etag {
		t.Errorf("HEAD: %d %q %v", w.Code, w.Body, w.Header())
	}
	if w = get("GET", "/static/board/"+s.files["README"].hashed); w.Body.String() != "hi" || w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("README: %d %q %v", w.Code, w.Body, w.Header())
	}
	wrong := strings.Replace(hashed, hashed[4:6], "zz", 1)
	if w = get("POST", "/static/board/app.css"); w.Code != 405 {
		t.Errorf("POST: %d", w.Code)
	}
	for _, target := range []string{"/static/board/" + wrong, "/static/board/", "/static/board/nope.css", "/static/other/app.css"} {
		if w = get("GET", target); w.Code != 404 {
			t.Errorf("%s: %d", target, w.Code)
		}
	}
}

// {{static}} in a template made before the Handler: the escaping pass
// that Pages runs gets an error, not a panic, and the page renders the
// path once the Handler has the files, under Config.Prefix.
func TestStaticFunc(t *testing.T) {
	current.Store(nil) // as if no test had made a Handler
	t.Cleanup(func() { current.Store(nil) })
	if _, err := StaticFunc("board/app.css"); err == nil {
		t.Error("before any Handler")
	}
	pages := Pages(staticFS(map[string]string{"templates/home_page.html": `<link href="{{static "board/app.css"}}">`}), template.FuncMap{"static": StaticFunc}, "home_page.html")
	s := Static("board", staticFS(map[string]string{"static/app.css": "body{}"}), "app.css")
	if _, err := Handler(Config{DB: testDB, Apps: []App{NewApp("example.com/board").WithStatic(s)}, Prefix: "/app"}); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := pages["home_page.html"].Execute(&b, nil); err != nil || b.String() != `<link href="/app/static/board/`+s.files["app.css"].hashed+`">` {
		t.Errorf("page: %q %v", b.String(), err)
	}
	for _, ref := range []string{"auth/app.css", "board/nope.css", "board/" + s.files["app.css"].hashed, "app.css"} {
		if _, err := StaticFunc(ref); err == nil {
			t.Errorf("%s: no error", ref)
		}
	}
}

// What the generated code could only get wrong when static/ changed after
// maago gen panics as the process starts.
func TestStaticPanics(t *testing.T) {
	for _, c := range []struct {
		files map[string]string
		names []string
		want  string
	}{
		{map[string]string{"static/app.css": ""}, []string{"gone.css"}, "maago: app board has no static/gone.css; run maago gen"},
		{map[string]string{"static/app.css": "x"}, []string{"app.2d711642b726b044.css"}, "maago: app board has no static/app.2d711642b726b044.css; run maago gen"},
		{map[string]string{"static/app.css": "x", "static/sub/a.css": ""}, nil, "maago: static/sub of app board: directory in static/; run maago gen"},
		{map[string]string{"static/.env": ""}, nil, "maago: static/.env of app board: static file name \".env\" does not start"},
		{map[string]string{"static/a b.css": ""}, nil, "maago: static/a b.css of app board: static file name \"a b.css\" does not start"},
		{map[string]string{"static/app.css": "x", "static/app.2d711642b726b044.css": ""}, nil, "maago: static/app.2d711642b726b044.css and static/app.css of app board would share the name app.2d711642b726b044.css"},
		{map[string]string{}, nil, "maago: static files of app board: "},
	} {
		func() {
			defer func() {
				if got, _ := recover().(string); !strings.HasPrefix(got, c.want) {
					t.Errorf("%v: got %q, want %q", c.files, got, c.want)
				}
			}()
			Static("board", staticFS(c.files), c.names...)
		}()
	}
}
