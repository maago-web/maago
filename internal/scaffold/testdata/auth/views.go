package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/maago-web/maago"
	authlib "github.com/maago-web/maago/contrib/auth"
)

type LoginIn struct {
	Next *string   `maago:"query=next"` // where to go once signed in
	Form LoginForm `maago:"form"`
}

type LoginPage struct {
	Form LoginForm
	Next string
}

// Login draws the form on GET and signs in on POST. Whatever is wrong,
// the answer is the same, and takes as long: it does not tell whether
// the email has an account.
func Login(ctx context.Context, in LoginIn) (maago.Result[LoginPage], error) {
	f, next := in.Form, ""
	if in.Next != nil {
		next = *in.Next
	}
	if !f.Valid() {
		return maago.Show(LoginPage{f, next}), nil
	}
	db := maago.DBFrom(ctx)
	u, err := UserByEmail(ctx, db, UserByEmailParams{Email: strings.ToLower(strings.TrimSpace(f.Email.Value))})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return maago.Result[LoginPage]{}, err
	}
	ok, rehash := authlib.Check(f.Password.Value, u.PasswordHash) // no account: an empty hash, the same work
	if !ok || !u.IsActive || u.VerifiedAt == nil {
		f.AddError("Incorrect email or password.")
		return maago.Show(LoginPage{f, next}), nil
	}
	if rehash {
		if u.PasswordHash, err = authlib.Hash(f.Password.Value); err != nil {
			return maago.Result[LoginPage]{}, err
		}
		if err := UpdateUser(ctx, db, &u); err != nil {
			return maago.Result[LoginPage]{}, err
		}
	}
	now := time.Now()
	if _, err := DeleteExpiredSessions(ctx, db, DeleteExpiredSessionsParams{Now: now}); err != nil {
		return maago.Result[LoginPage]{}, err
	}
	// A new token, never the one the request brought: a cookie planted
	// before sign-in does not become a session.
	token, hash := authlib.NewToken()
	if _, err := InsertSession(ctx, db, NewSessionInput{UserID: u.ID, Hash: hash, ExpiresAt: now.Add(SessionAge)}); err != nil {
		return maago.Result[LoginPage]{}, err
	}
	maago.SetCookie(ctx, authlib.SessionCookie(token, cookiePath(), SessionAge))
	return maago.SeeOther[LoginPage](authlib.SafeNext(next, maago.Prefix()+"/")), nil
}

// Logout signs the user out on every device: a view does not read the
// cookie, so it ends all of the user's sessions.
func Logout(ctx context.Context, in struct{}) (maago.Result[LoginPage], error) {
	if u, ok := maago.UserFrom[User](ctx); ok {
		if _, err := DeleteUserSessions(ctx, maago.DBFrom(ctx), DeleteUserSessionsParams{UserID: u.ID}); err != nil {
			return maago.Result[LoginPage]{}, err
		}
	}
	maago.SetCookie(ctx, authlib.SessionCookie("", cookiePath(), -1))
	return maago.SeeOther[LoginPage](LoginURL()), nil
}
