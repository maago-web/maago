package mediatest

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/maago-web/maago"
)

// A store with no disk under it passes too: the runtime asks nothing of
// a store's files but MediaFile.
func TestMemory(t *testing.T) {
	s := &memory{files: fstest.MapFS{}}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	defer s.srv.Close()
	Store(t, s)
}

// memory keeps files in a map and gives URLs its own server answers, as a
// bucket would.
type memory struct {
	mu    sync.Mutex
	files fstest.MapFS
	srv   *httptest.Server
}

func (s *memory) Put(ctx context.Context, key, typ string, r io.Reader) (int64, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[key] = &fstest.MapFile{Data: b, ModTime: time.Now(), Sys: typ}
	return int64(len(b)), nil
}

func (s *memory) Open(ctx context.Context, key string) (maago.MediaFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.files.Open(key)
	if err != nil {
		return nil, err
	}
	return f.(maago.MediaFile), nil
}

func (s *memory) Delete(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.files, name)
	return nil
}

// List gives one file a page, so the pages are many.
func (s *memory) List(ctx context.Context, after string) ([]fs.FileInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := fs.ReadDir(s.files, ".")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Name() > after {
			info, err := e.Info()
			return []fs.FileInfo{info}, err
		}
	}
	return nil, nil
}

// URL takes whole seconds, as a bucket's does.
func (s *memory) URL(m maago.Media, ttl time.Duration) (string, error) {
	if ttl%time.Second != 0 {
		return "", fmt.Errorf("ttl %v is not whole seconds", ttl)
	}
	q := url.Values{"name": {m.Name}, "exp": {strconv.FormatInt(time.Now().Add(ttl).Unix(), 10)}}
	q.Set("sig", sign(m.Key, q))
	return s.srv.URL + "/" + m.Key + "?" + q.Encode(), nil
}

func sign(key string, q url.Values) string {
	mac := hmac.New(sha256.New, []byte("memory"))
	fmt.Fprintf(mac, "%s\n%s\n%s", key, q.Get("name"), q.Get("exp"))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *memory) serve(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	key := r.URL.Path[1:]
	exp, err := strconv.ParseInt(q.Get("exp"), 10, 64)
	if !hmac.Equal([]byte(q.Get("sig")), []byte(sign(key, q))) || err != nil || time.Now().Unix() > exp {
		http.Error(w, "denied", http.StatusForbidden)
		return
	}
	f, err := s.Open(r.Context(), key)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header()["Content-Type"] = []string{info.Sys().(string)} // Put's type, never sniffed
	w.Header().Set("Content-Disposition", "attachment")
	io.Copy(w, f)
}
