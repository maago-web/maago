package maago

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPrefix(t *testing.T) {
	t.Cleanup(func() { current.Store(nil) })
	login := route("GET", "/secret", false, ok)
	login.LoginRequired = true
	seoul := time.FixedZone("KST", 9*60*60)
	cfg := Config{Apps: []App{NewApp("a", login)}, Auth: fakeAuth, LoginURL: "/app/login", Prefix: "/app", TimeZone: seoul}
	w, _ := serve(t, cfg, httptest.NewRequest("GET", "/secret?x=1", nil)) // the proxy took /app off
	if got := w.Header().Get("Location"); got != "/app/login?next=%2Fapp%2Fsecret%3Fx%3D1" {
		t.Errorf("login: %q", got)
	}
	if _, err := Handler(Config{DB: testDB, Apps: []App{NewApp("d", route("GET", "/x", false, ok))}, Prefix: "/app/sub"}); err != nil {
		t.Errorf("/app/sub: %v", err)
	}
	current.Store(&process{"/app", seoul, nil}) // back to the first Handler's
	if Prefix() != "/app" {
		t.Errorf("Prefix() = %q", Prefix())
	}
	u, err := URLFunc(map[string]string{"post": "/posts/{id}", "home": "/{$}"})("post", 7)
	if home, _ := URLFunc(map[string]string{"home": "/{$}"})("home"); err != nil || u != "/app/posts/7" || home != "/app/" {
		t.Errorf("url: %q %q %v", u, home, err)
	}
	at := time.Date(2026, 10, 1, 0, 30, 0, 0, time.UTC)
	for _, c := range []struct {
		t    any
		want string
	}{{at, "2026-10-01 09:30"}, {&at, "2026-10-01 09:30"}, {(*time.Time)(nil), ""}, {time.Time{}, ""}} {
		if got, err := DateFunc("2006-01-02 15:04", c.t); err != nil || got != c.want {
			t.Errorf("date %v: %q %v", c.t, got, err)
		}
	}
	if _, err := DateFunc("2006", 3); err == nil {
		t.Error("date of an int")
	}

	// The last Handler wins: one without a prefix, made after.
	serve(t, Config{Apps: []App{NewApp("b", route("GET", "/x", false, ok))}}, httptest.NewRequest("GET", "/x", nil))
	if got, _ := DateFunc("15:04", at); Prefix() != "" || got != "00:30" {
		t.Errorf("after a second Handler: %q, %q", Prefix(), got)
	}
	for _, bad := range []string{"app", "/app/", "/", "/a b", "/%2F", "/a?b", "/a//b"} {
		_, err := Handler(Config{DB: testDB, Apps: []App{NewApp("c", route("GET", "/x", false, ok))}, Prefix: bad})
		if err == nil || !strings.Contains(err.Error(), "Config.Prefix") {
			t.Errorf("Prefix %q: %v", bad, err)
		}
	}
}
