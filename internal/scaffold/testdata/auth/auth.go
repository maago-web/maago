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
// anyone send a link to another site.
var SiteURL string

// PasswordRules are what a new password must pass. Change them as you
// like; a rule is a func.
var PasswordRules = []authlib.PasswordRule{authlib.MinLength(8), authlib.NotCommon, authlib.NotSimilar}

// Current is the Config.Auth of the project: the user of the request's
// session, so that a view reads it with maago.UserFrom[auth.User].
var Current = authlib.FromSession(func(ctx context.Context, hash string) (User, error) {
	return SessionUser(ctx, maago.DBFrom(ctx), SessionUserParams{Hash: hash, Now: time.Now()})
})

// RequireStaff returns the signed-in user if staff, else a 403: a view
// for staff only starts with
//
//	u, err := auth.RequireStaff(ctx)
//
// on a route with maago.LoginRequired, which sends whoever is not signed
// in to the login page first.
func RequireStaff(ctx context.Context) (User, error) {
	u, ok := maago.UserFrom[User](ctx)
	if !ok || !u.IsStaff {
		return User{}, maago.Status(http.StatusForbidden, errors.New("staff only"))
	}
	return u, nil
}

// CreateSuperuser adds a staff account that may sign in at once, for
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
	return InsertUser(ctx, db, NewUserInput{Email: email, PasswordHash: hash, IsStaff: maago.Set(true), VerifiedAt: &now})
}

// cookiePath is where the session cookie goes: the whole site.
func cookiePath() string { return cmp.Or(maago.Prefix(), "/") }

// siteURL is SiteURL, once it is a scheme and a host and no more.
func siteURL() (string, error) {
	u, err := url.Parse(SiteURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Scheme+"://"+u.Host != strings.TrimSuffix(SiteURL, "/") {
		return "", fmt.Errorf("auth.SiteURL is %q; main sets it to the site's scheme and host, like https://example.com", SiteURL)
	}
	return u.Scheme + "://" + u.Host, nil
}
