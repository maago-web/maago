package auth

import (
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/maago-web/maago"
)

// LoginForm is what a person signs in with.
type LoginForm struct {
	maago.Form
	Email    maago.Field[string] `maago:"max=254"`
	Password maago.Field[string] `maago:"max=4096"`
}

// SignupForm is what a person signs up with.
type SignupForm struct {
	maago.Form
	Email    maago.Field[string] `maago:"max=254"`
	Password maago.Field[string] `maago:"max=4096"`
}

// Validate wants one plain ASCII address: a name, a list, or non-ASCII is
// not one a mail goes to.
func (f *SignupForm) Validate() {
	email := normalizeEmail(f.Email.Value)
	a, err := mail.ParseAddress(email)
	if err != nil || a.Address != email || strings.ContainsFunc(email, func(r rune) bool { return r >= utf8.RuneSelf }) {
		f.Email.Error = "Enter a valid email address."
	}
}

// VerifyForm carries the token of a verification link, and the password:
// who signed up with another's email cannot verify it.
type VerifyForm struct {
	maago.Form
	Token    maago.Field[string] `maago:"max=64"`
	Password maago.Field[string] `maago:"max=4096"`
}

// VerifyResendForm asks for the verification mail again.
type VerifyResendForm struct {
	maago.Form
	Email maago.Field[string] `maago:"max=254"`
}

// PasswordChangeForm changes the password of the signed-in user.
type PasswordChangeForm struct {
	maago.Form
	OldPassword maago.Field[string] `maago:"max=4096"`
	NewPassword maago.Field[string] `maago:"max=4096"`
}

// PasswordResetForm asks for a link to reset the password.
type PasswordResetForm struct {
	maago.Form
	Email maago.Field[string] `maago:"max=254"`
}

// PasswordResetConfirmForm sets a new password with the token of the link.
type PasswordResetConfirmForm struct {
	maago.Form
	Token    maago.Field[string] `maago:"max=64"`
	Password maago.Field[string] `maago:"max=4096"`
}
