package maago

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maago-web/maago/internal/rtcore"
)

// sentFile is the File of a form that sent body under name.
func sentFile(t *testing.T, name string, body []byte) File {
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
	return rtcore.NewFile(r.MultipartForm.File["doc"][0], "image/png").(File)
}

// A file is kept under a random key, never its name: ../ in a name goes
// nowhere (D-140, M7's condition 5). A key of another shape opens nothing.
func TestMedia(t *testing.T) {
	dir := t.TempDir()
	ctx := WithMedia(context.Background(), LocalMedia{Dir: dir})
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
	if _, err := SaveMedia(context.Background(), sentFile(t, "a.png", pngBytes)); err == nil || !strings.Contains(err.Error(), "maago.WithMedia") {
		t.Errorf("no store: %v", err)
	}
	// No store: r is not read, so a worker's reader is not half eaten.
	unread := strings.NewReader("x")
	if _, err := SaveMediaFrom(context.Background(), "a.txt", unread); err == nil || !strings.Contains(err.Error(), "maago.WithMedia") || unread.Len() != 1 {
		t.Errorf("no store, from a reader: %v, %d bytes left", err, unread.Len())
	}
	if m, err := SaveMedia(WithMedia(ctx, fullStore{}), sentFile(t, "a.png", pngBytes)); err == nil || m != (Media{}) {
		t.Errorf("a failed put: %+v, %v", m, err)
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
	// The store takes one file's name, never a path.
	l := LocalMedia{Dir: dir}
	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "../" + m.Key} {
		if _, err := l.Put(ctx, name, "", strings.NewReader("x")); err == nil {
			t.Errorf("put %q", name)
		}
		if _, err := l.Open(ctx, name); err == nil {
			t.Errorf("open %q", name)
		}
		if err := l.Delete(ctx, name); err == nil {
			t.Errorf("delete %q", name)
		}
	}
}

