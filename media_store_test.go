package maago_test

import (
	"bytes"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/internal/mediatest"
	"github.com/maago-web/maago/rt"
)

// site serves a Handler made of cfg once the site's address is known:
// cfg makes it from that address.
func site(t *testing.T, prefix string, cfg func(siteURL string) maago.Config) string {
	t.Helper()
	var h http.Handler
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) }))
	t.Cleanup(srv.Close)
	c := cfg(srv.URL + prefix)
	c.DB = sql.OpenDB(nil) // no driver: the media route asks no database
	c.Apps = []maago.App{rt.NewApp("a", nil, rt.AppRoute{Method: "GET", Path: "/x", Name: "x", Serve: func(w http.ResponseWriter, r *http.Request) error { return nil }})}
	c.Prefix = prefix
	var err error
	if h, err = maago.Handler(c); err != nil {
		t.Fatal(err)
	}
	if prefix != "" {
		h = http.StripPrefix(prefix, h)
	}
	return srv.URL + prefix
}

// The local store passes the stores' test, its URLs served by Handler.
func TestLocalMediaStore(t *testing.T) {
	var media maago.LocalMedia
	site(t, "", func(siteURL string) maago.Config {
		media = maago.LocalMedia{Dir: t.TempDir(), SiteURL: siteURL}
		return maago.Config{Media: media}
	})
	mediatest.Store(t, media)
}

// A URL leads to the file under Config.Prefix, and from a worker's own
// LocalMedia when Handler has the shorthand MediaDir: the same process
// signs both.
func TestLocalMediaURLServed(t *testing.T) {
	dir := t.TempDir()
	put := func(media maago.LocalMedia) string {
		t.Helper()
		ctx := maago.WithMedia(t.Context(), media)
		key := "0123456789abcdef0123456789abcdef"
		if _, err := media.Put(ctx, key, "text/plain", bytes.NewReader([]byte("hi"))); err != nil {
			t.Fatal(err)
		}
		u, err := maago.MediaURL(ctx, maago.Media{Key: key, Name: "a.txt", Type: "text/plain"}, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	var prefixed maago.LocalMedia
	site(t, "/app", func(siteURL string) maago.Config {
		prefixed = maago.LocalMedia{Dir: dir, SiteURL: siteURL + "/"} // a last / is no matter
		return maago.Config{Media: prefixed}
	})
	short := site(t, "", func(string) maago.Config { return maago.Config{MediaDir: dir} })
	for _, u := range []string{put(prefixed), put(maago.LocalMedia{Dir: dir, SiteURL: short})} {
		resp, err := http.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || string(b) != "hi" {
			t.Errorf("GET %s: %d %q", u, resp.StatusCode, b)
		}
	}
}
