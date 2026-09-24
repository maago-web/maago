package maago_test

import (
	"time"

	"github.com/maago-web/maago"
)

// Post embeds maago.Model from outside the package, with a tag, as users do.
type Post struct {
	maago.Model `maago:"table=posts"`

	ID          int64
	Title       string `maago:"max=200"`
	PublishedAt *time.Time
}

func ExampleModel() {
	_ = Post{Title: "Hello"}
}
