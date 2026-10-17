package rt

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maago-web/maago"
)

// One limit for every body: past Config.MaxBody, a form, JSON, and what a
// func(w, r) view reads answer 413, a 4xx OnError does not hear (D-139).
func TestMaxBody(t *testing.T) {
	form := func(w http.ResponseWriter, r *http.Request) error {
		_, err := FormValues(r, "title")
		return err
	}
	api := func(w http.ResponseWriter, r *http.Request) error {
		_, err := JSONValues(r, "title")
		return err
	}
	raw := func(w http.ResponseWriter, r *http.Request) error {
		_, err := io.ReadAll(r.Body)
		return err
	}
	apps := []maago.App{NewApp("a", nil, AppRoute{Method: "POST", Path: "/form", Name: "form", API: false, Serve: form}, AppRoute{Method: "POST", Path: "/api", Name: "api", API: true, Serve: api}, AppRoute{Method: "POST", Path: "/raw", Name: "raw", API: false, Serve: raw})}
	const limit = 100
	for _, c := range []struct {
		path, ct, body string
		code           int
	}{
		{"/form", "application/x-www-form-urlencoded", "title=" + strings.Repeat("a", limit-len("title=")), 200},
		{"/form", "application/x-www-form-urlencoded", "title=" + strings.Repeat("a", limit), 413},
		{"/api", "application/json", `{"title":"` + strings.Repeat("a", limit-len(`{"title":""}`)) + `"}`, 200},
		{"/api", "application/json", `{"title":"` + strings.Repeat("a", limit) + `"}`, 413},
		{"/api", "application/json", `{"title":"a"}` + strings.Repeat(" ", limit), 413}, // past it after the object
		{"/raw", "", strings.Repeat("a", limit), 200},
		{"/raw", "", strings.Repeat("a", limit+1), 413},
	} {
		r := httptest.NewRequest("POST", c.path, strings.NewReader(c.body))
		r.Header.Set("Content-Type", c.ct)
		w, reported := serve(t, maago.Config{Apps: apps, MaxBody: limit}, r)
		if w.Code != c.code || reported != nil {
			t.Errorf("%s of %d bytes: %d %q, OnError %v", c.path, len(c.body), w.Code, w.Body, reported)
		}
		if c.path == "/api" && c.code == 413 && (w.Header().Get("Content-Type") != "application/problem+json" || !strings.Contains(w.Body.String(), `"status":413`)) {
			t.Errorf("api 413: %v %q", w.Header(), w.Body)
		}
	}
	// 0 is 10MB: 10MB is read, a byte more is 413.
	for n, code := range map[int]int{10 << 20: 200, 10<<20 + 1: 413} {
		if w, _ := serve(t, maago.Config{Apps: apps}, httptest.NewRequest("POST", "/raw", strings.NewReader(strings.Repeat("a", n)))); w.Code != code {
			t.Errorf("%d bytes with the default: %d, want %d", n, w.Code, code)
		}
	}
	// A real server closes the connection after a body past the limit,
	// rather than read the rest of it.
	h, err := maago.Handler(maago.Config{DB: testDB, Apps: apps, MaxBody: limit})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/raw", "text/plain", strings.NewReader(strings.Repeat("a", 10*limit)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 413 || !resp.Close {
		t.Errorf("server: %d, Connection: close %t", resp.StatusCode, resp.Close)
	}
	if _, err := maago.Handler(maago.Config{DB: testDB, Apps: apps, MaxBody: -1}); err == nil || !strings.HasPrefix(err.Error(), "maago: Config.MaxBody is -1") {
		t.Errorf("negative: %v", err)
	}
}
