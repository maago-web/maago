// Package policy is an app with read policies: the posts its users may
// read, and the comments on them.
package policy

import (
	"time"

	"github.com/maago-web/maago"
)

type User struct {
	maago.Model `maago:"table=users"`

	ID       int64
	Name     string
	IsStaff  bool
	TeamID   *int64
	JoinedAt time.Time
}

type Post struct {
	maago.Model `maago:"table=posts"`

	ID        int64
	Title     string
	Status    string `maago:"choices=draft/published,default='draft'"`
	Hidden    bool
	AuthorID  *int64 `maago:"fk=User"`
	TeamID    *int64
	PublishAt *time.Time
}

type Comment struct {
	maago.Model `maago:"table=comments"`

	ID     int64
	PostID int64 `maago:"fk=Post"`
	Text   string
}
