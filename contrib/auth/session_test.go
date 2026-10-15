package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewToken(t *testing.T) {
	a, ha := NewToken()
	b, _ := NewToken()
	if len(a) != 43 || a == b || !wellFormed(a) {
		t.Errorf("tokens %q %q", a, b)
	}
	if ha != HashToken(a) || len(ha) != 64 || ha == a {
		t.Errorf("hash %q of %q", ha, a)
	}
}

func TestSessionCookie(t *testing.T) {
	c := SessionCookie("tok", "/app", 14*24*time.Hour, true)
	if c.Name != "session" || c.Value != "tok" || c.Path != "/app" || c.MaxAge != 14*24*3600 ||
		!c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("got %+v", c)
	}
	if c := SessionCookie("", "/", -1, true); c.MaxAge != -1 || c.Path != "/" || c.String() != "session=; Path=/; Max-Age=0; HttpOnly; Secure; SameSite=Lax" {
		t.Errorf("removing: %s", c)
	}
	if c := SessionCookie("tok", "/", time.Hour, false); c.String() != "session=tok; Path=/; Max-Age=3600; HttpOnly; SameSite=Lax" {
		t.Errorf("on http: %s", c)
	}
}

func TestFromSession(t *testing.T) {
	token, hash := NewToken()
	api, apiHash := NewToken()
	down := errors.New("database down")
	var asked string
	find := func(name string, users map[string]string) func(context.Context, string) (string, error) {
		return func(ctx context.Context, h string) (string, error) {
			asked = name
			if h == HashToken("broken"+token[6:]) {
				return "", down
			}
			if u, ok := users[h]; ok {
				return u, nil
			}
			return "", sql.ErrNoRows
		}
	}
	auth := FromSession(find("session", map[string]string{hash: "ann"}), find("token", map[string]string{apiHash: "bot"}))
	missing := strings.Repeat("A", 43)
	for _, c := range []struct {
		name, cookie, authorization string
		user                        any
		err                         error
		asks                        string
	}{
		{"no cookie", "", "", nil, nil, ""},
		{"empty", "session=", "", nil, nil, ""},
		{"not a token", "session=attacker", "", nil, nil, ""},
		{"too long", "session=" + token + "A", "", nil, nil, ""},
		{"padded", "session=" + token[:42] + "=", "", nil, nil, ""},
		{"session", "session=" + token, "", "ann", nil, "session"},
		{"no such session", "session=" + missing, "", nil, nil, "session"},
		{"database fails", "session=broken" + token[6:], "", nil, down, "session"},
		{"bearer", "", "Bearer " + api, "bot", nil, "token"},
		{"bearer over the cookie", "session=" + token, "Bearer " + api, "bot", nil, "token"},
		{"no such token, with a cookie", "session=" + token, "Bearer " + missing, nil, nil, "token"},
		{"bad token, with a cookie", "session=" + token, "Bearer attacker", nil, nil, ""},
		{"lower case, two spaces", "", "bearer  " + api, "bot", nil, "token"},
		{"another scheme", "session=" + token, "Basic YW5uOnB3", "ann", nil, "session"},
		{"another scheme, Bearer-like", "session=" + token, "Bearer" + api, "ann", nil, "session"},
		{"Bearer without a token", "session=" + token, "Bearer", nil, nil, ""},
		{"Bearer with two", "session=" + token, "Bearer " + api + " " + api, nil, nil, ""},
	} {
		asked = ""
		r := httptest.NewRequest("GET", "/", nil)
		if c.cookie != "" {
			r.Header.Set("Cookie", c.cookie)
		}
		if c.authorization != "" {
			r.Header.Set("Authorization", c.authorization)
		}
		u, err := auth(r)
		if u != c.user || err != c.err || asked != c.asks {
			t.Errorf("%s: %v %v, asked %q", c.name, u, err, asked)
		}
	}

	// Without a token func, the header is not read.
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Cookie", "session="+token)
	r.Header.Set("Authorization", "Bearer "+api)
	if u, err := FromSession(find("session", map[string]string{hash: "ann"}), nil)(r); u != "ann" || err != nil {
		t.Errorf("no token func: %v %v", u, err)
	}
}

func TestSafeNext(t *testing.T) {
	for next, want := range map[string]string{
		"/":                 "/",
		"/posts/1?page=2#c": "/posts/1?page=2#c",
		"/app/posts":        "/app/posts",
		"":                  "/home",
		"posts":             "/home",
		"//evil.example":    "/home",
		"/\\evil.example":   "/home",
		"https://evil":      "/home",
		"/%2F%2Fevil":       "/home",
		"/%2f/evil":         "/home",
		"/%5Cevil":          "/home",
		"/a\nb":             "/home",
		"/a\x7fb":           "/home",
		"\t//evil":          "/home",
	} {
		if got := SafeNext(next, "/home"); got != want {
			t.Errorf("SafeNext(%q) = %q, want %q", next, got, want)
		}
	}
}
