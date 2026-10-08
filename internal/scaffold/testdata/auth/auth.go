package auth

import (
	"cmp"
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/maago-web/maago"
	authlib "github.com/maago-web/maago/contrib/auth"
)

// SessionAge is how long a sign-in lasts; VerifyAge how long the link
// of a verification mail works.
const (
	SessionAge = 14 * 24 * time.Hour
	VerifyAge  = 3 * 24 * time.Hour
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