// List gives every file, keys or not, but no directory or link; Delete takes
// what List gave.
func TestLocalMediaList(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*"), 0o600)
	os.WriteFile(filepath.Join(dir, ".tmp-x"), []byte("xy"), 0o600)
	os.Mkdir(filepath.Join(dir, "sub"), 0o700)
	if err := os.Symlink(filepath.Join(dir, ".gitignore"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	l := LocalMedia{Dir: dir}
	infos, err := l.List(context.Background(), "")
	if err != nil || len(infos) != 2 || infos[0].Name() != ".gitignore" || infos[1].Name() != ".tmp-x" || infos[1].Size() != 2 {
		t.Fatalf("listed %v, %v", infos, err)
	}
	if infos, _ := l.List(context.Background(), ".tmp-x"); len(infos) != 0 {
		t.Errorf("after the last: %v", infos)
	}
	if err := l.Delete(context.Background(), ".tmp-x"); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tmp-x")); !os.IsNotExist(err) {
		t.Errorf("not deleted: %v", err)
	}
}

// A local URL needs the site's address and the last Handler serving the
// local route, whose Prefix goes after the address; it is signed whole:
// values of two URLs do not mix, nor is one added, and what is no time is
// refused however signed.
func TestLocalMediaURL(t *testing.T) {
	defer func(p *rtcore.Process) { rtcore.Current.Store(p) }(rtcore.Current.Load())
	key := strings.Repeat("a", 32)
	m := Media{Key: key, Name: "a.png", Type: "image/png"}
	if _, err := (LocalMedia{Dir: t.TempDir()}).URL(m, time.Minute); err == nil || !strings.HasPrefix(err.Error(), "maago: a local media store makes URLs only with its SiteURL: set Config.Media to maago.LocalMedia{") {
		t.Errorf("no SiteURL: %v", err)
	}
	l := LocalMedia{Dir: t.TempDir(), SiteURL: "https://example.com/"}
	if _, err := l.URL(Media{Key: "../x"}, time.Minute); err == nil {
		t.Error("a URL of ../x")
	}
	for _, site := range []string{"/app", "example.com", "ftp://example.com", "https://u:p@example.com", "https://example.com/?a=1", "https://example.com?", "https://example.com/#a", "https://", "https://example.com/app", "https://example.com//"} {
		if _, err := (LocalMedia{SiteURL: site}).URL(m, time.Minute); err == nil || !strings.HasPrefix(err.Error(), "maago: LocalMedia.SiteURL") {
			t.Errorf("SiteURL %q: %v", site, err)
		}
	}
	noHandler := func(when string) {
		t.Helper()
		if _, err := l.URL(m, time.Minute); err == nil || !strings.HasPrefix(err.Error(), "maago: no Handler serves this process's local media URLs yet") {
			t.Errorf("%s: %v", when, err)
		}
	}
	rtcore.Current.Store(nil)
	noHandler("no Handler")
	apps := []App{NewApp("a", route("GET", "/x", false, ok))}
	if _, err := Handler(Config{DB: testDB, Apps: apps, Media: l, Prefix: "/app"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Handler(Config{DB: testDB, Apps: apps, Media: l, Prefix: "/b", MaxBody: -1}); err == nil {
		t.Error("MaxBody -1: no error")
	}
	a, _ := l.URL(m, time.Minute)
	n := m
	n.Name = "b.png"
	b, _ := l.URL(n, time.Minute)
	if !strings.HasPrefix(a, "https://example.com/app/maago/media?") { // the failed Handler leaves the last one's
		t.Errorf("URL %s", a)
	}
	if _, err := Handler(Config{DB: testDB, Apps: apps, Media: struct{ MediaStore }{}}); err != nil {
		t.Fatal(err)
	}
	noHandler("a bucket's Handler last")
	ua, _ := url.Parse(a)
	ub, _ := url.Parse(b)
	qa, qb := ua.Query(), ub.Query()
	qa.Set("sig", qb.Get("sig"))
	notTime := url.Values{"key": {key}, "name": {"a.png"}, "type": {"image/png"}, "expires": {"soon"}}
	notTime.Set("sig", sign(notTime))
	ok := ua.Query()
	added := ua.Query()
	added.Set("x", "1")
	for _, c := range []struct {
		q    url.Values
		code int
	}{{ok, 404}, {qa, 403}, {notTime, 403}, {added, 403}} { // ok's file is none: a signed URL reaches the store
		r := httptest.NewRequest("GET", "/maago/media?"+c.q.Encode(), nil).WithContext(WithMedia(context.Background(), l))
		if err := serveSigned(httptest.NewRecorder(), r); rtcore.Code(err) != c.code {
			t.Errorf("%v: %v", c.q, err)
		}
	}
}

// Forms that take files need a media store: Config.MediaDir, a directory
// that exists, or Config.Media, not both.
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
		{Config{Apps: []App{NewApp("a", up)}, MediaDir: t.TempDir(), Media: LocalMedia{Dir: t.TempDir()}}, "maago: Config.Media and Config.MediaDir are both set; MediaDir is short for Media: maago.LocalMedia{Dir: MediaDir}: keep one"},
		{Config{Apps: []App{NewApp("a", up)}, Media: LocalMedia{}}, "maago: Config.Media's Dir is empty"},
		{Config{Apps: []App{NewApp("a", up)}, Media: &LocalMedia{}}, "maago: Config.Media's Dir is empty"},
		{Config{Apps: []App{NewApp("a", up)}, Media: (*LocalMedia)(nil)}, "maago: Config.Media's Dir is empty"},
		{Config{Apps: []App{NewApp("a", up)}, Media: LocalMedia{Dir: filepath.Join(t.TempDir(), "none")}}, "maago: Config.Media's Dir: "},
		{Config{Apps: []App{NewApp("a", up)}, Media: LocalMedia{Dir: file}}, "maago: Config.Media's Dir " + `"` + file + `" is not a directory`},
		{Config{Apps: []App{NewApp("a", up)}, Media: LocalMedia{Dir: t.TempDir(), SiteURL: "/app"}}, "maago: LocalMedia.SiteURL \"/app\" is not"},
		{Config{Apps: []App{NewApp("a", up)}, Media: LocalMedia{Dir: t.TempDir(), SiteURL: "https://example.com?a=1"}}, "maago: LocalMedia.SiteURL"},
		{Config{Apps: []App{NewApp("a", up)}, Media: LocalMedia{Dir: t.TempDir(), SiteURL: "https://u@example.com"}}, "maago: LocalMedia.SiteURL"},
		{Config{Apps: []App{NewApp("a", up)}, Media: LocalMedia{Dir: t.TempDir(), SiteURL: "https://example.com"}, Prefix: "/app"}, ""},
		{Config{Apps: []App{NewApp("a", up)}, Media: LocalMedia{Dir: t.TempDir(), SiteURL: "https://example.com/app"}, Prefix: "/app"}, "maago: LocalMedia.SiteURL \"https://example.com/app\" is not"},
		{Config{Apps: []App{NewApp("a", up)}, Media: &LocalMedia{Dir: t.TempDir(), SiteURL: "https://example.com/"}, Prefix: "/app"}, ""},
		{Config{Apps: []App{NewApp("a", up)}, Media: struct{ MediaStore }{}}, ""},
	} {
		c.cfg.DB = testDB
		_, err := Handler(c.cfg)
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.HasPrefix(err.Error(), c.want)) {
			t.Errorf("got %v, want %q", err, c.want)
		}
	}
	// Handler gives each request the store, MediaDir's a local one.
	dir := t.TempDir()
	var got MediaStore
	see := func(w http.ResponseWriter, r *http.Request) error {
		got, _ = mediaStore(r.Context())
		return nil
	}
	serve(t, Config{Apps: []App{NewApp("a", route("GET", "/x", false, see))}, MediaDir: dir}, httptest.NewRequest("GET", "/x", nil))
	if got != (LocalMedia{Dir: dir}) {
		t.Errorf("the request's store: %#v", got)
	}
	// Only a local store's URLs lead to Handler.
	other := struct{ MediaStore }{}
	if w, _ := serve(t, Config{Apps: []App{NewApp("a", route("GET", "/x", false, see))}, Media: other}, httptest.NewRequest("GET", "/maago/media", nil)); w.Code != 404 {
		t.Errorf("a bucket's site: %d", w.Code)
	}
	if w, _ := serve(t, Config{Apps: []App{NewApp("a", route("GET", "/x", false, see))}, MediaDir: dir}, httptest.NewRequest("GET", "/maago/media", nil)); w.Code != 403 {
		t.Errorf("a local store's site, no signature: %d", w.Code)
	}
}

