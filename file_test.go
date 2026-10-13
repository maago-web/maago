package maago

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"testing"
)

var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 100)...)

// part is one part of a multipart body: a text value when filename is
// nil, else a file part, as a browser sends it.
type part struct {
	name     string
	filename *string
	body     []byte
	ctype    string
}

func filePart(name, filename string, body []byte, ctype string) part {
	return part{name, &filename, body, ctype}
}

func multipartRequest(t *testing.T, path string, parts ...part) *http.Request {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	for _, p := range parts {
		h := textproto.MIMEHeader{}
		if p.filename == nil {
			h.Set("Content-Disposition", `form-data; name="`+p.name+`"`)
		} else {
			h.Set("Content-Disposition", `form-data; name="`+p.name+`"; filename="`+*p.filename+`"`)
			h.Set("Content-Type", p.ctype)
		}
		w, err := mw.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(p.body)
	}
	mw.Close()
	r := httptest.NewRequest("POST", path, &b)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r
}

// A form with a file comes as multipart/form-data, each key once; past
// Config.MaxBody it is a 413; the temporary files are gone after the
// route, after a panic too (D-139).
func TestMultipart(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	spilled := false // a file past the memory kept went to a temporary file
	read := func(w http.ResponseWriter, r *http.Request) error {
		_, err := MultipartValues(r, "title", "doc")
		if files, _ := os.ReadDir(tmp); len(files) > 0 {
			spilled = true
		}
		return err
	}
	panics := func(w http.ResponseWriter, r *http.Request) error {
		if _, err := MultipartValues(r, "title", "doc"); err != nil {
			return err
		}
		panic("after reading")
	}
	cfg := Config{Apps: []App{NewApp("a", route("POST", "/f", false, read), route("POST", "/p", false, panics))}, Auth: fakeAuth, MaxBody: 4 << 20}
	big := make([]byte, 2<<20) // past the 1MB kept in memory: a temporary file
	for _, c := range []struct {
		name string
		r    *http.Request
		code int
	}{
		{"a file and text", multipartRequest(t, "/f", part{name: "title", body: []byte("x")}, filePart("doc", "a.png", pngBytes, "image/png")), 200},
		{"a large file", multipartRequest(t, "/f", filePart("doc", "a.png", big, "image/png")), 200},
		{"a large file, then a panic", multipartRequest(t, "/p", filePart("doc", "a.png", big, "image/png")), 500},
		{"no file chosen", multipartRequest(t, "/f", filePart("doc", "", nil, "application/octet-stream")), 200},
		{"a file twice", multipartRequest(t, "/f", filePart("doc", "a.png", pngBytes, "image/png"), filePart("doc", "b.png", pngBytes, "image/png")), 400},
		{"text and a file", multipartRequest(t, "/f", part{name: "doc", body: []byte("x")}, filePart("doc", "a.png", pngBytes, "image/png")), 400},
		{"past MaxBody", multipartRequest(t, "/f", filePart("doc", "a.png", make([]byte, 5<<20), "image/png")), 413},
		{"urlencoded", func() *http.Request {
			r := httptest.NewRequest("POST", "/f", strings.NewReader("title=x&doc=a.png"))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return r
		}(), 400},
		{"no boundary", func() *http.Request {
			r := httptest.NewRequest("POST", "/f", strings.NewReader("x"))
			r.Header.Set("Content-Type", "multipart/form-data")
			return r
		}(), 400},
	} {
		c.r.Header.Set("X-User", "ann") // Auth gives the route a new r
		w, _ := serve(t, cfg, c.r)
		if w.Code != c.code {
			t.Errorf("%s: %d %q, want %d", c.name, w.Code, w.Body, c.code)
		}
		if left, _ := os.ReadDir(tmp); len(left) > 0 {
			t.Errorf("%s: temporary files left: %v", c.name, left)
		}
	}
	if !spilled {
		t.Error("no file went to a temporary file: the test does not see the removing")
	}
}

// A file field reads the file the request sent: none chosen is not given,
// its kind is its content's, not its name's or the browser's word.
func TestBindFile(t *testing.T) {
	html := []byte("<!DOCTYPE html><script>alert(1)</script>")
	bind := func(r *http.Request, rule Rule) Field[File] {
		t.Helper()
		var f Field[File]
		vals, err := MultipartValues(r, rule.Name)
		if err != nil {
			t.Fatal(err)
		}
		BindFile(r, vals, &f, rule)
		return f
	}
	rule := Rule{Name: "doc", Label: "Doc", Accept: []string{"png", "pdf"}}
	f := bind(multipartRequest(t, "/f", filePart("doc", "a.png", pngBytes, "application/octet-stream")), rule)
	if f.Error != "" || !f.Given || f.Raw != "a.png" || f.Value.Name != "a.png" || f.Value.Size != int64(len(pngBytes)) || f.Value.Type != "image/png" || f.Label != "Doc" {
		t.Errorf("png: %+v", f)
	}
	if rc, err := f.Value.Open(); err != nil {
		t.Errorf("open: %v", err)
	} else if b, _ := io.ReadAll(rc); !bytes.Equal(b, pngBytes) {
		t.Errorf("content: %q", b)
	}
	for name, c := range map[string]struct {
		r    *http.Request
		rule Rule
		err  string
	}{
		"html named .png, said to be png": {multipartRequest(t, "/f", filePart("doc", "a.png", html, "image/png")), rule, DefaultMessages.NotKind},
		"no file chosen, required":        {multipartRequest(t, "/f", filePart("doc", "", nil, "application/octet-stream")), rule, DefaultMessages.Required},
		"no part at all":                  {multipartRequest(t, "/f", part{name: "x", body: []byte("1")}), rule, DefaultMessages.Required},
		"text in a file field":            {multipartRequest(t, "/f", part{name: "doc", body: []byte("a.png")}), rule, DefaultMessages.Required},
		"no file chosen, optional":        {multipartRequest(t, "/f", filePart("doc", "", nil, "application/octet-stream")), Rule{Name: "doc", Optional: true}, ""},
		"html, any kind":                  {multipartRequest(t, "/f", filePart("doc", "a.html", html, "text/html")), Rule{Name: "doc"}, ""},
		"an empty file is sent":           {multipartRequest(t, "/f", filePart("doc", "e.txt", nil, "text/plain")), Rule{Name: "doc"}, ""},
	} {
		if f := bind(c.r, c.rule); f.Error != c.err {
			t.Errorf("%s: %q, want %q (%+v)", name, f.Error, c.err, f)
		}
	}
	if f := bind(multipartRequest(t, "/f", filePart("doc", "a.html", html, "image/png")), Rule{Name: "doc"}); f.Value.Type != "text/html" {
		t.Errorf("the type is the content's: %q", f.Value.Type)
	}
	var get Field[File] // a GET draws the field
	BindFile(httptest.NewRequest("GET", "/f", nil).WithContext(context.Background()), nil, &get, rule)
	if get.Error != "" || get.Given || get.Name != "doc" {
		t.Errorf("GET: %+v", get)
	}
	if _, err := (File{}).Open(); err == nil {
		t.Error("the zero File opens")
	}
	if !strings.Contains(strings.Join(FileKinds(), ","), "jpeg,pdf,png") {
		t.Errorf("FileKinds: %v", FileKinds())
	}
}
