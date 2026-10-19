package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
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

// errRefused rolls back what a view began in maago.WithTx, which it then
// shows again: a form error, with nothing used up.
var errRefused = errors.New("refused")

// nextOf is where to go once signed in: the link's next, or "".
func nextOf(in LoginIn) string {
	if in.Next != nil {
		return *in.Next
	}
	return ""
}

// Login draws the form.
func Login(ctx context.Context, in LoginIn) (LoginPage, error) {
	return LoginPage{in.Form, nextOf(in)}, nil
}

// LoginSubmit signs in. Whatever is wrong, the answer is the same, and
// takes as long: it does not tell whether the email has an account. It
// writes without the request's transaction (maago.NoTx): the attempt it
// records must count for other requests at once.
func LoginSubmit(ctx context.Context, db maago.DB, in LoginIn) (maago.Result[LoginPage], error) {
	f, next := in.Form, nextOf(in)
	if !f.Valid() {
		return maago.Show(LoginPage{f, next}), nil
	}
	email := normalizeEmail(f.Email.Value)
	attempt, err := limit(ctx, db, "login", email)
	if err != nil {
		return maago.Result[LoginPage]{}, err
	}
	u, err := UserByEmail(ctx, db, UserByEmailParams{Email: email})
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
		hash, err := authlib.Hash(f.Password.Value)
		if err != nil {
			return maago.Result[LoginPage]{}, err
		}
		// The hash alone: writing the row read before hashing would undo
		// what changed meanwhile, such as a closed account.
		if _, err := SetPassword(ctx, db, SetPasswordParams{ID: u.ID, Hash: hash}); err != nil {
			return maago.Result[LoginPage]{}, err
		}
	}
	if _, err := DeleteExpiredSessions(ctx, db, DeleteExpiredSessionsParams{Now: time.Now()}); err != nil {
		return maago.Result[LoginPage]{}, err
	}
	token, err := newSession(ctx, db, u.ID)
	if err != nil {
		return maago.Result[LoginPage]{}, err
	}
	if err := DeleteAttempt(ctx, db, attempt); err != nil { // a sign-in is no attempt to count
		return maago.Result[LoginPage]{}, err
	}
	maago.SetCookie(ctx, authlib.SessionCookie(token, cookiePath(), SessionAge, cookieSecure()))
	return maago.SeeOther[LoginPage](authlib.SafeNext(next, maago.Prefix()+"/")), nil
}

// limit records an attempt at action, "login" or "mail", for the email
// from the request's IP, and refuses it with 429 when the email or the
// IP has had too many in AttemptWindow. It records first and
// counts after: of requests at once, each counts every row recorded
// before, so no more than the limit get through. A refused attempt stays
// recorded. It returns the attempt's ID, for a sign-in to take back.
func limit(ctx context.Context, db maago.DB, action, email string) (int64, error) {
	if _, in := db.(maago.Tx); in { // its rows would wait for the commit, unseen
		panic("auth: limit in a transaction: the attempt must count for other requests at once; take db maago.DB on a route with maago.NoTx")
	}
	since := time.Now().Add(-AttemptWindow)
	if _, err := DeleteOldAttempts(ctx, db, DeleteOldAttemptsParams{Since: since}); err != nil {
		return 0, err
	}
	ip := authlib.ClientIP(maago.RemoteAddrFrom(ctx))
	a, err := InsertAttempt(ctx, db, NewAttemptInput{Action: action, Email: email, IP: ip})
	if err != nil {
		return 0, err
	}
	n, err := CountAttempts(ctx, db, CountAttemptsParams{Action: action, Email: email, IP: ip, Since: since})
	if err != nil {
		return 0, err
	}
	if n.ByEmail > AttemptLimit || n.ByIP > AttemptLimitIP {
		return 0, maago.Status(http.StatusTooManyRequests, errors.New("too many attempts; wait a while and try again"))
	}
	return a.ID, nil
}

// newSession stores a new session of the user and returns its token for
// the cookie: a new token, never the one the request brought, so that a
// cookie planted before sign-in does not become a session.
func newSession(ctx context.Context, db maago.DB, userID int64) (string, error) {
	token, hash := authlib.NewToken()
	_, err := InsertSession(ctx, db, NewSessionInput{UserID: userID, Hash: hash, ExpiresAt: time.Now().Add(SessionAge)})
	return token, err
}

