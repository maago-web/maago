// Package tags is an app without media: a program that links only it
// still sweeps with the project's media keys.
package tags

import "github.com/maago-web/maago"

type Tag struct {
	maago.Model

	ID   int64
	Name string
}
