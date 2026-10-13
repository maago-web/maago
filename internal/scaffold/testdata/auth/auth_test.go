package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
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
	return doFrom(h, "192.0.2.1", method, path, body, session)
}

// doFrom sends a request from the IP ip.
func doFrom(h http.Handler, ip, method, path, body, session string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = ip + ":1234"
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
	ann := newUser(t, db, "ann@example.com", nil)
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
	if got, _ := GetUser(context.Background(), db, ann.ID); got.PasswordHash != ann.PasswordHash {
		t.Error("a wrong password changed the hash")
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

	// Another algorithm's hash, as Django's PBKDF2-SHA1, is redone too.
	bob := newUser(t, db, "bob@example.com", func(in *NewUserInput) {
		in.PasswordHash = "pbkdf2_sha1$1000$lowsalt1234567890ab$FCbA1J14Exq3OYBLrlM69kCJN9w=" // of "low"
	})
	if w := do(h, "POST", "/login", form("bob@example.com", "low"), ""); w.Code != 303 {
		t.Fatalf("sign in with the SHA1 hash: %d", w.Code)
	}
	if got, _ := GetUser(context.Background(), db, bob.ID); !strings.HasPrefix(got.PasswordHash, "pbkdf2_sha256$1000$") {
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
	link(t, sent[0].Body, "https://example.com/password/reset")
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
		if w := do(h, "GET", "/verify?token="+token, "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `value="`+token+`"`) || !strings.Contains(w.Body.String(), `name="password"`) {
			t.Fatalf("verify page: %d %s", w.Code, w.Body)
		}
	}
	resend(t, h, box, "ann@example.com") // a second link, which verifying removes
	w = do(h, "POST", "/verify", verify(token, password), "")
	cookies := w.Result().Cookies()
	if w.Code != 303 || w.Header().Get("Location") != "/" || len(cookies) != 1 || whoami(h, cookies[0].Value) != "ann@example.com" {
		t.Fatalf("verify: %d %v %s", w.Code, cookies, w.Body)
	}
	if count(t, db, "SELECT count(*) FROM auth_token") != 0 {
		t.Error("verification tokens are left")
	}
	if w := do(h, "POST", "/verify", verify(token, password), ""); w.Code != 200 || !strings.Contains(w.Body.String(), "This link is invalid or has expired.") {
		t.Errorf("verify again: %d %s", w.Code, w.Body)
	}
	if c := login(t, h, "ann@example.com", ""); whoami(h, c.Value) != "ann@example.com" {
		t.Error("not signed in")
	}

	// On to a reset, the whole way of an account in one chain.
	reset := resetLink(t, h, box, "ann@example.com")
	const fresh = "a fresh secret phrase"
	if w := do(h, "POST", "/password/reset/confirm", confirm(reset, fresh), ""); w.Code != 303 {
		t.Fatalf("reset: %d %s", w.Code, w.Body)
	}
	if w := do(h, "POST", "/login", form("ann@example.com", password), ""); w.Code != 200 {
		t.Errorf("the old password after the reset: %d", w.Code)
	}
	if w := do(h, "POST", "/login", form("ann@example.com", fresh), ""); w.Code != 303 {
		t.Errorf("the new password after the reset: %d", w.Code)
	}
}

func verify(token, password string) string {
	return url.Values{"token": {token}, "password": {password}}.Encode()
}

// resend asks for the verification mail of email again, and returns the
// token of the mail.
func resend(t *testing.T, h http.Handler, box *maagotest.Inbox, email string) string {
	t.Helper()
	before := len(box.Messages())
	if w := do(h, "POST", "/verify/resend", url.Values{"email": {email}}.Encode(), ""); w.Code != 200 {
		t.Fatalf("resend: %d %s", w.Code, w.Body)
	}
	sent := box.Messages()
	if len(sent) != before+1 || sent[before].Subject != "Verify your email" {
		t.Fatalf("sent %v", sent)
	}
	return strings.TrimPrefix(link(t, sent[before].Body, "https://example.com/verify?token="), "https://example.com/verify?token=")
}

// Signing up with an email that has an account answers the same, adds no
// account, and tells the owner: to sign in, or, not verified yet, a new
// link to verify, with the password unchanged.
func TestSignupExisting(t *testing.T) {
	h, db := site(t)
	box := maagotest.Mailbox(t)
	newUser(t, db, "ann@example.com", nil)
	nu := newUser(t, db, "new@example.com", func(in *NewUserInput) { in.VerifiedAt = nil })
	fresh := do(h, "POST", "/signup", form("bob@example.com", "a long fresh password"), "")
	for i, c := range []struct{ email, subject, link string }{
		{"ANN@example.com ", "You already have an account", "https://example.com/login"},
		{"new@example.com", "Verify your email", "https://example.com/verify?token="},
	} {
		again := do(h, "POST", "/signup", form(c.email, "a long fresh password"), "")
		if again.Code != fresh.Code || again.Body.String() != fresh.Body.String() {
			t.Errorf("%s: answers differ:\n%s\n%s", c.email, fresh.Body, again.Body)
		}
		sent := box.Messages()
		if len(sent) != 2+i || sent[1+i].To != strings.ToLower(strings.TrimSpace(c.email)) || sent[1+i].Subject != c.subject {
			t.Fatalf("sent %v", sent)
		}
		link(t, sent[1+i].Body, c.link)
	}
	if got, _ := GetUser(context.Background(), db, nu.ID); got.PasswordHash != nu.PasswordHash {
		t.Error("signing up again changed the password")
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
	gone := newUser(t, db, "gone@example.com", func(in *NewUserInput) { in.VerifiedAt, in.IsActive = nil, maago.Set(false) })
	closed, err := newToken(ctx, db, gone.ID, "verify", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for name, token := range map[string]string{"expired": expired, "reset": reset, "closed": closed, "garbage": "x", "empty": ""} {
		w := do(h, "POST", "/verify", verify(token, password), "")
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
	if u, _ := GetUser(ctx, db, gone.ID); u.VerifiedAt != nil || count(t, db, "SELECT count(*) FROM auth_session") != 0 {
		t.Error("a closed account verified")
	}
	// A password changed since it was checked verifies nothing.
	if n, err := VerifyUser(ctx, db, VerifyUserParams{ID: ann.ID, PasswordHash: "another", Now: time.Now()}); n != 0 || err != nil {
		t.Errorf("verified with another password: %d %v", n, err)
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
		go func() { codes <- do(h, "POST", "/verify", verify(token, password), "").Code }()
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
	link(t, sent[0].Body, "https://example.com/app/password/reset")
	link(t, sent[1].Body, "https://example.com/app/login")
	do(sh, "POST", "/app/password/reset", url.Values{"email": {"ann@example.com"}}.Encode(), "")
	if sent = box.Messages(); len(sent) != 3 {
		t.Fatalf("sent %v", sent)
	}
	link(t, sent[2].Body, "https://example.com/app/password/reset/confirm?token=")
}

func count(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// resetLink asks for a reset of email and returns the token of the mail.
func resetLink(t *testing.T, h http.Handler, box *maagotest.Inbox, email string) string {
	t.Helper()
	before := len(box.Messages())
	if w := do(h, "POST", "/password/reset", url.Values{"email": {email}}.Encode(), ""); w.Code != 200 || w.Header().Get("Set-Cookie") != "" {
		t.Fatalf("reset: %d %v", w.Code, w.Header())
	}
	sent := box.Messages()
	if len(sent) != before+1 || sent[before].Subject != "Reset your password" {
		t.Fatalf("sent %v", sent)
	}
	return strings.TrimPrefix(link(t, sent[before].Body, "https://example.com/password/reset/confirm?token="), "https://example.com/password/reset/confirm?token=")
}

func confirm(token, password string) string {
	return url.Values{"token": {token}, "password": {password}}.Encode()
}

func TestPasswordReset(t *testing.T) {
	h, db := site(t)
	box := maagotest.Mailbox(t)
	ann := newUser(t, db, "ann@example.com", nil)
	newUser(t, db, "bob@example.com", nil)
	old := login(t, h, "ann@example.com", "")
	bobs1 := login(t, h, "bob@example.com", "")
	if _, err := newToken(context.Background(), db, ann.ID, "verify", time.Hour); err != nil {
		t.Fatal(err)
	}
	other := resetLink(t, h, box, "ann@example.com")
	bobs := resetLink(t, h, box, "bob@example.com")
	token := resetLink(t, h, box, " ANN@example.com ")

	for range 2 { // opening the link uses nothing up
		if w := do(h, "GET", "/password/reset/confirm?token="+token, "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `value="`+token+`"`) || !strings.Contains(w.Body.String(), `name="password"`) {
			t.Fatalf("GET: %d %s", w.Code, w.Body)
		}
	}
	for _, weak := range []string{"short", "baseball", strings.Repeat("가", 1400)} { // the last: 4200 bytes
		w := do(h, "POST", "/password/reset/confirm", confirm(token, weak), "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), "This password is too") || !strings.Contains(w.Body.String(), `value="`+token+`"`) || strings.Contains(w.Body.String(), `value="`+weak) {
			t.Errorf("%.10q: %d %s", weak, w.Code, w.Body)
		}
	}
	const fresh = "a fresh secret phrase"
	w := do(h, "POST", "/password/reset/confirm", confirm(token, fresh), "")
	if w.Code != 303 || w.Header().Get("Location") != "/login" || w.Header().Get("Set-Cookie") != "" {
		t.Fatalf("confirm: %d %v %s", w.Code, w.Header(), w.Body)
	}
	if w := do(h, "POST", "/login", form("ann@example.com", password), ""); w.Code != 200 {
		t.Errorf("the old password still signs in: %d", w.Code)
	}
	if w := do(h, "POST", "/login", form("ann@example.com", fresh), ""); w.Code != 303 {
		t.Errorf("the new password does not sign in: %d %s", w.Code, w.Body)
	}
	if whoami(h, old.Value) != "" {
		t.Error("a session from before the reset is still signed in")
	}
	if w := do(h, "POST", "/login", form("bob@example.com", password), ""); w.Code != 303 || whoami(h, bobs1.Value) != "bob@example.com" {
		t.Errorf("ann's reset changed bob's password or session: %d", w.Code)
	}
	for name, tk := range map[string]string{"the same link": token, "ann's other link": other} {
		if w := do(h, "POST", "/password/reset/confirm", confirm(tk, "another secret phrase"), ""); w.Code != 200 || !strings.Contains(w.Body.String(), "This link is invalid or has expired.") {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	if w := do(h, "POST", "/password/reset/confirm", confirm(bobs, "a fresh secret phrase"), ""); w.Code != 303 {
		t.Errorf("bob's link: %d %s", w.Code, w.Body)
	}
	if count(t, db, "SELECT count(*) FROM auth_token WHERE purpose = 'verify'") != 1 {
		t.Error("the reset removed the verification token")
	}
}

// Of two requests with one link at once, one sets the password.
func TestPasswordResetOnce(t *testing.T) {
	h, db := site(t)
	ann := newUser(t, db, "ann@example.com", nil)
	token, err := newToken(context.Background(), db, ann.ID, "reset", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	codes := make(chan int, 2)
	for _, pw := range []string{"first secret phrase", "second secret phrase"} {
		go func() { codes <- do(h, "POST", "/password/reset/confirm", confirm(token, pw), "").Code }()
	}
	if a, b := <-codes, <-codes; a+b != 303+200 {
		t.Errorf("codes %d and %d, want one 303 and one 200", a, b)
	}
}

// A reset request answers the same for any email, and mails only an
// active account, verified or not.
func TestPasswordResetRequest(t *testing.T) {
	h, db := site(t)
	box := maagotest.Mailbox(t)
	newUser(t, db, "ann@example.com", nil)
	newUser(t, db, "new@example.com", func(in *NewUserInput) { in.VerifiedAt = nil })
	newUser(t, db, "gone@example.com", func(in *NewUserInput) { in.IsActive = maago.Set(false) })
	ask := func(email string) *httptest.ResponseRecorder {
		return do(h, "POST", "/password/reset", url.Values{"email": {email}}.Encode(), "")
	}
	want := ask("ann@example.com")
	for _, email := range []string{"nobody@example.com", "gone@example.com", "new@example.com"} {
		if w := ask(email); w.Code != want.Code || w.Body.String() != want.Body.String() {
			t.Errorf("%s answers differently:\n%s", email, w.Body)
		}
	}
	sent := box.Messages()
	if len(sent) != 2 || sent[0].To != "ann@example.com" || sent[1].To != "new@example.com" {
		t.Errorf("sent %v", sent)
	}
	if w := do(h, "GET", "/password/reset", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `name="email"`) {
		t.Errorf("form: %d", w.Code)
	}
	if !strings.Contains(do(h, "GET", "/login", "", "").Body.String(), `href="/password/reset"`) {
		t.Error("no link from the login page")
	}

	t.Cleanup(func() { SiteURL = "https://example.com" })
	SiteURL = "example.com"
	tokens := count(t, db, "SELECT count(*) FROM auth_token")
	if w := ask("ann@example.com"); w.Code != 500 || count(t, db, "SELECT count(*) FROM auth_token") != tokens {
		t.Errorf("bad SiteURL: %d", w.Code)
	}
}

// A link that is expired, of another purpose, or of an account closed
// since, sets nothing.
func TestPasswordResetRefuses(t *testing.T) {
	h, db := site(t)
	ctx := context.Background()
	ann := newUser(t, db, "ann@example.com", nil)
	add := func(purpose string, expires time.Time) string {
		token, hash := authlib.NewToken()
		if _, err := InsertToken(ctx, db, NewTokenInput{UserID: ann.ID, Purpose: purpose, Hash: hash, ExpiresAt: expires}); err != nil {
			t.Fatal(err)
		}
		return token
	}
	expired := add("reset", time.Now().Add(-time.Second))
	verify := add("verify", time.Now().Add(time.Hour))
	closed := add("reset", time.Now().Add(time.Hour))
	refused := func(name, token string) {
		t.Helper()
		w := do(h, "POST", "/password/reset/confirm", confirm(token, "a fresh secret phrase"), "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), "This link is invalid or has expired.") {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	for name, token := range map[string]string{"expired": expired, "verify": verify, "garbage": "x", "too long": strings.Repeat("x", 65), "none": ""} {
		refused(name, token)
	}
	ann.IsActive = false
	if err := UpdateUser(ctx, db, &ann); err != nil {
		t.Fatal(err)
	}
	refused("closed", closed)
	if got, _ := GetUser(ctx, db, ann.ID); got.PasswordHash != ann.PasswordHash {
		t.Error("the password changed")
	}
	// Open again, the account's link still works: the refusal used nothing up.
	ann.IsActive = true
	if err := UpdateUser(ctx, db, &ann); err != nil {
		t.Fatal(err)
	}
	if w := do(h, "POST", "/password/reset/confirm", confirm(closed, "a fresh secret phrase"), ""); w.Code != 303 {
		t.Errorf("the link after reopening: %d %s", w.Code, w.Body)
	}
	for _, path := range []string{"/password/reset/confirm", "/password/reset/confirm?token=x"} {
		if w := do(h, "GET", path, "", ""); w.Code != 200 {
			t.Errorf("GET %s: %d", path, w.Code)
		}
	}
}

func TestPasswordChange(t *testing.T) {
	h, db := site(t)
	ann := newUser(t, db, "ann@example.com", nil)
	if w := do(h, "GET", "/password", "", ""); w.Code != 303 || w.Header().Get("Location") != "/login?next=%2Fpassword" {
		t.Errorf("GET without a session: %d %v", w.Code, w.Header())
	}
	change := func(old, new string) string { return url.Values{"old_password": {old}, "new_password": {new}}.Encode() }
	const fresh = "a fresh secret phrase"
	if w := do(h, "POST", "/password", change(password, fresh), ""); w.Code != 401 {
		t.Errorf("POST without a session: %d", w.Code)
	}
	here := login(t, h, "ann@example.com", "")
	there := login(t, h, "ann@example.com", "")
	if w := do(h, "GET", "/password", "", here.Value); w.Code != 200 || !strings.Contains(w.Body.String(), `name="old_password"`) {
		t.Errorf("form: %d %s", w.Code, w.Body)
	}
	for range 11 { // the rules refuse it before an attempt counts
		if w := do(h, "POST", "/password", change(password, "short"), here.Value); w.Code != 200 {
			t.Fatalf("a weak new password: %d", w.Code)
		}
	}
	for _, c := range []struct{ old, new, want string }{
		{"wrong", fresh, "Your old password is incorrect."},
		{password, "short", "This password is too short."},
	} {
		w := do(h, "POST", "/password", change(c.old, c.new), here.Value)
		if w.Code != 200 || !strings.Contains(w.Body.String(), c.want) || w.Header().Get("Set-Cookie") != "" || strings.Contains(w.Body.String(), `value="`+c.old) || strings.Contains(w.Body.String(), `value="`+c.new) {
			t.Errorf("%s: %d %v %s", c.want, w.Code, w.Header(), w.Body)
		}
	}
	if got, _ := GetUser(context.Background(), db, ann.ID); got.PasswordHash != ann.PasswordHash || whoami(h, here.Value) == "" {
		t.Error("a refused change changed something")
	}

	w := do(h, "POST", "/password", change(password, fresh), here.Value)
	cookies := w.Result().Cookies()
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Your password was changed.") || len(cookies) != 1 || cookies[0].Value == here.Value {
		t.Fatalf("change: %d %v %s", w.Code, cookies, w.Body)
	}
	if whoami(h, cookies[0].Value) != "ann@example.com" || whoami(h, here.Value) != "" || whoami(h, there.Value) != "" {
		t.Error("the sessions after a change")
	}
	if w := do(h, "POST", "/login", form("ann@example.com", fresh), ""); w.Code != 303 {
		t.Errorf("the new password: %d", w.Code)
	}
	if w := do(h, "POST", "/login", form("ann@example.com", password), ""); w.Code != 200 {
		t.Errorf("the old password still signs in: %d", w.Code)
	}
}

// from posts a form from the IP ip.
func from(h http.Handler, ip, path, body string) *httptest.ResponseRecorder {
	return doFrom(h, ip, "POST", path, body, "")
}

// The eleventh attempt in the window is refused, for the email from any
// IP, whether it has an account or not; signing in counts apart from
// mails, and a sign-in does not count.
func TestLimit(t *testing.T) {
	h, db := site(t)
	box := maagotest.Mailbox(t)
	ann := newUser(t, db, "ann@example.com", nil)
	newUser(t, db, "carol@example.com", nil)
	tooMany := func(name string, w *httptest.ResponseRecorder) {
		t.Helper()
		if w.Code != 429 || w.Header().Get("Set-Cookie") != "" {
			t.Errorf("%s: %d %v", name, w.Code, w.Header())
		}
	}

	for i := range 10 {
		if w := from(h, fmt.Sprintf("198.51.100.%d", i), "/login", form("ann@example.com", "wrong")); w.Code != 200 {
			t.Fatalf("attempt %d: %d", i+1, w.Code)
		}
	}
	tooMany("the eleventh", from(h, "198.51.100.99", "/login", form(" ANN@example.com ", "wrong")))
	tooMany("the right password", from(h, "198.51.100.99", "/login", form("ann@example.com", password)))
	token, err := newToken(context.Background(), db, ann.ID, "verify", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	tooMany("verifying with the password", from(h, "198.51.100.99", "/verify", verify(token, password)))
	if n := count(t, db, "SELECT count(*) FROM auth_attempt WHERE email = 'ann@example.com'"); n != 13 {
		t.Errorf("%d attempts recorded, want 13 with the refused", n)
	}
	for range 15 { // a sign-in does not count
		if w := from(h, "198.51.100.1", "/login", form("carol@example.com", password)); w.Code != 303 {
			t.Fatalf("carol: %d", w.Code)
		}
	}

	// Mails: signing up, resetting and verifying again count together,
	// and the sign-ins above not.
	email := url.Values{"email": {"dave@example.com"}}.Encode()
	mails := []struct{ path, body string }{
		{"/signup", form("dave@example.com", "a long fresh password")},
		{"/password/reset", email},
		{"/verify/resend", email},
	}
	for i := range 10 {
		if m := mails[i%3]; from(h, "203.0.113.1", m.path, m.body).Code != 200 {
			t.Fatalf("mail %d %s", i+1, m.path)
		}
	}
	sent := len(box.Messages())
	for _, m := range mails {
		tooMany(m.path, from(h, "203.0.113.2", m.path, m.body))
	}
	if len(box.Messages()) != sent {
		t.Errorf("a refused attempt sent mail")
	}
	if w := from(h, "203.0.113.2", "/password/reset", url.Values{"email": {"ann@example.com"}}.Encode()); w.Code != 200 {
		t.Errorf("signing in refused, a reset is not: %d", w.Code)
	}

	// The window passes.
	t.Cleanup(func() { AttemptWindow = 15 * time.Minute })
	AttemptWindow = time.Nanosecond
	if w := from(h, "198.51.100.99", "/login", form("ann@example.com", password)); w.Code != 303 {
		t.Errorf("after the window: %d", w.Code)
	}
	if n := count(t, db, "SELECT count(*) FROM auth_attempt"); n != 0 {
		t.Errorf("%d old attempts are left", n)
	}
}

// One IP may make AttemptLimitIP attempts, for any emails.
func TestLimitIP(t *testing.T) {
	h, _ := site(t)
	for i := range AttemptLimitIP {
		if w := from(h, "2001:db8::1", "/login", form(fmt.Sprintf("u%d@example.com", i), "wrong")); w.Code != 200 {
			t.Fatalf("attempt %d: %d", i+1, w.Code)
		}
	}
	if w := from(h, "2001:db8::2", "/login", form("ann@example.com", "wrong")); w.Code != 429 { // the same /64
		t.Errorf("over the limit: %d", w.Code)
	}
	if w := from(h, "2001:db8:0:1::1", "/login", form("ann@example.com", "wrong")); w.Code != 200 {
		t.Errorf("another IP: %d", w.Code)
	}
}

// Of attempts at once, no more than the limit get through.
func TestLimitAtOnce(t *testing.T) {
	h, _ := site(t)
	codes := make(chan int, 20)
	for i := range 20 {
		go func() {
			codes <- from(h, fmt.Sprintf("198.51.100.%d", i), "/login", form("ann@example.com", "wrong")).Code
		}()
	}
	passed := 0
	for range 20 {
		if <-codes != 429 {
			passed++
		}
	}
	if passed > 10 { // fewer is right too: racing, it refuses more
		t.Errorf("%d of 20 got through", passed)
	}
}

// An account whose first mail failed is verified by sending it again; any
// other email gets the same answer and no mail.
func TestVerifyResend(t *testing.T) {
	h, db := site(t)
	old := authlib.Mail
	authlib.Mail = func(context.Context, authlib.Message) error { return errors.New("smtp down") }
	t.Cleanup(func() { authlib.Mail = old }) // after Mailbox's below: cleanups run last first
	if w := do(h, "POST", "/signup", form("ann@example.com", password), ""); w.Code != 500 {
		t.Fatalf("mail fails: %d", w.Code)
	}
	box := maagotest.Mailbox(t)
	token := resend(t, h, box, " Ann@Example.com ")
	if w := do(h, "POST", "/verify", verify(token, password), ""); w.Code != 303 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("verify: %d %s", w.Code, w.Body)
	}

	newUser(t, db, "bob@example.com", nil)
	newUser(t, db, "gone@example.com", func(in *NewUserInput) { in.VerifiedAt, in.IsActive = nil, maago.Set(false) })
	want := do(h, "POST", "/verify/resend", url.Values{"email": {"nobody@example.com"}}.Encode(), "")
	for _, email := range []string{"ann@example.com", "bob@example.com", "gone@example.com"} {
		if w := do(h, "POST", "/verify/resend", url.Values{"email": {email}}.Encode(), ""); w.Code != 200 || w.Body.String() != want.Body.String() {
			t.Errorf("%s answers differently: %d\n%s", email, w.Code, w.Body)
		}
	}
	if sent := box.Messages(); len(sent) != 1 {
		t.Errorf("sent %v", sent)
	}
	if w := do(h, "GET", "/verify/resend", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `name="email"`) {
		t.Errorf("form: %d", w.Code)
	}
	if !strings.Contains(do(h, "GET", "/login", "", "").Body.String(), `href="/verify/resend"`) {
		t.Error("no link from the login page")
	}
}

// Who signs up first with another's email, choosing the password, does
// not get the account: verifying asks for the password, and the owner
// resets it first.
func TestSignupTakenFirst(t *testing.T) {
	h, db := site(t)
	box := maagotest.Mailbox(t)
	const theirs = "mallory picked this"
	if w := do(h, "POST", "/signup", form("ann@example.com", theirs), ""); w.Code != 200 {
		t.Fatalf("their signup: %d", w.Code)
	}
	// The owner signs up too, and gets a link; the password stays theirs.
	do(h, "POST", "/signup", form("ann@example.com", password), "")
	sent := box.Messages()
	if len(sent) != 2 || sent[1].Subject != "Verify your email" {
		t.Fatalf("sent %v", sent)
	}
	token := strings.TrimPrefix(link(t, sent[1].Body, "https://example.com/verify?token="), "https://example.com/verify?token=")
	tokens := count(t, db, "SELECT count(*) FROM auth_token")
	w := do(h, "POST", "/verify", verify(token, password), "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Incorrect password.") || w.Header().Get("Set-Cookie") != "" || strings.Contains(w.Body.String(), password) {
		t.Fatalf("verify with the owner's password: %d %v %s", w.Code, w.Header(), w.Body)
	}
	if count(t, db, "SELECT count(*) FROM auth_token") != tokens || count(t, db, "SELECT count(*) FROM auth_user WHERE verified_at IS NULL") != 1 {
		t.Error("a refused verification used the token or verified")
	}

	reset := resetLink(t, h, box, "ann@example.com")
	if w := do(h, "POST", "/password/reset/confirm", confirm(reset, password), ""); w.Code != 303 {
		t.Fatalf("reset: %d %s", w.Code, w.Body)
	}
	if w := do(h, "POST", "/verify", verify(token, password), ""); w.Code != 303 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("verify after the reset: %d %s", w.Code, w.Body)
	}
	if w := do(h, "POST", "/login", form("ann@example.com", theirs), ""); w.Code != 200 {
		t.Errorf("their password signs in: %d", w.Code)
	}
}

func TestRequireStaff(t *testing.T) {
	ctx := context.Background()
	if _, err := RequireStaff(ctx); err == nil {
		t.Error("nobody passed")
	}
	if _, err := RequireStaff(maago.WithUser(ctx, User{ID: 1})); err == nil {
		t.Error("a user not staff passed")
	}
	if u, err := RequireStaff(maago.WithUser(ctx, User{ID: 2, IsStaff: true})); err != nil || u.ID != 2 {
		t.Errorf("staff: %v %v", u, err)
	}
}

func TestAccountClose(t *testing.T) {
	h, db := site(t)
	box := maagotest.Mailbox(t)
	ctx := context.Background()
	ann := newUser(t, db, "ann@example.com", nil)
	newUser(t, db, "bob@example.com", nil)
	bob := login(t, h, "bob@example.com", "")
	if w := do(h, "GET", "/account/close", "", ""); w.Code != 303 {
		t.Errorf("GET without a session: %d", w.Code)
	}
	closing := url.Values{"password": {password}}.Encode()
	if w := do(h, "POST", "/account/close", closing, ""); w.Code != 401 {
		t.Errorf("POST without a session: %d", w.Code)
	}
	here := login(t, h, "ann@example.com", "")
	there := login(t, h, "ann@example.com", "")
	reset := resetLink(t, h, box, "ann@example.com")
	if _, err := newToken(ctx, db, ann.ID, "verify", VerifyAge); err != nil {
		t.Fatal(err)
	}
	if w := do(h, "GET", "/account/close", "", here.Value); w.Code != 200 || !strings.Contains(w.Body.String(), `name="password"`) {
		t.Errorf("form: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(do(h, "GET", "/password", "", here.Value).Body.String(), `href="/account/close"`) {
		t.Error("no link from the password page")
	}

	w := do(h, "POST", "/account/close", url.Values{"password": {"wrong"}}.Encode(), here.Value)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Your password is incorrect.") || w.Header().Get("Set-Cookie") != "" {
		t.Errorf("wrong password: %d %v %s", w.Code, w.Header(), w.Body)
	}
	if u, _ := GetUser(ctx, db, ann.ID); !u.IsActive || whoami(h, there.Value) == "" {
		t.Error("a wrong password closed something")
	}

	w = do(h, "POST", "/account/close", closing, here.Value)
	gone := w.Result().Cookies()
	if w.Code != 303 || w.Header().Get("Location") != "/login" || len(gone) != 1 || gone[0].MaxAge != -1 {
		t.Fatalf("close: %d %v", w.Code, w.Header())
	}
	if u, err := GetUser(ctx, db, ann.ID); err != nil || u.IsActive {
		t.Errorf("after closing: %v %v", u, err)
	}
	if n := count(t, db, fmt.Sprintf("SELECT count(*) FROM auth_session WHERE user_id = %d", ann.ID)); n != 0 {
		t.Errorf("%d sessions left", n)
	}
	if n := count(t, db, fmt.Sprintf("SELECT count(*) FROM auth_token WHERE user_id = %d", ann.ID)); n != 0 {
		t.Errorf("%d tokens left", n)
	}
	if w := do(h, "POST", "/password/reset/confirm", confirm(reset, "a fresh secret phrase"), ""); !strings.Contains(w.Body.String(), "This link is invalid or has expired.") {
		t.Errorf("the reset link from before: %d", w.Code)
	}
	if whoami(h, bob.Value) != "bob@example.com" {
		t.Error("closing ann signed bob out")
	}

	// Its email answers as one with no account, and gets no mail.
	sent := len(box.Messages())
	for _, c := range []struct{ path, body string }{
		{"/login", form("ann@example.com", password)},
		{"/signup", form("ann@example.com", "a long fresh password")},
		{"/password/reset", url.Values{"email": {"ann@example.com"}}.Encode()},
		{"/verify/resend", url.Values{"email": {"ann@example.com"}}.Encode()},
	} {
		want := do(h, "POST", c.path, strings.ReplaceAll(c.body, "ann%40", "nobody%40"), "")
		if w := do(h, "POST", c.path, c.body, ""); w.Code != want.Code || w.Body.String() != strings.ReplaceAll(want.Body.String(), "nobody", "ann") { // the form draws the email back
			t.Errorf("%s: %d\n%s", c.path, w.Code, w.Body)
		}
	}
	for _, m := range box.Messages()[sent:] {
		if m.To == "ann@example.com" {
			t.Errorf("sent %s to the closed account", m.Subject)
		}
	}

	// Opened again by hand, the account has none of its old sessions.
	ann.IsActive = true
	if err := UpdateUser(ctx, db, &ann); err != nil {
		t.Fatal(err)
	}
	if whoami(h, here.Value) != "" || whoami(h, there.Value) != "" {
		t.Error("an old session came back")
	}
}

func TestCreateSuperuser(t *testing.T) {
	h, db := site(t)
	ctx := context.Background()
	u, err := CreateSuperuser(ctx, db, " Boss@Example.com ", password)
	if err != nil || u.Email != "boss@example.com" || !u.IsStaff || !u.IsSuperuser || u.VerifiedAt == nil {
		t.Fatalf("%+v %v", u, err)
	}
	if c := login(t, h, "boss@example.com", ""); whoami(h, c.Value) != "boss@example.com" {
		t.Error("cannot sign in")
	}
	for _, c := range []struct{ email, password, want string }{
		{"BOSS@example.com", "another long phrase", "has an account already"},
		{"not an email", password, "is not a valid email address"},
		{"new@example.com", "tiny7", "This password is too short."},
		{"newton@example.com", "newton rides a bike", "This password is too similar to your email."},
	} {
		_, err := CreateSuperuser(ctx, db, c.email, c.password)
		if err == nil || !strings.Contains(err.Error(), c.want) || strings.Contains(err.Error(), c.password) {
			t.Errorf("%s: %v", c.email, err)
		}
	}
	if n := count(t, db, "SELECT count(*) FROM auth_user"); n != 1 {
		t.Errorf("%d users", n)
	}
}

// bearer sends a request with the API token.
func bearer(h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// makeToken makes an API token of the session's user and returns it.
func makeToken(t *testing.T, h http.Handler, session, name string) string {
	t.Helper()
	w := do(h, "POST", "/tokens", url.Values{"name": {name}, "password": {password}}.Encode(), session)
	_, rest, ok := strings.Cut(w.Body.String(), "<code>")
	token, _, _ := strings.Cut(rest, "</code>")
	if w.Code != 200 || !ok || len(token) != 43 {
		t.Fatalf("make a token: %d %s", w.Code, w.Body)
	}
	return token
}

func TestAPITokens(t *testing.T) {
	h, db := site(t)
	newUser(t, db, "ann@example.com", nil)
	bob := newUser(t, db, "bob@example.com", nil)
	if w := do(h, "GET", "/tokens", "", ""); w.Code != 303 {
		t.Errorf("GET without a session: %d", w.Code)
	}
	s := login(t, h, "ann@example.com", "")
	if w := do(h, "POST", "/tokens", url.Values{"name": {"ci"}, "password": {"wrong"}}.Encode(), s.Value); w.Code != 200 || !strings.Contains(w.Body.String(), "Your password is incorrect.") || strings.Contains(w.Body.String(), "<code>") {
		t.Errorf("wrong password: %d %s", w.Code, w.Body)
	}
	if count(t, db, "SELECT count(*) FROM auth_api_token") != 0 {
		t.Error("a wrong password made a token")
	}

	token := makeToken(t, h, s.Value, "ci")
	var stored string
	if err := db.QueryRow("SELECT hash FROM auth_api_token").Scan(&stored); err != nil || stored != authlib.HashToken(token) {
		t.Errorf("stored %q %v, want the token's hash", stored, err)
	}
	list := do(h, "GET", "/tokens", "", s.Value).Body.String()
	if !strings.Contains(list, "ci, made ") || strings.Contains(list, token) || strings.Contains(list, stored) {
		t.Errorf("list:\n%s", list)
	}
	if got := bearer(h, "GET", "/whoami", "", token).Body.String(); got != "ann@example.com" {
		t.Errorf("the token is %q", got)
	}

	// A token cannot make another without the password.
	if w := bearer(h, "POST", "/tokens", url.Values{"name": {"more"}, "password": {"guess"}}.Encode(), token); w.Code != 200 || count(t, db, "SELECT count(*) FROM auth_api_token") != 1 {
		t.Errorf("a token made a token: %d", w.Code)
	}
	// Another's token is not there to delete.
	bobs := makeToken(t, h, login(t, h, "bob@example.com", "").Value, "bob's")
	var bobsID int64
	if err := db.QueryRow(fmt.Sprintf("SELECT id FROM auth_api_token WHERE user_id = %d", bob.ID)).Scan(&bobsID); err != nil {
		t.Fatal(err)
	}
	if w := do(h, "POST", fmt.Sprintf("/tokens/%d/delete", bobsID), "", s.Value); w.Code != 404 || bearer(h, "GET", "/whoami", "", bobs).Body.String() != "bob@example.com" {
		t.Errorf("deleting bob's: %d", w.Code)
	}
	var id int64
	if err := db.QueryRow(fmt.Sprintf("SELECT id FROM auth_api_token WHERE hash = '%s'", stored)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if w := do(h, "POST", fmt.Sprintf("/tokens/%d/delete", id), "", s.Value); w.Code != 303 || w.Header().Get("Location") != "/tokens" {
		t.Errorf("delete: %d %v", w.Code, w.Header())
	}
	if got := bearer(h, "GET", "/whoami", "", token).Body.String(); got != "" {
		t.Errorf("a deleted token is %q", got)
	}
}

// Changing the password keeps the API tokens; a reset and closing the
// account end them. A token of a user who may not sign in is no one's.
func TestAPITokensEnd(t *testing.T) {
	h, db := site(t)
	box := maagotest.Mailbox(t)
	ctx := context.Background()
	ann := newUser(t, db, "ann@example.com", nil)
	token := makeToken(t, h, login(t, h, "ann@example.com", "").Value, "ci")

	s := login(t, h, "ann@example.com", "")
	const fresh = "a fresh secret phrase"
	if w := do(h, "POST", "/password", url.Values{"old_password": {password}, "new_password": {fresh}}.Encode(), s.Value); w.Code != 200 || bearer(h, "GET", "/whoami", "", token).Body.String() != "ann@example.com" {
		t.Errorf("after a change: %d", w.Code)
	}
	reset := resetLink(t, h, box, "ann@example.com")
	if w := do(h, "POST", "/password/reset/confirm", confirm(reset, password), ""); w.Code != 303 || bearer(h, "GET", "/whoami", "", token).Body.String() != "" {
		t.Errorf("after a reset: %d", w.Code)
	}

	token = makeToken(t, h, login(t, h, "ann@example.com", "").Value, "ci")
	ann, _ = GetUser(ctx, db, ann.ID)
	ann.VerifiedAt = nil
	if err := UpdateUser(ctx, db, &ann); err != nil {
		t.Fatal(err)
	}
	if bearer(h, "GET", "/whoami", "", token).Body.String() != "" {
		t.Error("the token of a user not verified")
	}
	now := time.Now()
	ann.VerifiedAt, ann.IsActive = &now, false
	if err := UpdateUser(ctx, db, &ann); err != nil {
		t.Fatal(err)
	}
	if bearer(h, "GET", "/whoami", "", token).Body.String() != "" {
		t.Error("the token of a closed user")
	}
	ann.IsActive = true
	if err := UpdateUser(ctx, db, &ann); err != nil {
		t.Fatal(err)
	}
	if w := bearer(h, "POST", "/account/close", url.Values{"password": {password}}.Encode(), token); w.Code != 303 || count(t, db, "SELECT count(*) FROM auth_api_token") != 0 {
		t.Errorf("close: %d", w.Code)
	}
}

// A token gets no more password tries at the forms that ask it again than
// a sign-in does.
func TestAPITokenTriesPasswords(t *testing.T) {
	h, db := site(t)
	newUser(t, db, "ann@example.com", nil)
	token := makeToken(t, h, login(t, h, "ann@example.com", "").Value, "ci")
	for range 3 { // a success does not count
		makeToken(t, h, login(t, h, "ann@example.com", "").Value, "more")
	}
	tries := []struct{ path, body string }{
		{"/password", url.Values{"old_password": {"guess"}, "new_password": {"a fresh secret phrase"}}.Encode()},
		{"/account/close", url.Values{"password": {"guess"}}.Encode()},
		{"/tokens", url.Values{"name": {"x"}, "password": {"guess"}}.Encode()},
	}
	for i := range 10 {
		if w := bearer(h, "POST", tries[i%3].path, tries[i%3].body, token); w.Code != 200 {
			t.Fatalf("try %d %s: %d", i+1, tries[i%3].path, w.Code)
		}
	}
	for _, c := range tries {
		if w := bearer(h, "POST", c.path, c.body, token); w.Code != 429 {
			t.Errorf("%s: %d", c.path, w.Code)
		}
	}
}

// Can, the admin's: staff only, a superuser for all, other staff what
// their own rows allow, spelled as the admin spells it, and never to add,
// change or delete a User or a Permission (D-133).
func TestCan(t *testing.T) {
	_, db := site(t)
	ctx := context.Background()
	nobody := newUser(t, db, "nobody@example.com", func(in *NewUserInput) { in.IsSuperuser = maago.Set(true) })
	staff := newUser(t, db, "staff@example.com", func(in *NewUserInput) { in.IsStaff = maago.Set(true) })
	ed := newUser(t, db, "ed@example.com", func(in *NewUserInput) { in.IsStaff = maago.Set(true) })
	boss := newUser(t, db, "boss@example.com", func(in *NewUserInput) { in.IsStaff, in.IsSuperuser = maago.Set(true), maago.Set(true) })
	for _, p := range []NewPermissionInput{
		{UserID: ed.ID, App: "blog", ModelName: "Post", Action: "view"},
		{UserID: ed.ID, App: "blog", ModelName: "Tag", Action: "change"}, // the app right, the model not asked
		{UserID: ed.ID, App: "shop", ModelName: "Item", Action: "add"},   // the model below asked of another app
		{UserID: ed.ID, App: "auth", ModelName: "User", Action: "view"},
		{UserID: ed.ID, App: "auth", ModelName: "User", Action: "change"},
		{UserID: ed.ID, App: "auth", ModelName: "Permission", Action: "add"},
		{UserID: staff.ID, App: "blog", ModelName: "Post", Action: "change"}, // another user's row
		{UserID: nobody.ID, App: "blog", ModelName: "Post", Action: "view"},  // not staff
	} {
		if _, err := InsertPermission(ctx, db, p); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		user               *User
		app, model, action string
		want               bool
	}{
		{nil, "blog", "Post", "view", false},
		{&nobody, "blog", "Post", "view", false},
		{&staff, "blog", "Post", "view", false},
		{&ed, "blog", "Post", "view", true},
		{&ed, "blog", "Post", "change", false},
		{&ed, "blog", "post", "view", false},
		{&ed, "blog", "Tag", "view", false},
		{&ed, "blog", "Tag", "change", true},
		{&ed, "shop", "Tag", "change", false},
		{&ed, "blog", "Item", "add", false},
		{&ed, "auth", "User", "view", true},
		{&ed, "auth", "User", "change", false},
		{&ed, "auth", "Permission", "add", false},
		{&boss, "auth", "Permission", "add", true},
		{&boss, "any", "Thing", "delete", true},
	} {
		cctx := maago.WithDB(ctx, db)
		name := "nobody signed in"
		if c.user != nil {
			cctx, name = maago.WithUser(cctx, *c.user), c.user.Email
		}
		if got, err := Can(cctx, c.app, c.model, c.action); err != nil || got != c.want {
			t.Errorf("%s %s %s.%s: %t %v, want %t", name, c.action, c.app, c.model, got, err, c.want)
		}
	}

	closed := maagotest.DB(t)
	closed.Close()
	if _, err := Can(maago.WithUser(maago.WithDB(ctx, closed), ed), "blog", "Post", "view"); err == nil {
		t.Error("no error from a closed database")
	}
}

// appName is the package's name, which the admin calls the app.
func TestAppName(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "auth.go", nil, parser.PackageClauseOnly)
	if err != nil || f.Name.Name != appName {
		t.Errorf("appName is %q, but the package is %v (%v): rename appName with the package", appName, f.Name, err)
	}
}
