// Package mediatest is the test every maago.MediaStore of this module
// passes: the local one, a fake, contrib/s3's.
package mediatest

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/internal/rtcore"
)

// pngBytes is what Store keeps; the second file ends in another byte.
var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 100)...)

// Store checks s through the runtime's media functions, as views and
// workers reach it. Of s's methods it calls only what the runtime does
// not: List and Put.
func Store(t *testing.T, s maago.MediaStore) {
	ctx := maago.WithMedia(t.Context(), s)
	other := append(slices.Clone(pngBytes[:len(pngBytes)-1]), 1)
	m, err := maago.SaveMedia(ctx, sentFile(t, "a.png", pngBytes))
	if err != nil {
		t.Fatal(err)
	}
	m2, err := maago.SaveMedia(ctx, sentFile(t, "b.png", other))
	if err != nil {
		t.Fatal(err)
	}
	if m.Key == m2.Key || m.Name != "a.png" || m.Type != "image/png" || m.Size != int64(len(pngBytes)) {
		t.Errorf("saved %+v and %+v", m, m2)
	}

	// Open: the bytes, their size and time; no file is fs.ErrNotExist.
	if b, info := open(t, ctx, m.Key); !bytes.Equal(b, pngBytes) || info.Size() != int64(len(pngBytes)) || time.Since(info.ModTime()).Abs() > time.Minute {
		t.Errorf("opened %q, %d bytes of %v", b, info.Size(), info.ModTime())
	}
	if _, err := maago.OpenMedia(ctx, newKey()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("open a key of no file: %v", err)
	}
	key := newKey()
	if n, err := s.Put(ctx, key, "image/png", bytes.NewReader(pngBytes)); err != nil || n != int64(len(pngBytes)) {
		t.Errorf("put: %d, %v", n, err)
	}
	if b, _ := open(t, ctx, key); !bytes.Equal(b, pngBytes) {
		t.Errorf("put, then opened %q", b)
	}
	if err := maago.DeleteMedia(ctx, key); err != nil {
		t.Error(err)
	}

	serveMedia(t, ctx, m)

	// List: every name in order, a page at a time, after after.
	all := list(t, ctx, s, "")
	first, second := min(m.Key, m2.Key), max(m.Key, m2.Key)
	if ns := names(all); !slices.IsSorted(ns) || len(slices.Compact(slices.Clone(ns))) != len(ns) || size(all, m.Key) != int64(len(pngBytes)) || size(all, m2.Key) != int64(len(other)) || size(all, key) != -1 {
		t.Errorf("listed %v", names(all))
	}
	if rest := list(t, ctx, s, first); size(rest, first) != -1 || size(rest, second) < 0 {
		t.Errorf("listed after %s: %v", first, names(rest))
	}

	// A failed Put leaves nothing, a half-written file neither.
	failed := newKey()
	if _, err := s.Put(ctx, failed, "image/png", io.MultiReader(bytes.NewReader(pngBytes[:50]), iotest.ErrReader(errors.New("cut")))); err == nil {
		t.Error("put of a broken reader: no error")
	}
	if _, err := maago.OpenMedia(ctx, failed); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("open after a failed put: %v", err)
	}
	for _, n := range names(list(t, ctx, s, "")) {
		if strings.Contains(n, failed) {
			t.Errorf("a failed put left %s", n)
		}
	}

	signedURLs(t, ctx, m, m2, other)

	for range 2 { // gone already is no error
		if err := maago.DeleteMedia(ctx, m.Key); err != nil {
			t.Error(err)
		}
	}
	if _, err := maago.OpenMedia(ctx, m.Key); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("open after delete: %v", err)
	}
	if size(list(t, ctx, s, ""), m.Key) != -1 {
		t.Error("listed after delete")
	}
	maago.DeleteMedia(ctx, m2.Key)
}

