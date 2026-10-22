// Package board is the fixture of TestPolicyCheck: posts with read
// policies, the boards and comments around them, and the user.
package board

import (
	"time"

	"github.com/maago-web/maago"
)

type User struct {
	maago.Model

	ID       int64
	Name     string
	IsStaff  bool
	TeamID   *int64
	JoinedAt time.Time
}

type Board struct {
	maago.Model

	ID   int64
	Name string
}

type Post struct {
	maago.Model

	ID        int64
	BoardID   int64 `maago:"fk=Board"`
	Status    string
	Hidden    bool
	AuthorID  *int64 `maago:"fk=User"`
	PublishAt *time.Time
}

type Comment struct {
	maago.Model

	ID     int64
	PostID int64 `maago:"fk=Post"`
}
