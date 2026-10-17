package maago

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maago-web/maago/internal/rtcore"
)

func TestPrefix(t *testing.T) {
	t.Cleanup(func() { rtcore.Current.Store(nil) })
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
	rtcore.Current.Store(&rtcore.Process{Prefix: "/app", TZ: seoul}) // back to the first Handler's
	if Prefix() != "/app" {
		t.Errorf("Prefix() = %q", Prefix())
	}
	if rtcore.TimeZone() != seoul {
		t.Errorf("TimeZone() = %v", rtcore.TimeZone())
	}
	// The last Handler wins: one without a prefix, made after.
	serve(t, Config{Apps: []App{NewApp("b", route("GET", "/x", false, ok))}}, httptest.NewRequest("GET", "/x", nil))
	if Prefix() != "" || rtcore.TimeZone() != time.UTC {
		t.Errorf("after a second Handler: %q, %v", Prefix(), rtcore.TimeZone())
	}
	for _, bad := range []string{"app", "/app/", "/", "/a b", "/%2F", "/a?b", "/a//b"} {
		_, err := Handler(Config{DB: testDB, Apps: []App{NewApp("c", route("GET", "/x", false, ok))}, Prefix: bad})
		if err == nil || !strings.Contains(err.Error(), "Config.Prefix") {
			t.Errorf("Prefix %q: %v", bad, err)
		}
	}
}