// fullStore keeps nothing.
type fullStore struct{ MediaStore }

func (fullStore) Put(context.Context, string, string, io.Reader) (int64, error) {
	return 0, errors.New("full")
}

var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 100)...)

// The branches of SweepMedia that stop before the database answers: none
// deletes a file. The gentest packages sweep a real database (M13 7번).
func TestSweepMedia(t *testing.T) {
	saved := rtcore.MediaQueries
	t.Cleanup(func() { rtcore.MediaQueries = saved })
	dir := t.TempDir()
	orphan := filepath.Join(dir, strings.Repeat("a", 32))
	if err := os.WriteFile(orphan, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(orphan, old, old); err != nil {
		t.Fatal(err)
	}
	ctx := WithMedia(context.Background(), LocalMedia{Dir: dir})
	query := `SELECT "key" FROM "files_file"`
	for _, c := range []struct {
		queries   []string
		olderThan time.Duration
		want      string
	}{
		{[]string{query}, 30 * time.Minute, "at least an hour"},
		{[]string{query}, time.Hour, "no driver"}, // an hour is enough
		{nil, 0, `maago:"media"`},
		{[]string{query, query + " UNION ALL SELECT 1"}, 0, "run maago gen"},
		{[]string{query}, 0, "no driver"}, // what is in use is unknown: nothing is deleted
	} {
		rtcore.MediaQueries = c.queries
		if err := SweepMedia(ctx, testDB, c.olderThan); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%d queries, %v: %v, want %q", len(c.queries), c.olderThan, err, c.want)
		}
		if _, err := os.Stat(orphan); err != nil {
			t.Fatalf("%d queries, %v: the orphan is gone: %v", len(c.queries), c.olderThan, err)
		}
	}
	// No old file: the rows are not read.
	if err := SweepMedia(WithMedia(context.Background(), LocalMedia{Dir: t.TempDir()}), testDB, 0); err != nil {
		t.Errorf("no old file: %v", err)
	}
}
