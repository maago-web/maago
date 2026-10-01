// Package site is the fixture app of cmd/maago's TestWeb: routes, views
// and templates over one model.
package site

import "github.com/maago-web/maago"

type Post struct {
	maago.Model

	ID    int64
	Title string `maago:"max=200"`
}
