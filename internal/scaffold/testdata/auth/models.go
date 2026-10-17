// Package auth is the accounts of the project: who may sign in.
// "maago add auth" wrote it, and it is yours now: add fields to User as
// the project needs.
//
// The first superuser comes from main, given -createsuperuser, with
// the email and password in the environment of the shell (not .env, and
// no flag: a flag stays in ps and the shell's history), once maago
// migrate has run:
//
//	createSuperuser := flag.Bool("createsuperuser", false, "create a superuser from MAAGO_SUPERUSER_EMAIL and MAAGO_SUPERUSER_PASSWORD")
//	flag.Parse()
//	// … open db …
//	if *createSuperuser {
//		u, err := auth.CreateSuperuser(ctx, db, os.Getenv("MAAGO_SUPERUSER_EMAIL"), os.Getenv("MAAGO_SUPERUSER_PASSWORD"))
//		if err != nil {
//			log.Fatal(err)
//		}
//		log.Printf("created superuser %s (id %d)", u.Email, u.ID)
//		return
//	}
package auth

import (
	"time"

	"github.com/maago-web/maago"
)

// User is an account. It may sign in once its email is verified and while
// it is active: IsActive && VerifiedAt != nil.
//
// The admin saves a User through the rules of its model form only, not
// through the views:
//   - The views store and look up the email in lowercase, with no spaces
//     around it; the admin saves it as typed. Changed to Ann@example.com,
//     the account cannot sign in, and two accounts can differ by case.
//   - Closing an account (AccountClose) deletes its sessions and tokens;
//     turning IsActive off in the admin does not. While it is off, Current
//     refuses the sessions and API tokens, and the views the one-time
//     tokens; once it is on, those not yet expired work again.
//   - The admin has no PasswordHash, so it adds no User: an account comes
//     from signing up, or from -createsuperuser, and its password changes
//     by PasswordChange or PasswordReset.
//   - The last superuser who turns off their own IsSuperuser, IsStaff or
//     IsActive leaves no one to turn it on again in the admin: run
//     -createsuperuser with another email.
//   - Its sessions and tokens keep a User from being deleted, and the
//     admin hides them: the user closes the account instead, or the admin
//     turns IsActive off.
type User struct {
	maago.Model

	ID           int64
	Email        string     `maago:"max=254,unique,help='lowercase, as the user signs in'"`
	PasswordHash string     `maago:"admin=hidden"`
	IsActive     bool       `maago:"default=true"`  // false: closed by the user or an admin
	IsStaff      bool       `maago:"default=false"` // may use the staff pages and the admin
	IsSuperuser  bool       `maago:"default=false"` // may do anything in the admin
	VerifiedAt   *time.Time // when the email was verified; nil: not yet
	CreatedAt    time.Time  `maago:"auto=create"`
}

// Permission lets a staff user do an action to a model in the admin. App
// and ModelName are written as the admin's URL has them, such as board
// and Post (typed, with no picker): a row with another spelling allows
// nothing. There are no groups of permissions.
type Permission struct {
	maago.Model `maago:"unique=(UserID,App,ModelName,Action)"`

	ID        int64
	UserID    int64  `maago:"fk=User"`
	App       string `maago:"max=100"`
	ModelName string `maago:"max=100"`
	Action    string `maago:"choices=view/add/change/delete"`
}

// Session is a signed-in browser. Its cookie holds a token; Hash is the
// token's hash, so the table holds no token.
type Session struct {
	maago.Model `maago:"admin=hidden"`

	ID        int64
	UserID    int64     `maago:"fk=User"`
	Hash      string    `maago:"max=64,unique"`
	ExpiresAt time.Time // the session ends then
	CreatedAt time.Time `maago:"auto=create"`
}

// Token is a one-time token that a mail carries: to verify the email, or
// to reset the password. Hash is the token's hash; Purpose keeps a
// verification token from resetting a password.
type Token struct {
	maago.Model `maago:"admin=hidden"`

	ID        int64
	UserID    int64     `maago:"fk=User"`
	Purpose   string    `maago:"choices=verify/reset"`
	Hash      string    `maago:"max=64,unique"`
	ExpiresAt time.Time // the token is no use after it
	CreatedAt time.Time `maago:"auto=create"`
}

// Attempt is an attempt to sign in or to have a mail sent, by the email
// it was for and the IP it came from: too many in a while are refused.
type Attempt struct {
	maago.Model `maago:"index=(Action,CreatedAt),admin=hidden"`

	ID        int64
	Action    string `maago:"choices=login/mail"`
	Email     string `maago:"max=254"`
	IP        string
	CreatedAt time.Time `maago:"auto=create"`
}

// APIToken lets a program call the API as its user, with the header
// Authorization: Bearer and the token. Hash is the token's hash; the
// token is shown once, when it is made.
type APIToken struct {
	maago.Model `maago:"admin=hidden"`

	ID        int64
	UserID    int64     `maago:"fk=User"`
	Name      string    `maago:"max=100"` // what it is for
	Hash      string    `maago:"max=64,unique"`
	CreatedAt time.Time `maago:"auto=create"`
}