// Logout signs the user out on every device: a view does not read the
// cookie, so it ends all of the user's sessions.
func Logout(ctx context.Context, u *User, tx maago.Tx, in struct{}) (maago.Result[LoginPage], error) {
	if u != nil {
		if _, err := DeleteUserSessions(ctx, tx, DeleteUserSessionsParams{UserID: u.ID}); err != nil {
			return maago.Result[LoginPage]{}, err
		}
	}
	maago.SetCookie(ctx, authlib.SessionCookie("", cookiePath(), -1, cookieSecure()))
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

// Signup draws the form.
func Signup(ctx context.Context, in SignupIn) (SignupPage, error) {
	return SignupPage{Form: in.Form}, nil
}

// SignupSubmit signs up. Whether the email has an account or not, the
// answer is the same, and so is the work of the password hash: a mail
// tells the owner of the email what happened. Without the request's
// transaction (maago.NoTx): its attempt counts at once, and the hash does
// not hold the database.
func SignupSubmit(ctx context.Context, db maago.DB, in SignupIn) (maago.Result[SignupPage], error) {
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
	if _, err := limit(ctx, db, "mail", email); err != nil {
		return maago.Result[SignupPage]{}, err
	}
	hash, err := authlib.Hash(f.Password.Value)
	if err != nil {
		return maago.Result[SignupPage]{}, err
	}
	switch u, err := UserByEmail(ctx, db, UserByEmailParams{Email: email}); {
	case err == nil && !u.IsActive: // closed: nothing to send
		return maago.Show(SignupPage{Sent: true}), nil
	case err == nil && u.VerifiedAt == nil:
		// Not verified: the mail again, with the password unchanged. If
		// someone else signed up with this email, the owner resets the
		// password first, as the mail says, since verifying asks for it.
		if err := sendVerify(ctx, db, site, u); err != nil {
			return maago.Result[SignupPage]{}, err
		}
		return maago.Show(SignupPage{Sent: true}), nil
	case err == nil:
		if err := authlib.Mail(ctx, accountExistsMail(email, site+LoginURL())); err != nil {
			return maago.Result[SignupPage]{}, err
		}
		return maago.Show(SignupPage{Sent: true}), nil
	case !errors.Is(err, sql.ErrNoRows):
		return maago.Result[SignupPage]{}, err
	}
	u, err := InsertUser(ctx, db, NewUserInput{Email: email, PasswordHash: hash})
	if errors.As(err, new(*maago.UniqueError)) {
		// Signed up meanwhile, by another request: the same as the account
		// read above, which got its mail. Not the form's Fail: that would
		// tell that the account exists.
		return maago.Show(SignupPage{Sent: true}), nil
	}
	if err != nil {
		return maago.Result[SignupPage]{}, err
	}
	// If the mail fails, the account stays unverified: VerifyResend sends
	// it again.
	if err := sendVerify(ctx, db, site, u); err != nil {
		return maago.Result[SignupPage]{}, err
	}
	return maago.Show(SignupPage{Sent: true}), nil
}

// sendVerify mails the user a new link to verify the email.
func sendVerify(ctx context.Context, db maago.DB, site string, u User) error {
	token, err := newToken(ctx, db, u.ID, "verify", VerifyAge)
	if err != nil {
		return err
	}
	return authlib.Mail(ctx, verifyMail(u.Email, site+VerifyURL()+"?token="+token, site+PasswordResetURL()))
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

// Verify draws the form of the link: a mail scanner that opens the link
// does not use the token up.
func Verify(ctx context.Context, in VerifyIn) (VerifyPage, error) {
	token := ""
	if in.Token != nil {
		token = *in.Token
	}
	return VerifyPage{Form: in.Form, Token: token}, nil
}

// VerifySubmit verifies and signs in, given the password: who signed up
// first with another's email, choosing the password, does not get the
// account once its owner follows the link. Without the request's
// transaction (maago.NoTx), like LoginSubmit; the writes that go together
// run in one of their own.
func VerifySubmit(ctx context.Context, db maago.DB, in VerifyIn) (maago.Result[VerifyPage], error) {
	f := in.Form
	page := VerifyPage{Form: f, Token: f.Token.Value}
	if !f.Valid() {
		if f.Token.Error != "" {
			page.Form.AddError("This link is invalid or has expired.")
		}
		return maago.Show(page), nil
	}
	hash := authlib.HashToken(f.Token.Value)
	u, err := TokenUser(ctx, db, TokenUserParams{Hash: hash, Purpose: "verify", Now: time.Now()})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return maago.Result[VerifyPage]{}, err
	}
	if !u.IsActive { // no user is no active one
		page.Form.AddError("This link is invalid or has expired.")
		return maago.Show(page), nil
	}
	attempt, err := limit(ctx, db, "login", u.Email)
	if err != nil {
		return maago.Result[VerifyPage]{}, err
	}
	// The password out of the transaction: the database is not held
	// while it works.
	if ok, _ := authlib.Check(f.Password.Value, u.PasswordHash); !ok {
		page.Form.Password.Error = "Incorrect password."
		return maago.Show(page), nil
	}
	var token string
	// db is the transaction in here, so the outer one cannot be used.
	err = maago.WithTx(ctx, db, func(ctx context.Context, db maago.Tx) error {
		now := time.Now()
		if _, err := UseToken(ctx, db, UseTokenParams{Hash: hash, Purpose: "verify", Now: now}); errors.Is(err, sql.ErrNoRows) {
			page.Form.AddError("This link is invalid or has expired.")
			return errRefused
		} else if err != nil {
			return err
		}
		// Only if the password is the one checked: a reset since then
		// makes the link ask again, and the token stays.
		n, err := VerifyUser(ctx, db, VerifyUserParams{ID: u.ID, PasswordHash: u.PasswordHash, Now: now})
		if err != nil {
			return err
		}
		if n == 0 {
			page.Form.Password.Error = "Incorrect password."
			return errRefused
		}
		if _, err := DeleteUserTokens(ctx, db, DeleteUserTokensParams{UserID: u.ID, Purpose: "verify"}); err != nil {
			return err
		}
		if token, err = newSession(ctx, db, u.ID); err != nil {
			return err
		}
		return DeleteAttempt(ctx, db, attempt)
	})
	if errors.Is(err, errRefused) {
		return maago.Show(page), nil
	}
	if err != nil {
		return maago.Result[VerifyPage]{}, err
	}
	maago.SetCookie(ctx, authlib.SessionCookie(token, cookiePath(), SessionAge, cookieSecure()))
	return maago.SeeOther[VerifyPage](maago.Prefix() + "/"), nil
}

type VerifyResendIn struct {
	Form VerifyResendForm `maago:"form"`
}

type VerifyResendPage struct {
	Form VerifyResendForm
	Sent bool
}

// VerifyResend draws the form.
func VerifyResend(ctx context.Context, in VerifyResendIn) (VerifyResendPage, error) {
	return VerifyResendPage{Form: in.Form}, nil
}

// VerifyResendSubmit mails a new link to verify an active account that is
// not verified yet: one whose first mail failed or got lost. The answer is
// the same whatever the email. Without the request's transaction
// (maago.NoTx): its attempt counts at once.
func VerifyResendSubmit(ctx context.Context, db maago.DB, in VerifyResendIn) (maago.Result[VerifyResendPage], error) {
	f := in.Form
	if !f.Valid() {
		return maago.Show(VerifyResendPage{Form: f}), nil
	}
	site, err := siteURL()
	if err != nil {
		return maago.Result[VerifyResendPage]{}, err
	}
	email := normalizeEmail(f.Email.Value)
	if _, err := limit(ctx, db, "mail", email); err != nil {
		return maago.Result[VerifyResendPage]{}, err
	}
	u, err := UserByEmail(ctx, db, UserByEmailParams{Email: email})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return maago.Result[VerifyResendPage]{}, err
	}
	if u.IsActive && u.VerifiedAt == nil { // no account is no active one
		if err := sendVerify(ctx, db, site, u); err != nil {
			return maago.Result[VerifyResendPage]{}, err
		}
	}
	return maago.Show(VerifyResendPage{Sent: true}), nil
}

