package s3_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/contrib/s3"
	"github.com/maago-web/maago/internal/mediatest"
	"github.com/maago-web/maago/internal/s3fake"
)

func TestFake(t *testing.T) {
	mediatest.Store(t, s3fake.New(t))
}

// A reader of unknown length goes up with its length, and a Put that
// fails leaves no temporary file.
func TestPutLength(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	s := s3fake.New(t)
	ctx := t.Context()
	for _, body := range [][]byte{bytes.Repeat([]byte("x"), 1000), nil} {
		key := newKey()
		n, err := s.Put(ctx, key, "text/plain", iotest.OneByteReader(bytes.NewReader(body)))
		if err != nil || n != int64(len(body)) {
			t.Fatalf("put of %d bytes: %d, %v", len(body), n, err)
		}
		f, err := s.Open(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if b, err := io.ReadAll(f); !bytes.Equal(b, body) || err != nil {
			t.Errorf("opened %d bytes, %v", len(b), err)
		}
		f.Close()
	}
	wrong := s
	wrong.SecretAccessKey = "wrong"
	if _, err := wrong.Put(ctx, newKey(), "text/plain", strings.NewReader("x")); err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "SignatureDoesNotMatch") {
		t.Errorf("put signed with another secret: %v", err)
	}
	if left, _ := os.ReadDir(dir); len(left) > 0 {
		t.Errorf("left %v in TMPDIR", left)
	}
	other := s
	other.Bucket = "other"
	if err := other.Delete(ctx, newKey()); err == nil || !strings.Contains(err.Error(), "NoSuchBucket") || errors.Is(err, fs.ErrNotExist) {
		t.Errorf("delete in a bucket of none: %v", err)
	}
}

// A redirect is an error, not followed: a PUT or DELETE followed as a
// GET would end in a 200 for nothing done.
func TestRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ok" {
			http.Redirect(w, r, "/ok", http.StatusFound)
			return
		}
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		io.WriteString(w, "<ListBucketResult></ListBucketResult>")
	}))
	defer srv.Close()
	s := s3.Store{Endpoint: srv.URL, Region: "us-east-1", Bucket: "media", AccessKeyID: "id", SecretAccessKey: "secret"}
	ctx := t.Context()
	key := newKey()
	_, put := s.Put(ctx, key, "text/plain", strings.NewReader("x"))
	_, open := s.Open(ctx, key)
	_, list := s.List(ctx, "")
	for i, err := range []error{put, open, s.Delete(ctx, key), list} {
		if err == nil || !strings.Contains(err.Error(), "302") {
			t.Errorf("call %d: %v", i, err)
		}
	}
}

// Only a file's name reaches the bucket, never the bucket itself or
// another path; a URL is only ever of a key.
func TestNames(t *testing.T) {
	s := s3fake.New(t)
	ctx := t.Context()
	for _, name := range []string{"", ".", "..", "a/b", `a\b`} {
		const want = "is not the name of one file"
		if _, err := s.Put(ctx, name, "text/plain", strings.NewReader("x")); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("put %q: %v", name, err)
		}
		if _, err := s.Open(ctx, name); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("open %q: %v", name, err)
		}
		if err := s.Delete(ctx, name); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("delete %q: %v", name, err)
		}
	}
	for _, key := range []string{"", ".", "..", "a/b", `a\b`, "a.png"} {
		if _, err := s.URL(maago.Media{Key: key}, time.Hour); err == nil || !strings.Contains(err.Error(), "is not one maago.SaveMedia makes") {
			t.Errorf("URL of %q: %v", key, err)
		}
	}
}

// Every method says which field is wrong, and never prints a secret.
func TestConfig(t *testing.T) {
	good := s3fake.New(t)
	ctx := t.Context()
	key := newKey()
	for field, change := range map[string][]func(*s3.Store){
		"Endpoint": {
			func(s *s3.Store) { s.Endpoint = "" },
			func(s *s3.Store) { s.Endpoint = "https://s3.amazonaws.com/media" },
			func(s *s3.Store) { s.Endpoint = "https://s3.amazonaws.com?x=1" },
			func(s *s3.Store) { s.Endpoint = "https://KEY:SECRET@s3.amazonaws.com" },
			func(s *s3.Store) { s.Endpoint = "ftp://s3.amazonaws.com" },
			func(s *s3.Store) { s.Endpoint = "http://SECRET@[::1" },
		},
		"Region":          {func(s *s3.Store) { s.Region = "" }},
		"AccessKeyID":     {func(s *s3.Store) { s.AccessKeyID = "" }},
		"SecretAccessKey": {func(s *s3.Store) { s.SecretAccessKey = "" }},
		"Bucket": {
			func(s *s3.Store) { s.Bucket = "" },
			func(s *s3.Store) { s.Bucket = "a/b" },
			func(s *s3.Store) { s.Bucket = "." },
			func(s *s3.Store) { s.Bucket = ".." },
			func(s *s3.Store) { s.Bucket = `a\b` },
			func(s *s3.Store) { s.Bucket = "ab" },
			func(s *s3.Store) { s.Bucket = "Media" },
			func(s *s3.Store) { s.Bucket = "-ab" },
			func(s *s3.Store) { s.Bucket = "a\nb" },
		},
	} {
		for _, c := range change {
			s := good
			c(&s)
			_, put := s.Put(ctx, key, "text/plain", strings.NewReader("x"))
			_, open := s.Open(ctx, key)
			_, list := s.List(ctx, "")
			_, url := s.URL(maago.Media{Key: key}, time.Hour)
			for _, err := range []error{put, open, s.Delete(ctx, key), list, url} {
				if err == nil || !strings.Contains(err.Error(), "Store."+field) || strings.Contains(err.Error(), "SECRET") {
					t.Errorf("%+v: %v", s, err)
				}
			}
		}
	}
	for _, ttl := range []time.Duration{0, 1500 * time.Millisecond, 8 * 24 * time.Hour} {
		if _, err := good.URL(maago.Media{Key: key}, ttl); err == nil {
			t.Errorf("a URL for %v: no error", ttl)
		}
	}
	for _, ttl := range []time.Duration{time.Second, 7 * 24 * time.Hour} {
		if _, err := good.URL(maago.Media{Key: key}, ttl); err != nil {
			t.Error(err)
		}
	}
}

// A URL downloads the file under the name ServeMedia gives it.
func TestDisposition(t *testing.T) {
	s := s3fake.New(t)
	ctx := maago.WithMedia(t.Context(), s)
	key := newKey()
	if _, err := s.Put(ctx, key, "image/png", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{`사진 "1".png`, "a.png", ""} {
		m := maago.Media{Key: key, Name: name, Type: "image/png"}
		w := httptest.NewRecorder()
		if err := maago.ServeMedia(w, httptest.NewRequest("GET", "/m", nil).WithContext(ctx), m); err != nil {
			t.Fatal(err)
		}
		u, err := s.URL(m, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if got, want := resp.Header.Get("Content-Disposition"), w.Header().Get("Content-Disposition"); resp.StatusCode != 200 || got != want {
			t.Errorf("%q: %d %s, want %s", name, resp.StatusCode, got, want)
		}
	}
}

func newKey() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
