package blog

import (
	"time"

	"github.com/maago-web/maago"
)

type Board struct {
	maago.Model `maago:"table=boards"`

	ID   int64
	Name string `maago:"max=100,unique"`
}

type Post struct {
	maago.Model `maago:"table=posts"`

	ID          int64
	BoardID     int64  `maago:"fk=Board"`
	Title       string `maago:"max=200"`
	AuthorName  *string
	PublishedAt *time.Time
	URL         string // column url: its field name is not fieldName("url")
}