type PasswordChangeIn struct {
	Form PasswordChangeForm `maago:"form"`
}

type PasswordChangePage struct {
	Form    PasswordChangeForm
	Changed bool
}

// PasswordChange draws the form for the signed-in user.
func PasswordChange(ctx context.Context, u User, in PasswordChangeIn) (PasswordChangePage, error) {
	return PasswordChangePage{Form: in.Form}, nil
}

// PasswordChangeSubmit changes the password of the signed-in user. It
// ends every session of the user and starts a new one for this browser:
// the view does not read the cookie, so it cannot keep the current
// session. Without the request's transaction (maago.NoTx), like
// LoginSubmit, since it asks the password again.
func PasswordChangeSubmit(ctx context.Context, u User, db maago.DB, in PasswordChangeIn) (maago.Result[PasswordChangePage], error) {
	f := in.Form
	if !f.Valid() {
		return maago.Show(PasswordChangePage{Form: f}), nil
	}
	// The rules first: only a wrong old password is an attempt that counts.
	if err := authlib.ValidatePassword(f.NewPassword.Value, []string{u.Email}, PasswordRules...); err != nil {
		f.NewPassword.Error = err.Error()
		return maago.Show(PasswordChangePage{Form: f}), nil
	}
	attempt, ok, err := recheck(ctx, db, u, f.OldPassword.Value)
	if err != nil {
		return maago.Result[PasswordChangePage]{}, err
	}
	if !ok {
		f.OldPassword.Error = "Your old password is incorrect."
		return maago.Show(PasswordChangePage{Form: f}), nil
	}
	hash, err := authlib.Hash(f.NewPassword.Value)
	if err != nil {
		return maago.Result[PasswordChangePage]{}, err
	}
	var token string
	err = maago.WithTx(ctx, db, func(ctx context.Context, db maago.Tx) error {
		if _, err := SetPassword(ctx, db, SetPasswordParams{ID: u.ID, Hash: hash}); err != nil {
			return err
		}
		if _, err := DeleteUserSessions(ctx, db, DeleteUserSessionsParams{UserID: u.ID}); err != nil {
			return err
		}
		var err error
		if token, err = newSession(ctx, db, u.ID); err != nil {
			return err
		}
		return DeleteAttempt(ctx, db, attempt)
	})
	if err != nil {
		return maago.Result[PasswordChangePage]{}, err
	}
	maago.SetCookie(ctx, authlib.SessionCookie(token, cookiePath(), SessionAge, cookieSecure()))
	return maago.Show(PasswordChangePage{Changed: true}), nil
}

