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
	c := SessionCookie("tok", "/app", 14*24*time.Hour)
	if c.Name != "session" || c.Value != "tok" || c.Path != "/app" || c.MaxAge != 14*24*3600 ||
		!c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("got %+v", c)
	}
	if c := SessionCookie("", "/", -1); c.MaxAge != -1 || c.Path != "/" || c.String() != "session=; Path=/; Max-Age=0; HttpOnly; Secure; SameSite=Lax" {
		t.Errorf("removing: %s", c)
	}
}

func TestFromSession(t *testing.T) {
	token, hash := NewToken()
	down := errors.New("database down")
	var asked bool
	auth := FromSession(func(ctx context.Context, h string) (string, error) {
		asked = true
		switch h {
		case hash:
			return "ann", nil
		case HashToken("broken" + token[6:]):
			return "", down
		}
		return "", sql.ErrNoRows
	})
	for _, c := range []struct {
		name, cookie string
		user         any
		err          error
		asks         bool
	}{
		{"no cookie", "", nil, nil, false},
		{"empty", "session=", nil, nil, false},
		{"not a token", "session=attacker", nil, nil, false},
		{"too long", "session=" + token + "A", nil, nil, false},
		{"padded", "session=" + token[:42] + "=", nil, nil, false},
		{"session", "session=" + token, "ann", nil, true},
		{"no such session", "session=" + strings.Repeat("A", 43), nil, nil, true},
		{"database fails", "session=broken" + token[6:], nil, down, true},
	} {
		asked = false
		r := httptest.NewRequest("GET", "/", nil)
		if c.cookie != "" {
			r.Header.Set("Cookie", c.cookie)
		}
		u, err := auth(r)
		if u != c.user || err != c.err || asked != c.asks {
			t.Errorf("%s: %v %v, asked %v", c.name, u, err, asked)
		}
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
