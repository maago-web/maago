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
	u, err := UserByEmail(ctx, db, UserByEmailParams{Email: normalizeEmail(f.Email.Value)})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return maago.Result[LoginPage]{}, err
	}
	ok, rehash := authlib.Check(f.Password.Value, u.PasswordHash) // no account: an empty hash, the same work
	if !ok || !u.IsActive {
		f.AddError("Incorrect email or password.")
		return maago.Show(LoginPage{f, next}), nil
	}
	if u.VerifiedAt == nil { // only who knows the password learns this
		f.AddError("Verify your email first. Follow the link we sent you.")
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

// normalizeEmail is the email as it is stored and looked up.
func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

type SignupIn struct {
	Form SignupForm `maago:"form"`
}

type SignupPage struct {
	Form SignupForm
	Sent bool // the mail is sent: check your email
}

// Signup draws the form on GET and signs up on POST. Whether the email
// has an account or not, the answer is the same, and so is the work of
// the password hash: a mail tells the owner of the email what happened.
func Signup(ctx context.Context, in SignupIn) (maago.Result[SignupPage], error) {
	f := in.Form
	if !f.Valid() {
		return maago.Show(SignupPage{Form: f}), nil
	}
	site, err := siteURL()
	if err != nil {
		return maago.Result[SignupPage]{}, err
	}
	email := normalizeEmail(f.Email.Value)
	if err := authlib.ValidatePassword(f.Password.Value, []string{email}, PasswordRules...); err != nil {
		f.Password.Error = err.Error()
		return maago.Show(SignupPage{Form: f}), nil
	}
	hash, err := authlib.Hash(f.Password.Value)
	if err != nil {
		return maago.Result[SignupPage]{}, err
	}
	db := maago.DBFrom(ctx)
	switch _, err := UserByEmail(ctx, db, UserByEmailParams{Email: email}); {
	case err == nil:
		if err := authlib.Mail(ctx, accountExistsMail(email, site+LoginURL())); err != nil {
			return maago.Result[SignupPage]{}, err
		}
		return maago.Show(SignupPage{Sent: true}), nil
	case !errors.Is(err, sql.ErrNoRows):
		return maago.Result[SignupPage]{}, err
	}
	u, err := InsertUser(ctx, db, NewUserInput{Email: email, PasswordHash: hash})
	if err != nil {
		return maago.Result[SignupPage]{}, err
	}
	token, err := newToken(ctx, db, u.ID, "verify", VerifyAge)
	if err != nil {
		return maago.Result[SignupPage]{}, err
	}
	// If the mail fails, the account stays unverified: sending the mail
	// again verifies it.
	if err := authlib.Mail(ctx, verifyMail(email, site+VerifyURL()+"?token="+token)); err != nil {
		return maago.Result[SignupPage]{}, err
	}
	return maago.Show(SignupPage{Sent: true}), nil
}

// newToken stores a new one-time token of the user for purpose, and
// returns it for the mail.
func newToken(ctx context.Context, db maago.DB, userID int64, purpose string, age time.Duration) (string, error) {
	now := time.Now()
	if _, err := DeleteExpiredTokens(ctx, db, DeleteExpiredTokensParams{Now: now}); err != nil {
		return "", err
	}
	token, hash := authlib.NewToken()
	_, err := InsertToken(ctx, db, NewTokenInput{UserID: userID, Purpose: purpose, Hash: hash, ExpiresAt: now.Add(age)})
	return token, err
}

type VerifyIn struct {
	Token *string    `maago:"query=token"` // of the link
	Form  VerifyForm `maago:"form"`
}

type VerifyPage struct {
	Form  VerifyForm
	Token string
}

// Verify draws a button on GET and verifies on POST: a mail scanner that
// opens the link does not use the token up.
func Verify(ctx context.Context, in VerifyIn) (maago.Result[VerifyPage], error) {
	f := in.Form
	if !f.Bound() {
		token := ""
		if in.Token != nil {
			token = *in.Token
		}
		return maago.Show(VerifyPage{Token: token}), nil
	}
	if !f.Valid() {
		f.AddError("This link is invalid or has expired.")
		return maago.Show(VerifyPage{Form: f}), nil
	}
	tx, err := maago.DBFrom(ctx).BeginTx(ctx, nil)
	if err != nil {
		return maago.Result[VerifyPage]{}, err
	}
	defer tx.Rollback()
	now := time.Now()
	t, err := UseToken(ctx, tx, UseTokenParams{Hash: authlib.HashToken(f.Token.Value), Purpose: "verify", Now: now})
	if errors.Is(err, sql.ErrNoRows) {
		f.AddError("This link is invalid or has expired.")
		return maago.Show(VerifyPage{Form: f}), nil
	}
	if err != nil {
		return maago.Result[VerifyPage]{}, err
	}
	if _, err := VerifyUser(ctx, tx, VerifyUserParams{ID: t.UserID, Now: now}); err != nil {
		return maago.Result[VerifyPage]{}, err
	}
	if err := tx.Commit(); err != nil {
		return maago.Result[VerifyPage]{}, err
	}
	return maago.SeeOther[VerifyPage](LoginURL()), nil
}