type PasswordResetIn struct {
	Form PasswordResetForm `maago:"form"`
}

type PasswordResetPage struct {
	Form PasswordResetForm
	Sent bool
}

// PasswordReset draws the form.
func PasswordReset(ctx context.Context, in PasswordResetIn) (PasswordResetPage, error) {
	return PasswordResetPage{Form: in.Form}, nil
}

// PasswordResetSubmit mails a link to reset the password to an active
// account. The answer is the same whether the email has one or not.
// Without the request's transaction (maago.NoTx): its attempt counts at
// once.
func PasswordResetSubmit(ctx context.Context, db maago.DB, in PasswordResetIn) (maago.Result[PasswordResetPage], error) {
	f := in.Form
	if !f.Valid() {
		return maago.Show(PasswordResetPage{Form: f}), nil
	}
	site, err := siteURL()
	if err != nil {
		return maago.Result[PasswordResetPage]{}, err
	}
	email := normalizeEmail(f.Email.Value)
	if _, err := limit(ctx, db, "mail", email); err != nil {
		return maago.Result[PasswordResetPage]{}, err
	}
	u, err := UserByEmail(ctx, db, UserByEmailParams{Email: email})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return maago.Result[PasswordResetPage]{}, err
	}
	if !u.IsActive { // no account is no active one
		return maago.Show(PasswordResetPage{Sent: true}), nil // nothing to send
	}
	token, err := newToken(ctx, db, u.ID, "reset", ResetAge)
	if err != nil {
		return maago.Result[PasswordResetPage]{}, err
	}
	if err := authlib.Mail(ctx, resetMail(u.Email, site+PasswordResetConfirmURL()+"?token="+token)); err != nil {
		return maago.Result[PasswordResetPage]{}, err
	}
	return maago.Show(PasswordResetPage{Sent: true}), nil
}

type PasswordResetConfirmIn struct {
	Token *string                  `maago:"query=token"` // of the link
	Form  PasswordResetConfirmForm `maago:"form"`
}

type PasswordResetConfirmPage struct {
	Form  PasswordResetConfirmForm
	Token string
}

// PasswordResetConfirm draws the form of the link.
func PasswordResetConfirm(ctx context.Context, in PasswordResetConfirmIn) (PasswordResetConfirmPage, error) {
	token := ""
	if in.Token != nil {
		token = *in.Token
	}
	return PasswordResetConfirmPage{Form: in.Form, Token: token}, nil
}

