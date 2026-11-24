package s3

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/maago-web/maago/internal/mediatest"
)

// The examples of AWS's Signature Version 4 documents, read from the
// Wayback Machine's copies of
// https://docs.aws.amazon.com/AmazonS3/latest/API/sig-v4-header-based-auth.html
// and https://docs.aws.amazon.com/AmazonS3/latest/API/sigv4-query-string-auth.html.
func TestVectors(t *testing.T) {
	s := Store{Region: "us-east-1", AccessKeyID: "AKIAIOSFODNN7EXAMPLE", SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}
	now := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name, url, rng, want string
	}{
		{"GET Object", "https://examplebucket.s3.amazonaws.com/test.txt", "bytes=0-9",
			"AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request,SignedHeaders=host;range;x-amz-content-sha256;x-amz-date,Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"},
		{"GET Bucket", "https://examplebucket.s3.amazonaws.com/?max-keys=2&prefix=J", "",
			"AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request,SignedHeaders=host;x-amz-content-sha256;x-amz-date,Signature=34b48302e7b5fa45bde8084f4b7868a86f0a534bc59db6670ed5711ef69dc6f7"},
	} {
		req, err := http.NewRequest("GET", c.url, nil)
		if err != nil {
			t.Fatal(err)
		}
		if c.rng != "" {
			req.Header.Set("Range", c.rng)
		}
		s.sign(req, emptyHash, now)
		if got := req.Header.Get("Authorization"); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	u, _ := url.Parse("https://examplebucket.s3.amazonaws.com/test.txt")
	signed, err := url.Parse(s.presign(u, now, 86400*time.Second, nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := signed.Query().Get("X-Amz-Signature"); got != "aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404" {
		t.Errorf("presigned URL: %s", signed)
	}
}

func TestEscape(t *testing.T) {
	for b := range 256 {
		s := string([]byte{byte(b)}) // a byte, not a rune
		unreserved := 'A' <= b && b <= 'Z' || 'a' <= b && b <= 'z' || '0' <= b && b <= '9' || strings.ContainsRune("-._~", rune(b))
		for _, slash := range []bool{false, true} {
			want := "%" + strings.ToUpper(hex.EncodeToString([]byte{byte(b)}))
			if unreserved || slash && b == '/' {
				want = s
			}
			if got := escape(s, slash); got != want {
				t.Errorf("escape(%q, %v) = %q, want %q", s, slash, got, want)
			}
		}
	}
	if got := escape("a b~/c", false); got != "a%20b~%2Fc" {
		t.Errorf("escape: %q", got)
	}
}

// Reading asks for the bytes from where it reads, and takes nothing
// else: not the whole file for a Range, nor a range from elsewhere.
func TestReads(t *testing.T) {
	body := strings.Repeat("0123456789", 100) // more than one Read of io.ReadAll
	mod := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	var gets int
	answer := "range" // or "whole", "elsewhere", "no time"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if answer != "no time" {
			w.Header().Set("Last-Modified", mod.Format(http.TimeFormat))
		}
		if r.Method == "GET" {
			gets++
		}
		switch {
		case r.Method == "HEAD":
			w.Header().Set("Content-Length", "1000")
		case answer == "whole":
			io.WriteString(w, body)
		case answer == "elsewhere":
			w.Header().Set("Content-Range", "bytes 0-999/1000")
			w.WriteHeader(http.StatusPartialContent)
			io.WriteString(w, body)
		default:
			http.ServeContent(w, r, "", mod, strings.NewReader(body))
		}
	}))
	defer srv.Close()
	s := Store{Endpoint: srv.URL, Region: "us-east-1", Bucket: "media", AccessKeyID: "id", SecretAccessKey: "secret"}
	key := strings.Repeat("a", 32)

	f, err := s.Open(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	if info, _ := f.Stat(); info.Size() != 1000 || !info.ModTime().Equal(mod) {
		t.Errorf("stat: %d %v", info.Size(), info.ModTime())
	}
	if b, err := io.ReadAll(f); string(b) != body || err != nil || gets != 1 {
		t.Errorf("read all: %d bytes, %v, in %d GETs", len(b), err, gets)
	}
	f.Seek(5, io.SeekStart)
	if b, err := io.ReadAll(f); string(b) != body[5:] || err != nil || gets != 2 {
		t.Errorf("read from 5: %d bytes, %v, in %d GETs", len(b), err, gets)
	}
	f.Close()

	for _, a := range []string{"whole", "elsewhere"} {
		answer = a
		f, err := s.Open(t.Context(), key)
		if err != nil {
			t.Fatal(err)
		}
		f.Seek(5, io.SeekStart)
		if b, err := io.ReadAll(f); err == nil {
			t.Errorf("%s: read %d bytes, no error", a, len(b))
		}
		f.Close()
	}
	answer = "no time"
	if _, err := s.Open(t.Context(), key); err == nil {
		t.Error("open of a file of no Last-Modified: no error")
	}
}

// A real server, when MAAGO_TEST_S3_URL names one: http://KEY:SECRET@host:port,
// region us-east-1. Each run makes its own bucket, so runs sharing the
// server do not see each other's files.
func TestServer(t *testing.T) {
	raw := os.Getenv("MAAGO_TEST_S3_URL")
	switch {
	case raw != "":
	case os.Getenv("CI") != "":
		t.Fatal("MAAGO_TEST_S3_URL is not set; CI must run the S3 tests")
	default:
		t.Skip("MAAGO_TEST_S3_URL is not set; set it to an S3 server, such as http://KEY:SECRET@127.0.0.1:9000, to run this test")
	}
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		t.Fatal("MAAGO_TEST_S3_URL is not http://KEY:SECRET@host:port")
	}
	secret, _ := u.User.Password()
	b := make([]byte, 8)
	rand.Read(b)
	s := Store{Endpoint: u.Scheme + "://" + u.Host, Region: "us-east-1", Bucket: "maago-" + hex.EncodeToString(b), AccessKeyID: u.User.Username(), SecretAccessKey: secret}
	resp, err := s.do(t.Context(), "PUT", "", nil, nil, nil, 0, emptyHash)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	t.Cleanup(func() {
		ctx := context.Background() // t.Context is done by now
		for {
			page, err := s.List(ctx, "")
			if err != nil {
				t.Fatal(err)
			}
			if len(page) == 0 {
				break
			}
			for _, f := range page {
				if err := s.Delete(ctx, f.Name()); err != nil {
					t.Fatal(err)
				}
			}
		}
		resp, err := s.do(ctx, "DELETE", "", nil, nil, nil, 0, emptyHash)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	})
	mediatest.Store(t, s)
}
