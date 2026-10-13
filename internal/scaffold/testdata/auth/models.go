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
//		log.Printf("created %s", u.Email)
//		return
//	}
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
	Email        string     `maago:"max=254,unique"`
	PasswordHash string     `maago:"admin=hidden"`
	IsActive     bool       `maago:"default=true"`  // false: closed by the user or an admin
	IsStaff      bool       `maago:"default=false"` // may use the staff pages and the admin
	IsSuperuser  bool       `maago:"default=false"` // may do anything in the admin
	VerifiedAt   *time.Time // when the email was verified; nil: not yet
	CreatedAt    time.Time  `maago:"auto=create"`
}

// Permission lets a staff user do an action to a model in the admin. App
// and ModelName are written as the admin's URL has them, such as board
// and Post: a row with another spelling allows nothing.
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