// PasswordResetConfirmSubmit sets the new password. It ends every session
// of the user and leaves them to sign in. Without the request's
// transaction (maago.NoTx): the hash does not hold the database; the
// writes that go together run in one of their own.
func PasswordResetConfirmSubmit(ctx context.Context, db maago.DB, in PasswordResetConfirmIn) (maago.Result[PasswordResetConfirmPage], error) {
	f := in.Form
	page := PasswordResetConfirmPage{Form: f, Token: f.Token.Value}
	if !f.Valid() {
		if f.Token.Error != "" {
			page.Form.AddError("This link is invalid or has expired.")
		}
		return maago.Show(page), nil
	}
	// The link first, reading only: no hash for whoever has none. Then the
	// hash, out of the transaction: the database is not held while it works.
	switch u, err := TokenUser(ctx, db, TokenUserParams{Hash: authlib.HashToken(f.Token.Value), Purpose: "reset", Now: time.Now()}); {
	case errors.Is(err, sql.ErrNoRows) || err == nil && !u.IsActive:
		page.Form.AddError("This link is invalid or has expired.")
		return maago.Show(page), nil
	case err != nil:
		return maago.Result[PasswordResetConfirmPage]{}, err
	}
	hash, err := authlib.Hash(f.Password.Value)
	if errors.Is(err, authlib.ErrTooLong) {
		page.Form.Password.Error = authlib.ValidatePassword(f.Password.Value, nil).Error()
		return maago.Show(page), nil
	}
	if err != nil {
		return maago.Result[PasswordResetConfirmPage]{}, err
	}
	// A refusal rolls back: the token stays for another try.
	err = maago.WithTx(ctx, db, func(ctx context.Context, db maago.Tx) error {
		t, err := UseToken(ctx, db, UseTokenParams{Hash: authlib.HashToken(f.Token.Value), Purpose: "reset", Now: time.Now()})
		if errors.Is(err, sql.ErrNoRows) {
			page.Form.AddError("This link is invalid or has expired.")
			return errRefused
		}
		if err != nil {
			return err
		}
		u, err := GetUser(ctx, db, t.UserID)
		if err != nil {
			return err
		}
		if !u.IsActive {
			page.Form.AddError("This link is invalid or has expired.")
			return errRefused
		}
		if err := authlib.ValidatePassword(f.Password.Value, []string{u.Email}, PasswordRules...); err != nil {
			page.Form.Password.Error = err.Error()
			return errRefused
		}
		if _, err := SetPassword(ctx, db, SetPasswordParams{ID: u.ID, Hash: hash}); err != nil {
			return err
		}
		if _, err := DeleteUserSessions(ctx, db, DeleteUserSessionsParams{UserID: u.ID}); err != nil {
			return err
		}
		if _, err := DeleteUserTokens(ctx, db, DeleteUserTokensParams{UserID: u.ID, Purpose: "reset"}); err != nil {
			return err
		}
		// Who took the account may have made tokens: getting it back ends them.
		_, err = DeleteUserAPITokens(ctx, db, DeleteUserAPITokensParams{UserID: u.ID})
		return err
	})
	if errors.Is(err, errRefused) {
		return maago.Show(page), nil
	}
	if err != nil {
		return maago.Result[PasswordResetConfirmPage]{}, err
	}
	return maago.SeeOther[PasswordResetConfirmPage](LoginURL()), nil
}

type AccountCloseIn struct {
	Form AccountCloseForm `maago:"form"`
}

type AccountClosePage struct {
	Form AccountCloseForm
}

// AccountClose draws the form for the signed-in user.
func AccountClose(ctx context.Context, u User, in AccountCloseIn) (AccountClosePage, error) {
	return AccountClosePage{in.Form}, nil
}

