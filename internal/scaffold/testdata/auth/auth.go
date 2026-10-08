package auth

import (
	"cmp"
	"context"
	"time"

	"github.com/maago-web/maago"
	authlib "github.com/maago-web/maago/contrib/auth"
)

// SessionAge is how long a sign-in lasts.
const SessionAge = 14 * 24 * time.Hour

// Current is the Config.Auth of the project: the user of the request's
// session, so that a view reads it with maago.UserFrom[auth.User].
var Current = authlib.FromSession(func(ctx context.Context, hash string) (User, error) {
	return SessionUser(ctx, maago.DBFrom(ctx), SessionUserParams{Hash: hash, Now: time.Now()})
})

// cookiePath is where the session cookie goes: the whole site.
func cookiePath() string { return cmp.Or(maago.Prefix(), "/") }
