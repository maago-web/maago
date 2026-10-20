package auth

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/maago-web/maago"
	authlib "github.com/maago-web/maago/contrib/auth"
)

// SessionAge is how long a sign-in lasts; VerifyAge and ResetAge how long
// the link of a verification or reset mail works.
const (
	SessionAge = 14 * 24 * time.Hour
	VerifyAge  = 3 * 24 * time.Hour
	ResetAge   = time.Hour
)

// AttemptLimit is how many attempts an email may have in AttemptWindow,
// and AttemptLimitIP how many one IP may make, larger because an office
// or a proxy is many people. Signing in and having a mail sent are
// counted apart. The next attempt is refused with 429.
var (
	AttemptLimit   int64 = 10
	AttemptLimitIP int64 = 100
	AttemptWindow        = 15 * time.Minute
)

// SiteURL is where the site is, for the links of mails: scheme and host,
// https://example.com. main sets it. The request's Host header would let
// anyone send a link to another site. Its scheme also decides the session
// cookie's Secure: http://localhost:8000 in development, where clients drop
// a Secure cookie, and https in production.
var SiteURL string

// PasswordRules are what a new password must pass. Change them as you
// like; a rule is a func.
var PasswordRules = []authlib.PasswordRule{authlib.MinLength(8), authlib.NotCommon, authlib.NotSimilar}

// Current is the Config.Auth of the project: the user of the request's
// API token or session, which a view takes as u User (signed in) or
// u *User (maybe not, nil then).
var Current = authlib.FromSession(
	func(ctx context.Context, hash string) (User, error) {
		return SessionUser(ctx, maago.DBFrom(ctx), SessionUserParams{Hash: hash, Now: time.Now()})
	},
	func(ctx context.Context, hash string) (User, error) {
		return APITokenUser(ctx, maago.DBFrom(ctx), APITokenUserParams{Hash: hash})
	},
)

// RequireStaff is a 403 unless u is staff: a view for staff only takes
// u User, which sends whoever is not signed in to the login page first,
// and starts with
//
//	if err := auth.RequireStaff(u); err != nil {
func RequireStaff(u User) error {
	if !u.IsStaff {
		return maago.Status(http.StatusForbidden, errors.New("staff only"))
	}
	return nil
}

// appName is this package's name, as the admin calls the app: rename it
// with the package, or Can no longer keeps User and Permission to
// superusers (TestAppName checks it).
const appName = "auth"

// Can is the admin's Config.Can: may the user of the request do
// action to the model of the app? Not a staff user: no, even a superuser.
// A superuser: yes. Any other staff user: what a Permission row allows,
// but never to add, change or delete a User or a Permission, which would
// give one more power: that is a superuser's job. A closed or unverified
// account is no user of the request at all (Current).
func Can(ctx context.Context, app, model, action string) (bool, error) {
	u, ok := maago.UserFrom[User](ctx)
	switch {
	case !ok || !u.IsStaff:
		return false, nil
	case u.IsSuperuser:
		return true, nil
	case app == appName && (model == "User" || model == "Permission") && action != "view":
		return false, nil
	}
	n, err := CountPermissions(ctx, maago.DBFrom(ctx), CountPermissionsParams{UserID: u.ID, App: app, ModelName: model, Action: action})
	return n.N > 0, err
}

// CreateSuperuser adds a superuser that may sign in at once, for
// main's -createsuperuser (see the package's doc). An email that has an
// account is an error.
func CreateSuperuser(ctx context.Context, db maago.DB, email, password string) (User, error) {
	email = normalizeEmail(email)
	if !validEmail(email) {
		return User{}, fmt.Errorf("auth: %q is not a valid email address", email)
	}
	if err := authlib.ValidatePassword(password, []string{email}, PasswordRules...); err != nil {
		return User{}, fmt.Errorf("auth: %w", err)
	}
	switch _, err := UserByEmail(ctx, db, UserByEmailParams{Email: email}); {
	case err == nil:
		return User{}, fmt.Errorf("auth: %s has an account already", email)
	case !errors.Is(err, sql.ErrNoRows):
		return User{}, err
	}
	hash, err := authlib.Hash(password)
	if err != nil {
		return User{}, err
	}
	now := time.Now()
	return InsertUser(ctx, db, NewUserInput{Email: email, PasswordHash: hash, IsStaff: maago.Set(true), IsSuperuser: maago.Set(true), VerifiedAt: &now})
}

// SignIn starts a session of u, as signing in does, and returns its
// cookie: for a test that asks pages as u, with maagotest.As(h, cookie).
// An account that may not sign in, closed or not verified, is an error:
// its session would be nobody's. A verified account for a test:
//
//	now := time.Now()
//	u, err := auth.InsertUser(ctx, db, auth.NewUserInput{Email: "ann@example.com", VerifiedAt: &now})
func SignIn(ctx context.Context, db maago.DB, u User) (*http.Cookie, error) {
	if !u.IsActive || u.VerifiedAt == nil {
		return nil, fmt.Errorf("auth: %s may not sign in: the account is closed or not verified", u.Email)
	}
	return newSession(ctx, db, u.ID)
}

// cookiePath is where the session cookie goes: the whole site.
func cookiePath() string { return cmp.Or(maago.Prefix(), "/") }

// cookieSecure is whether the session cookie is Secure: unless SiteURL is
// a right http one.
func cookieSecure() bool {
	site, err := siteURL()
	return err != nil || !strings.HasPrefix(site, "http://")
}

// siteURL is SiteURL, once it is a scheme and a host and no more.
func siteURL() (string, error) {
	u, err := url.Parse(SiteURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Scheme+"://"+u.Host != strings.TrimSuffix(SiteURL, "/") {
		return "", fmt.Errorf("auth.SiteURL is %q; main sets it to the site's scheme and host, like https://example.com", SiteURL)
	}
	return u.Scheme + "://" + u.Host, nil
}