// AccountCloseSubmit closes the account of the signed-in user, given the
// password again: it may not sign in, and its sessions and tokens end.
// The row stays, as do the rows that point to it. Without the request's
// transaction (maago.NoTx), like LoginSubmit, since it asks the password.
func AccountCloseSubmit(ctx context.Context, u User, db maago.DB, in AccountCloseIn) (maago.Result[AccountClosePage], error) {
	f := in.Form
	if !f.Valid() {
		return maago.Show(AccountClosePage{f}), nil
	}
	attempt, ok, err := recheck(ctx, db, u, f.Password.Value)
	if err != nil {
		return maago.Result[AccountClosePage]{}, err
	}
	if !ok {
		f.Password.Error = "Your password is incorrect."
		return maago.Show(AccountClosePage{f}), nil
	}
	err = maago.WithTx(ctx, db, func(ctx context.Context, db maago.Tx) error {
		if _, err := CloseUser(ctx, db, CloseUserParams{ID: u.ID}); err != nil {
			return err
		}
		// The sessions go too: opened again, the account does not get them back.
		if _, err := DeleteUserSessions(ctx, db, DeleteUserSessionsParams{UserID: u.ID}); err != nil {
			return err
		}
		for _, purpose := range []string{"verify", "reset"} {
			if _, err := DeleteUserTokens(ctx, db, DeleteUserTokensParams{UserID: u.ID, Purpose: purpose}); err != nil {
				return err
			}
		}
		if _, err := DeleteUserAPITokens(ctx, db, DeleteUserAPITokensParams{UserID: u.ID}); err != nil {
			return err
		}
		return DeleteAttempt(ctx, db, attempt)
	})
	if err != nil {
		return maago.Result[AccountClosePage]{}, err
	}
	maago.SetCookie(ctx, authlib.SessionCookie("", cookiePath(), -1, cookieSecure()))
	return maago.SeeOther[AccountClosePage](LoginURL()), nil
}

// recheck asks the signed-in user's password again, as an attempt to
// sign in: an API token reaches these forms too, and must not try
// passwords without limit. It returns the attempt's ID, for a success to
// take back.
func recheck(ctx context.Context, db maago.DB, u User, password string) (int64, bool, error) {
	attempt, err := limit(ctx, db, "login", u.Email)
	if err != nil {
		return 0, false, err
	}
	ok, _ := authlib.Check(password, u.PasswordHash)
	return attempt, ok, nil
}

type APITokensIn struct {
	Form APITokenForm `maago:"form"`
}

type APITokensPage struct {
	Form   APITokenForm
	Tokens []APIToken
	Made   string // the new token, shown this once
}

// APITokens lists the user's API tokens, with the form for a new one.
func APITokens(ctx context.Context, u User, db maago.DB, in APITokensIn) (APITokensPage, error) {
	tokens, err := UserAPITokens(ctx, db, UserAPITokensParams{UserID: u.ID})
	return APITokensPage{Form: in.Form, Tokens: tokens}, err
}

// APITokensSubmit makes an API token, given the password again: a leaked
// token cannot make more. Without the request's transaction (maago.NoTx),
// like LoginSubmit, since it asks the password.
func APITokensSubmit(ctx context.Context, u User, db maago.DB, in APITokensIn) (maago.Result[APITokensPage], error) {
	page := APITokensPage{Form: in.Form}
	if in.Form.Valid() {
		attempt, ok, err := recheck(ctx, db, u, in.Form.Password.Value)
		if err != nil {
			return maago.Result[APITokensPage]{}, err
		}
		if !ok {
			page.Form.Password.Error = "Your password is incorrect."
		} else {
			if err := DeleteAttempt(ctx, db, attempt); err != nil {
				return maago.Result[APITokensPage]{}, err
			}
			token, hash := authlib.NewToken()
			if _, err := InsertAPIToken(ctx, db, NewAPITokenInput{UserID: u.ID, Name: in.Form.Name.Value, Hash: hash}); err != nil {
				return maago.Result[APITokensPage]{}, err
			}
			page = APITokensPage{Made: token}
		}
	}
	tokens, err := UserAPITokens(ctx, db, UserAPITokensParams{UserID: u.ID})
	if err != nil {
		return maago.Result[APITokensPage]{}, err
	}
	page.Tokens = tokens
	return maago.Show(page), nil
}

type APITokenDeleteIn struct {
	ID int64 `maago:"path=id"`
}

// APITokenDelete removes one of the user's API tokens; another's is a 404.
func APITokenDelete(ctx context.Context, u User, tx maago.Tx, in APITokenDeleteIn) (maago.Result[APITokensPage], error) {
	n, err := DeleteOwnAPIToken(ctx, tx, DeleteOwnAPITokenParams{ID: in.ID, UserID: u.ID})
	if err != nil {
		return maago.Result[APITokensPage]{}, err
	}
	if n == 0 {
		return maago.Result[APITokensPage]{}, sql.ErrNoRows
	}
	return maago.SeeOther[APITokensPage](APITokensURL()), nil
}