// serveMedia checks that a file goes down to download, never as a page
// of the site, under the name it came with; none is 404 (D-140).
func serveMedia(t *testing.T, ctx context.Context, m maago.Media) {
	t.Helper()
	serve := func(method string, m maago.Media, header ...string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/m", nil).WithContext(ctx)
		for i := 0; i < len(header); i += 2 {
			r.Header.Set(header[i], header[i+1])
		}
		w := httptest.NewRecorder()
		if err := maago.ServeMedia(w, r, m); err != nil {
			t.Fatalf("%s %s: %v", method, m.Key, err)
		}
		return w
	}
	w := serve("GET", m)
	if h := w.Header(); w.Code != 200 || w.Body.String() != string(pngBytes) || h.Get("Content-Type") != "image/png" || h.Get("X-Content-Type-Options") != "nosniff" ||
		h.Get("Content-Disposition") != `attachment; filename=a.png` || h.Get("Cache-Control") != "private" || h.Get("Last-Modified") == "" {
		t.Errorf("GET: %d %v", w.Code, h)
	}
	if w = serve("GET", m, "Range", "bytes=1-3"); w.Code != 206 || w.Body.String() != "PNG" {
		t.Errorf("Range: %d %q", w.Code, w.Body)
	}
	if w = serve("HEAD", m); w.Code != 200 || w.Body.Len() != 0 {
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
		if got := serve("GET", n).Header().Get("Content-Disposition"); got != want {
			t.Errorf("%q: %s, want %s", name, got, want)
		}
	}
	for typ, want := range map[string]string{"": "application/octet-stream", "not a type": "application/octet-stream", "text/html; charset=utf-8": "text/html; charset=utf-8"} {
		n := m
		n.Type = typ
		if got := serve("GET", n).Header().Get("Content-Type"); got != want {
			t.Errorf("type %q: %s, want %s", typ, got, want)
		}
	}
	// None is a 404 the view returns, with nothing written.
	for _, key := range []string{"../x", newKey()} {
		w := httptest.NewRecorder()
		err := maago.ServeMedia(w, httptest.NewRequest("GET", "/m", nil).WithContext(ctx), maago.Media{Key: key})
		if rtcore.Code(err) != 404 || !errors.Is(err, fs.ErrNotExist) || len(w.Header()) > 0 || w.Body.Len() > 0 {
			t.Errorf("%s: %v %v", key, err, w.Header())
		}
	}
}

// signedURLs checks that a URL gives m's file to download while it lasts,
// and nothing once any part of it is changed.
func signedURLs(t *testing.T, ctx context.Context, m, m2 maago.Media, other []byte) {
	t.Helper()
	u, err := maago.MediaURL(ctx, m, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if code, h, body := get(t, u); code != 200 || body != string(pngBytes) || h.Get("Content-Type") != "image/png" || !strings.HasPrefix(h.Get("Content-Disposition"), "attachment") {
		t.Errorf("GET %s: %d %v", u, code, h)
	}
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	reencoded := *parsed
	reencoded.RawQuery = parsed.Query().Encode()
	if code, _, _ := get(t, reencoded.String()); code != 200 {
		t.Errorf("GET with the query encoded again: %d", code)
	}
	changed := []string{strings.Replace(u, m.Key, m2.Key, 1)} // another file's key
	for name, values := range parsed.Query() {
		for i, v := range values {
			c := *parsed
			q := parsed.Query()
			q[name][i] = otherFirst(v)
			c.RawQuery = q.Encode()
			changed = append(changed, c.String())
		}
	}
	for _, c := range changed {
		if code, _, body := get(t, c); code < 400 || code > 499 || body == string(pngBytes) || body == string(other) {
			t.Errorf("GET %s: %d", c, code)
		}
	}
	short, err := maago.MediaURL(ctx, m, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	if code, _, _ := get(t, short); code < 400 || code > 499 {
		t.Errorf("GET of an expired URL: %d", code)
	}
	for _, ttl := range []time.Duration{0, 8 * 24 * time.Hour} {
		if _, err := maago.MediaURL(ctx, m, ttl); err == nil {
			t.Errorf("a URL for %v: no error", ttl)
		}
	}
}

// otherFirst is v with its first character another: all of its bits
// count, in base64 or hex.
func otherFirst(v string) string {
	if v == "" {
		return "a"
	}
	if v[0] == 'a' {
		return "b" + v[1:]
	}
	return "a" + v[1:]
}

func get(t *testing.T, u string) (int, http.Header, string) {
	t.Helper()
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(b)
}

func open(t *testing.T, ctx context.Context, key string) ([]byte, fs.FileInfo) {
	t.Helper()
	f, err := maago.OpenMedia(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	return b, info
}

// list is every file s lists after after, a page at a time.
func list(t *testing.T, ctx context.Context, s maago.MediaStore, after string) []fs.FileInfo {
	t.Helper()
	var all []fs.FileInfo
	for range 1000 { // a List that ignores after would never end
		page, err := s.List(ctx, after)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			return all
		}
		all = append(all, page...)
		after = page[len(page)-1].Name()
	}
	t.Fatal("List gives page after page")
	return nil
}

func names(infos []fs.FileInfo) []string {
	var ns []string
	for _, i := range infos {
		ns = append(ns, i.Name())
	}
	return ns
}

// size is the size of name's file in infos, -1 for none.
func size(infos []fs.FileInfo, name string) int64 {
	for _, i := range infos {
		if i.Name() == name {
			return i.Size()
		}
	}
	return -1
}

func newKey() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// sentFile is the File of a form that sent body under name.
func sentFile(t *testing.T, name string, body []byte) maago.File {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	w, _ := mw.CreateFormFile("doc", name)
	w.Write(body)
	mw.Close()
	r := httptest.NewRequest("POST", "/f", &b)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	return rtcore.NewFile(r.MultipartForm.File["doc"][0], "image/png").(maago.File)
}
