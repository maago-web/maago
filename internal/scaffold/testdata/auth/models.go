// Package auth is the accounts of the project: who may sign in.
// "maago add auth" wrote it, and it is yours now: add fields to User as
// the project needs.
package auth

import (
	"time"

	"github.com/maago-web/maago"
)

// User is an account. It may sign in once its email is verified and while
// it is active: IsActive && VerifiedAt != nil.
type User struct {
	maago.Model

	ID           int64
	Email        string `maago:"max=254,unique"`
	PasswordHash string
	IsActive     bool       `maago:"default=true"`  // false: closed by the user or an admin
	IsStaff      bool       `maago:"default=false"` // may use the staff pages
	VerifiedAt   *time.Time // when the email was verified; nil: not yet
	CreatedAt    time.Time  `maago:"auto=create"`
}
