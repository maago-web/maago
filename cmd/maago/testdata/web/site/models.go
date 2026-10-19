// Package site is the fixture app of cmd/maago's TestWeb: routes, views
// and templates over one model.
package site

import "github.com/maago-web/maago"

type Post struct {
	maago.Model

	ID    int64
	Title string `maago:"max=200"`
}

// Tag has two unique constraints: Name, and Kind with Code together. A
// value one refuses is its form's error, with no read before (D-154).
type Tag struct {
	maago.Model `maago:"unique=(Kind,Code)"`

	ID   int64
	Name string `maago:"max=50,unique"`
	Kind string `maago:"max=50"`
	Code string `maago:"max=50"`
}
