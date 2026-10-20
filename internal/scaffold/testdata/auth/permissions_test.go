package auth

import (
	"net/url"
	"testing"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/maagotest"
)

// Who may ask each page of this app: nobody, or a signed-in user. A
// route added without its row fails here. The rows that end a session
// come last, each for one user: bob signs out, ann closes her account,
// and neither row asks as a user whose session is gone.
func TestPermissions(t *testing.T) {
	h, db := site(t)
	maagotest.Mailbox(t)
	u := newUser(t, db, "ann@example.com", nil)
	session, err := SignIn(t.Context(), db, u)
	if err != nil {
		t.Fatal(err)
	}
	ann := maagotest.As(h, session)
	session, err = SignIn(t.Context(), db, newUser(t, db, "bob@example.com", nil))
	if err != nil {
		t.Fatal(err)
	}
	bob := maagotest.As(h, session)
	token, err := InsertAPIToken(t.Context(), db, NewAPITokenInput{UserID: u.ID, Name: "laptop", Hash: "h"})
	if err != nil {
		t.Fatal(err)
	}
	// Each row that counts an attempt has an email of its own: the
	// limit then counts no other row's.
	form := func(email string, kv ...string) string {
		v := url.Values{"email": {email}}
		for i := 0; i < len(kv); i += 2 {
			v.Set(kv[i], kv[i+1])
		}
		return v.Encode()
	}
	// bob is another user: what ann owns is not his.
	want := func(nobody, bob, ann int) map[string]int {
		return map[string]int{"nobody": nobody, "bob": bob, "ann": ann}
	}
	maagotest.Permissions(t, []maago.App{App}, []maagotest.Role{{Name: "nobody", Handler: h}, {Name: "bob", Handler: bob}, {Name: "ann", Handler: ann}},
		maagotest.Row{Route: "login", URL: LoginURL(), Want: want(200, 200, 200)},
		maagotest.Row{Route: "login_submit", URL: LoginSubmitURL(), Body: form("a@example.com", "password", "wrong"), Want: want(200, 200, 200)},
		maagotest.Row{Route: "signup", URL: SignupURL(), Want: want(200, 200, 200)},
		maagotest.Row{Route: "signup_submit", URL: SignupSubmitURL(), Body: form("b@example.com", "password", "short"), Want: want(200, 200, 200)},
		maagotest.Row{Route: "verify", URL: VerifyURL(), Want: want(200, 200, 200)},
		maagotest.Row{Route: "verify_submit", URL: VerifySubmitURL(), Body: form("c@example.com", "token", "x", "password", "wrong"), Want: want(200, 200, 200)},
		maagotest.Row{Route: "verify_resend", URL: VerifyResendURL(), Want: want(200, 200, 200)},
		maagotest.Row{Route: "verify_resend_submit", URL: VerifyResendSubmitURL(), Body: form("d@example.com"), Want: want(200, 200, 200)},
		maagotest.Row{Route: "password_change", URL: PasswordChangeURL(), Want: want(303, 200, 200)},
		maagotest.Row{Route: "password_change_submit", URL: PasswordChangeSubmitURL(), Body: form("", "old_password", "wrong", "new_password", "another horse battery"), Want: want(401, 200, 200)},
		maagotest.Row{Route: "password_reset", URL: PasswordResetURL(), Want: want(200, 200, 200)},
		maagotest.Row{Route: "password_reset_submit", URL: PasswordResetSubmitURL(), Body: form("e@example.com"), Want: want(200, 200, 200)},
		maagotest.Row{Route: "password_reset_confirm", URL: PasswordResetConfirmURL(), Want: want(200, 200, 200)},
		maagotest.Row{Route: "password_reset_confirm_submit", URL: PasswordResetConfirmSubmitURL(), Body: form("", "token", "x", "password", "another horse battery"), Want: want(200, 200, 200)},
		maagotest.Row{Route: "api_tokens", URL: APITokensURL(), Want: want(303, 200, 200)},
		maagotest.Row{Route: "api_tokens_submit", URL: APITokensSubmitURL(), Body: form("", "name", "phone", "password", "wrong"), Want: want(401, 200, 200)},
		maagotest.Row{Route: "api_token_delete", URL: APITokenDeleteURL(token.ID), Want: want(401, 404, 303)},
		maagotest.Row{Route: "account_close", URL: AccountCloseURL(), Want: want(303, 200, 200)},
		maagotest.Row{Route: "logout", URL: LogoutURL(), Want: want(303, 303, maagotest.Skip)},
		maagotest.Row{Route: "account_close_submit", URL: AccountCloseSubmitURL(), Body: form("", "password", password), Want: want(401, maagotest.Skip, 303)},
	)
}

// A closed or unverified account's session would be nobody's: SignIn
// refuses it, so a table's user is never nobody unseen.
func TestSignInRefuses(t *testing.T) {
	_, db := site(t)
	for _, u := range []User{
		newUser(t, db, "closed@example.com", func(in *NewUserInput) { in.IsActive = maago.Set(false) }),
		newUser(t, db, "new@example.com", func(in *NewUserInput) { in.VerifiedAt = nil }),
	} {
		if c, err := SignIn(t.Context(), db, u); err == nil {
			t.Errorf("%s: %v", u.Email, c)
		}
	}
}
