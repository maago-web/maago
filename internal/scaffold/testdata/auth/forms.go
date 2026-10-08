package auth

import "github.com/maago-web/maago"

// LoginForm is what a person signs in with.
type LoginForm struct {
	maago.Form
	Email    maago.Field[string] `maago:"max=254"`
	Password maago.Field[string] `maago:"max=4096"`
}
