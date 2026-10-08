package auth

import (
	"context"
	"database/sql"
	"errors"
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

func init() {
	authlib.Iterations = 1000 // fast hashes in tests only
	SiteURL = "https://example.com"
}

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
		"not verified":   form("new@example.com", "wrong"),
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

// link is the one line of body that starts with prefix.
func link(t *testing.T, body, prefix string) string {
	t.Helper()
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	t.Fatalf("no link %s in\n%s", prefix, body)
	return ""
}

func TestSignup(t *testing.T) {
	h, db := site(t)
	box := maagotest.Mailbox(t)
	if w := do(h, "GET", "/signup", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `name="password"`) {
		t.Errorf("form: %d %s", w.Code, w.Body)
	}
	w := do(h, "POST", "/signup", form(" Ann@Example.com ", password), "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Check your email") || w.Header().Get("Set-Cookie") != "" || strings.Contains(w.Body.String(), password) {
		t.Fatalf("signup: %d %v\n%s", w.Code, w.Header(), w.Body)
	}
	sent := box.Messages()
	if len(sent) != 1 || sent[0].To != "ann@example.com" || sent[0].Subject != "Verify your email" {
		t.Fatalf("sent %v", sent)
	}
	l := link(t, sent[0].Body, "https://example.com/verify?token=")
	token := strings.TrimPrefix(l, "https://example.com/verify?token=")
	if _, err := GetUser(context.Background(), db, 1); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.QueryRow("SELECT hash FROM auth_token").Scan(&stored); err != nil || stored != authlib.HashToken(token) {
		t.Errorf("stored %q %v, want the token's hash", stored, err)
	}

	// Not verified yet: no sign-in, and the right password learns why.
	if w := do(h, "POST", "/login", form("ann@example.com", password), ""); !strings.Contains(w.Body.String(), "Verify your email first.") || w.Header().Get("Set-Cookie") != "" {
		t.Errorf("before verifying: %d %s", w.Code, w.Body)
	}
	// Opening the link, even twice, uses nothing up.
	for range 2 {
		if w := do(h, "GET", "/verify?token="+token, "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `value="`+token+`"`) {
			t.Fatalf("verify page: %d %s", w.Code, w.Body)
		}
	}
	verify := url.Values{"token": {token}}.Encode()
	if w := do(h, "POST", "/verify", verify, ""); w.Code != 303 || w.Header().Get("Location") != "/login" {
		t.Fatalf("verify: %d %s", w.Code, w.Body)
	}
	if w := do(h, "POST", "/verify", verify, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "This link is invalid or has expired.") {
		t.Errorf("verify again: %d %s", w.Code, w.Body)
	}
	if c := login(t, h, "ann@example.com", ""); whoami(h, c.Value) != "ann@example.com" {
		t.Error("not signed in")
	}
}

// Signing up with an email that has an account answers the same, adds no
// account, and tells the owner.
func TestSignupExisting(t *testing.T) {
	h, db := site(t)
	box := maagotest.Mailbox(t)
	newUser(t, db, "ann@example.com", nil)
	newUser(t, db, "new@example.com", func(in *NewUserInput) { in.VerifiedAt = nil }) // verified or not: until resending (task 8), the same
	fresh := do(h, "POST", "/signup", form("bob@example.com", "a long fresh password"), "")
	for i, email := range []string{"ANN@example.com ", "new@example.com"} {
		again := do(h, "POST", "/signup", form(email, "a long fresh password"), "")
		if again.Code != fresh.Code || again.Body.String() != fresh.Body.String() {
			t.Errorf("%s: answers differ:\n%s\n%s", email, fresh.Body, again.Body)
		}
		sent := box.Messages()
		if len(sent) != 2+i || sent[1+i].To != strings.ToLower(strings.TrimSpace(email)) || sent[1+i].Subject != "You already have an account" {
			t.Fatalf("sent %v", sent)
		}
		link(t, sent[1+i].Body, "https://example.com/login")
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM auth_user").Scan(&n); err != nil || n != 3 {
		t.Errorf("%d users, %v", n, err)
	}
}

// What the form refuses adds nothing and sends nothing.
func TestSignupRefuses(t *testing.T) {
	h, db := site(t)
	box := maagotest.Mailbox(t)
	for _, c := range []struct{ email, password, want string }{
		{"ann@example.com", "short", "This password is too short."},
		{"ann@example.com", "BaseBall", "This password is too common."},
		{"ann@example.com", "ann rides a bike", "This password is too similar to your email."},
		{"ann@example.com", strings.Repeat("가", 1400), "This password is too long."},
		{"not an email", "a long new password", "Enter a valid email address."},
		{"Ann <ann@example.com>", "a long new password", "Enter a valid email address."},
		{"앤@example.com", "a long new password", "Enter a valid email address."},
	} {
		w := do(h, "POST", "/signup", form(c.email, c.password), "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), c.want) || strings.Contains(w.Body.String(), `value="`+c.password) {
			t.Errorf("%s %.20q: %d\n%s", c.email, c.password, w.Code, w.Body)
		}
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM auth_user").Scan(&n); err != nil || n != 0 || len(box.Messages()) != 0 {
		t.Errorf("%d users, %v, sent %v", n, err, box.Messages())
	}
}

// A mail that fails is a 500 that leaves the account unverified; without
// SiteURL nothing is stored.
func TestSignupMailFails(t *testing.T) {
	h, db := site(t)
	old := authlib.Mail
	authlib.Mail = func(context.Context, authlib.Message) error { return errors.New("smtp down") }
	t.Cleanup(func() { authlib.Mail = old }) // after Mailbox's below: cleanups run last first
	if w := do(h, "POST", "/signup", form("ann@example.com", "a long new password"), ""); w.Code != 500 {
		t.Errorf("mail fails: %d", w.Code)
	}
	u, err := UserByEmail(context.Background(), db, UserByEmailParams{Email: "ann@example.com"})
	if err != nil || u.VerifiedAt != nil {
		t.Errorf("%v %v", u, err)
	}

	box := maagotest.Mailbox(t)
	t.Cleanup(func() { SiteURL = "https://example.com" })
	for _, SiteURL = range []string{"", "example.com", "https://", "ftp://example.com", "https://example.com/app", "https://ann@example.com", "https://example.com?x=1"} {
		if w := do(h, "POST", "/signup", form("bob@example.com", "a long new password"), ""); w.Code != 500 {
			t.Errorf("SiteURL %q: %d", SiteURL, w.Code)
		}
	}
	if _, err := UserByEmail(context.Background(), db, UserByEmailParams{Email: "bob@example.com"}); err != sql.ErrNoRows || len(box.Messages()) != 0 {
		t.Errorf("stored without SiteURL: %v, sent %v", err, box.Messages())
	}
	SiteURL = "https://example.com/" // a slash at the end is no path
	if w := do(h, "POST", "/signup", form("bob@example.com", "a long new password"), ""); w.Code != 200 || len(box.Messages()) != 1 || !strings.HasPrefix(link(t, box.Messages()[0].Body, "https://"), "https://example.com/verify?token=") {
		t.Errorf("SiteURL with a slash: %d %v", w.Code, box.Messages())
	}
}

// A token works only for its purpose, until it expires; making a token
// removes the expired ones.
func TestVerifyTokens(t *testing.T) {
	h, db := site(t)
	ctx := context.Background()
	ann := newUser(t, db, "ann@example.com", func(in *NewUserInput) { in.VerifiedAt = nil })
	add := func(purpose string, expires time.Time) (string, Token) {
		token, hash := authlib.NewToken()
		tk, err := InsertToken(ctx, db, NewTokenInput{UserID: ann.ID, Purpose: purpose, Hash: hash, ExpiresAt: expires})
		if err != nil {
			t.Fatal(err)
		}
		return token, tk
	}
	expired, old := add("verify", time.Now().Add(-time.Second))
	reset, _ := add("reset", time.Now().Add(time.Hour))
	for name, token := range map[string]string{"expired": expired, "reset": reset, "garbage": "x", "empty": ""} {
		w := do(h, "POST", "/verify", url.Values{"token": {token}}.Encode(), "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), "This link is invalid or has expired.") {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	for _, path := range []string{"/verify", "/verify?token=x"} {
		if w := do(h, "GET", path, "", ""); w.Code != 200 {
			t.Errorf("GET %s: %d", path, w.Code)
		}
	}
	if u, _ := GetUser(ctx, db, ann.ID); u.VerifiedAt != nil {
		t.Error("verified by a token that does not verify")
	}
	if _, err := newToken(ctx, db, ann.ID, "verify", time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := GetToken(ctx, db, old.ID); err != sql.ErrNoRows {
		t.Errorf("the expired token is still there: %v", err)
	}
}

// Of two requests with one token at once, one verifies and the other is
// told the link is used.
func TestVerifyOnce(t *testing.T) {
	h, db := site(t)
	ann := newUser(t, db, "ann@example.com", func(in *NewUserInput) { in.VerifiedAt = nil })
	token, err := newToken(context.Background(), db, ann.ID, "verify", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	codes := make(chan int, 2)
	for range 2 {
		go func() { codes <- do(h, "POST", "/verify", url.Values{"token": {token}}.Encode(), "").Code }()
	}
	if a, b := <-codes, <-codes; a+b != 303+200 {
		t.Errorf("codes %d and %d, want one 303 and one 200", a, b)
	}
}

// Under a prefix, the links of the mails have it.
func TestMailLinksPrefix(t *testing.T) {
	_, db := site(t)
	box := maagotest.Mailbox(t)
	h, err := maago.Handler(maago.Config{DB: db, Apps: []maago.App{App}, Auth: Current, LoginURL: "/app/login", Prefix: "/app"})
	if err != nil {
		t.Fatal(err)
	}
	sh := http.StripPrefix("/app", h)
	newUser(t, db, "ann@example.com", nil)
	do(sh, "POST", "/app/signup", form("bob@example.com", "a long new password"), "")
	do(sh, "POST", "/app/signup", form("ann@example.com", "a long new password"), "")
	sent := box.Messages()
	if len(sent) != 2 {
		t.Fatalf("sent %v", sent)
	}
	link(t, sent[0].Body, "https://example.com/app/verify?token=")
	link(t, sent[1].Body, "https://example.com/app/login")
}
