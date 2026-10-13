package maago

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sentFile is the File of a form that sent body under name.
func sentFile(t *testing.T, name string, body []byte) File {
	t.Helper()
	r := multipartRequest(t, "/f", filePart("doc", name, body, "application/octet-stream"))
	vals, err := MultipartValues(r, "doc")
	if err != nil {
		t.Fatal(err)
	}
	var f Field[File]
	BindFile(r, vals, &f, Rule{Name: "doc"})
	if f.Error != "" {
		t.Fatal(f.Error)
	}
	return f.Value
}

// A file is kept under a random key, never its name: ../ in a name goes
// nowhere (D-140, M7's condition 5). A key of another shape opens nothing.
func TestMedia(t *testing.T) {
	dir := t.TempDir()
	ctx := WithMediaDir(context.Background(), dir)
	m, err := SaveMedia(ctx, sentFile(t, "../../evil.png", pngBytes))
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != m.Key || !validKey(m.Key) || m.Name != "../../evil.png" && m.Name != "evil.png" || m.Type != "image/png" || m.Size != int64(len(pngBytes)) {
		t.Errorf("saved %+v as %v", m, entries)
	}
	if info, _ := os.Stat(filepath.Join(dir, m.Key)); info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", info.Mode())
	}
	f, err := OpenMedia(ctx, m.Key)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(f); string(b) != string(pngBytes) {
		t.Errorf("content %q", b)
	}
	f.Close()
	if other, _ := SaveMedia(ctx, sentFile(t, "a.png", pngBytes)); other.Key == m.Key {
		t.Error("two files, one key")
	}
	for _, key := range []string{"../x", strings.ToUpper(m.Key), m.Key[:31], "", ".tmp-" + m.Key, strings.Repeat("0", 32)} {
		if _, err := OpenMedia(ctx, key); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("open %q: %v", key, err)
		}
	}
	if err := DeleteMedia(ctx, "../x"); err == nil {
		t.Error("delete ../x")
	}
	for range 2 { // gone already is no error
		if err := DeleteMedia(ctx, m.Key); err != nil {
			t.Error(err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, m.Key)); !os.IsNotExist(err) {
		t.Errorf("not deleted: %v", err)
	}
	if _, err := SaveMedia(context.Background(), sentFile(t, "a.png", pngBytes)); err == nil || !strings.Contains(err.Error(), "maago.WithMediaDir") {
		t.Errorf("no directory: %v", err)
	}
	// A link inside the directory does not lead out of it.
	outside := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(outside, []byte("secret"), 0o600)
	link := strings.Repeat("a", 32)
	if err := os.Symlink(outside, filepath.Join(dir, link)); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenMedia(ctx, link); err == nil {
		t.Error("a link out of the directory opens")
	}
}

// A file goes down to download, never as a page of the site, under the
// name it came with; none is 404 (D-140).
func TestServeMedia(t *testing.T) {
	dir := t.TempDir()
	ctx := WithMediaDir(context.Background(), dir)
	m, err := SaveMedia(ctx, sentFile(t, "a.png", pngBytes))
	if err != nil {
		t.Fatal(err)
	}
	serveMedia := func(method string, m Media, header ...string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/m", nil).WithContext(ctx)
		for i := 0; i < len(header); i += 2 {
			r.Header.Set(header[i], header[i+1])
		}
		w := httptest.NewRecorder()
		ServeMedia(w, r, m)
		return w
	}
	w := serveMedia("GET", m)
	if h := w.Header(); w.Code != 200 || w.Body.String() != string(pngBytes) || h.Get("Content-Type") != "image/png" || h.Get("X-Content-Type-Options") != "nosniff" ||
		h.Get("Content-Disposition") != `attachment; filename=a.png` || h.Get("Cache-Control") != "private" || h.Get("Last-Modified") == "" {
		t.Errorf("GET: %d %v", w.Code, h)
	}
	if w = serveMedia("GET", m, "Range", "bytes=1-3"); w.Code != 206 || w.Body.String() != "PNG" {
		t.Errorf("Range: %d %q", w.Code, w.Body)
	}
	if w = serveMedia("HEAD", m); w.Code != 200 || w.Body.Len() != 0 {
		t.Errorf("HEAD: %d %q", w.Code, w.Body)
	}
	for name, want := range map[string]string{
		`say "hi".png`:   `attachment; filename="say \"hi\".png"`,
		`a\b.png`:        `attachment; filename="a\\b.png"`,
		"사진.png":         `attachment; filename*=utf-8''%EC%82%AC%EC%A7%84.png`,
		"a\r\nX-Evil: 1": `attachment; filename*=utf-8''a%0D%0AX-Evil%3A%201`,
		"":               `attachment`,
	} {
		n := m
		n.Name = name
		if got := serveMedia("GET", n).Header().Get("Content-Disposition"); got != want {
			t.Errorf("%q: %s, want %s", name, got, want)
		}
	}
	for typ, want := range map[string]string{"": "application/octet-stream", "not a type": "application/octet-stream", "text/html; charset=utf-8": "text/html; charset=utf-8"} {
		n := m
		n.Type = typ
		if got := serveMedia("GET", n).Header().Get("Content-Type"); got != want {
			t.Errorf("type %q: %s, want %s", typ, got, want)
		}
	}
	for _, key := range []string{"../x", strings.Repeat("0", 32)} {
		n := m
		n.Key = key
		if w := serveMedia("GET", n); w.Code != 404 || w.Header().Get("Content-Disposition") != "" {
			t.Errorf("%s: %d %v", key, w.Code, w.Header())
		}
	}
}

// Forms that take files need Config.MediaDir, a directory that exists.
func TestMediaDirConfig(t *testing.T) {
	up := route("POST", "/up", false, ok)
	up.Files = true
	file := filepath.Join(t.TempDir(), "file")
	os.WriteFile(file, nil, 0o600)
	for _, c := range []struct {
		cfg  Config
		want string
	}{
		{Config{Apps: []App{NewApp("a", up)}}, "maago: forms take files, but Config.MediaDir is empty"},
		{Config{Apps: []App{NewApp("a", up)}, MediaDir: filepath.Join(t.TempDir(), "none")}, "maago: Config.MediaDir: "},
		{Config{Apps: []App{NewApp("a", route("GET", "/x", false, ok))}, MediaDir: file}, "maago: Config.MediaDir " + `"` + file + `" is not a directory`},
		{Config{Apps: []App{NewApp("a", up)}, MediaDir: t.TempDir()}, ""},
	} {
		c.cfg.DB = testDB
		_, err := Handler(c.cfg)
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.HasPrefix(err.Error(), c.want)) {
			t.Errorf("got %v, want %q", err, c.want)
		}
	}
	// Handler gives each request the directory.
	dir := t.TempDir()
	var got string
	see := func(w http.ResponseWriter, r *http.Request) error {
		got, _ = r.Context().Value(mediaKey{}).(string)
		return nil
	}
	serve(t, Config{Apps: []App{NewApp("a", route("GET", "/x", false, see))}, MediaDir: dir}, httptest.NewRequest("GET", "/x", nil))
	if got != dir {
		t.Errorf("the request's directory: %q", got)
	}
}
