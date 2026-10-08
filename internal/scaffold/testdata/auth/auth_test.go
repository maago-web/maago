package auth

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/maago-web/maago"
	authlib "github.com/maago-web/maago/contrib/auth"
	"github.com/maago-web/maago/maagotest"
)

func init() { authlib.Iterations = 1000 } // fast hashes in tests only

const password = "correct horse battery staple"

// site serves this app on a new database with the project's migrations,
// with a page that says who is signed in.
func site(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	db := maagotest.DB(t)
	h, err := maago.Handler(maago.Config{DB: db, Apps: []maago.App{App}, Auth: Current, LoginURL: "/login"})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/", h)
	mux.HandleFunc("GET /whoami", func(w http.ResponseWriter, r *http.Request) {
		u, err := Current(r.WithContext(maago.WithDB(r.Context(), db)))
		if err != nil {
			t.Error(err)
		}
		if u, ok := u.(User); ok {
			w.Write([]byte(u.Email))
		}
	})
	return mux, db
}

// newUser adds an account that may sign in, unless change says otherwise.
func newUser(t *testing.T, db *sql.DB, email string, change func(*NewUserInput)) User {
	t.Helper()
	hash, err := authlib.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	in := NewUserInput{Email: email, PasswordHash: hash, VerifiedAt: &now}
	if change != nil {
		change(&in)
	}
	u, err := InsertUser(context.Background(), db, in)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// do sends a request, with the session cookie if one is given.
func do(h http.Handler, method, path, body, session string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if session != "" {
		r.AddCookie(&http.Cookie{Name: authlib.CookieName, Value: session})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func form(email, password string) string {
	return url.Values{"email": {email}, "password": {password}}.Encode()
}

// login signs in and returns the session cookie it set.
func login(t *testing.T, h http.Handler, email, session string) *http.Cookie {
	t.Helper()
	w := do(h, "POST", "/login", form(email, password), session)
	cookies := w.Result().Cookies()
	if w.Code != 303 || len(cookies) != 1 {
		t.Fatalf("login: %d %v\n%s", w.Code, cookies, w.Body)
	}
	return cookies[0]
}

func whoami(h http.Handler, session string) string {
	return do(h, "GET", "/whoami", "", session).Body.String()
}

func TestLogin(t *testing.T) {
	h, db := site(t)
	newUser(t, db, "ann@example.com", nil)

	if w := do(h, "GET", "/login?next=/posts", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `action="/login?next=%2fposts"`) {
		t.Errorf("form: %d %s", w.Code, w.Body)
	}

	c := login(t, h, " Ann@Example.com ", "")
	if c.Name != authlib.CookieName || c.Path != "/" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.MaxAge != int(SessionAge/time.Second) {
		t.Errorf("cookie %s", c)
	}
	if got := whoami(h, c.Value); got != "ann@example.com" {
		t.Errorf("signed in as %q", got)
	}
	if got := whoami(h, ""); got != "" {
		t.Errorf("no cookie: %q", got)
	}
}

// A sign-in that fails says the same, whatever failed, and sets nothing.
func TestLoginFails(t *testing.T) {
	h, db := site(t)
	newUser(t, db, "ann@example.com", nil)
	newUser(t, db, "new@example.com", func(in *NewUserInput) { in.VerifiedAt = nil })
	newUser(t, db, "gone@example.com", func(in *NewUserInput) { in.IsActive = maago.Set(false) })
	for name, body := range map[string]string{
		"wrong password": form("ann@example.com", "wrong"),
		"no account":     form("nobody@example.com", password),
		"not verified":   form("new@example.com", password),
		"not active":     form("gone@example.com", password),
	} {
		w := do(h, "POST", "/login", body, "")
		b := w.Body.String()
		if w.Code != 200 || !strings.Contains(b, `<p class="error">Incorrect email or password.</p>`) || w.Header().Get("Set-Cookie") != "" {
			t.Errorf("%s: %d %v\n%s", name, w.Code, w.Header(), b)
		}
		if strings.Contains(b, "wrong") || strings.Contains(b, password) {
			t.Errorf("%s: the password is drawn back:\n%s", name, b)
		}
	}
}

// Signing in makes a new session, never the one the request brought
// (session fixation), and signing out ends the user's sessions.
func TestSessions(t *testing.T) {
	h, db := site(t)
	ann := newUser(t, db, "ann@example.com", nil)
	newUser(t, db, "bob@example.com", nil)
	bob := login(t, h, "bob@example.com", "")

	planted := strings.Repeat("A", 43)
	s1 := login(t, h, "ann@example.com", planted)
	if s1.Value == planted || whoami(h, planted) != "" {
		t.Errorf("the planted cookie became the session")
	}
	s2 := login(t, h, "ann@example.com", s1.Value)
	if s2.Value == s1.Value {
		t.Errorf("signing in again kept the session %q", s1.Value)
	}
	if got := whoami(h, s1.Value); got != "ann@example.com" { // a sign-in ends only expired sessions
		t.Errorf("the first session after the second sign-in: %q", got)
	}

	w := do(h, "POST", "/logout", "", s2.Value)
	gone := w.Result().Cookies()
	if w.Code != 303 || w.Header().Get("Location") != "/login" || len(gone) != 1 || gone[0].MaxAge != -1 || gone[0].Path != "/" {
		t.Errorf("logout: %d %v", w.Code, w.Header())
	}
	for _, s := range []string{s1.Value, s2.Value} {
		if got := whoami(h, s); got != "" {
			t.Errorf("signed out, the session still is %q", got)
		}
	}
	if got := whoami(h, bob.Value); got != "bob@example.com" {
		t.Errorf("ann signed out bob: %q", got)
	}
	if w := do(h, "POST", "/logout", "", s2.Value); w.Code != 303 {
		t.Errorf("logout without a session: %d", w.Code)
	}

	// A session ends when it expires, or when its user may not sign in.
	s3 := login(t, h, "ann@example.com", "")
	ann.VerifiedAt = nil
	if err := UpdateUser(context.Background(), db, &ann); err != nil {
		t.Fatal(err)
	}
	if got := whoami(h, s3.Value); got != "" {
		t.Errorf("unverified, still %q", got)
	}
	now := time.Now()
	ann.VerifiedAt, ann.IsActive = &now, false
	if err := UpdateUser(context.Background(), db, &ann); err != nil {
		t.Fatal(err)
	}
	if got := whoami(h, s3.Value); got != "" {
		t.Errorf("closed, still %q", got)
	}
	ann.IsActive = true
	if err := UpdateUser(context.Background(), db, &ann); err != nil {
		t.Fatal(err)
	}
	if got := whoami(h, s3.Value); got != "ann@example.com" {
		t.Errorf("verified and active again: %q", got)
	}
	token, hash := authlib.NewToken()
	expired, err := InsertSession(context.Background(), db, NewSessionInput{UserID: ann.ID, Hash: hash, ExpiresAt: now.Add(-time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if got := whoami(h, token); got != "" {
		t.Errorf("expired, still %q", got)
	}
	login(t, h, "ann@example.com", "") // removes expired sessions
	if _, err := GetSession(context.Background(), db, expired.ID); err != sql.ErrNoRows {
		t.Errorf("the expired session is still there: %v", err)
	}
}

// A hash with other rounds is redone at sign-in.
func TestRehash(t *testing.T) {
	h, db := site(t)
	authlib.Iterations = 500
	defer func() { authlib.Iterations = 1000 }()
	ann := newUser(t, db, "ann@example.com", nil)
	authlib.Iterations = 1000
	login(t, h, "ann@example.com", "")
	got, err := GetUser(context.Background(), db, ann.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.PasswordHash, "pbkdf2_sha256$1000$") {
		t.Errorf("hash %q", got.PasswordHash)
	}
}

// After sign-in the browser goes to next, if it is a path of this site.
func TestNext(t *testing.T) {
	h, db := site(t)
	newUser(t, db, "ann@example.com", nil)
	for next, want := range map[string]string{
		"":                "/",
		"/posts?page=2":   "/posts?page=2",
		"//evil.example":  "/",
		"https://evil":    "/",
		"/\\evil.example": "/",
		"/%2F%2Fevil":     "/",
	} {
		w := do(h, "POST", "/login?next="+url.QueryEscape(next), form("ann@example.com", password), "")
		if w.Code != 303 || w.Header().Get("Location") != want {
			t.Errorf("next %q: %d Location %q, want %q", next, w.Code, w.Header().Get("Location"), want)
		}
	}
}
