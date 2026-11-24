// Package files is a model with two media keys, one of them optional: a
// sweep keeps the files of both.
package files

import "github.com/maago-web/maago"

type File struct {
	maago.Model

	ID    int64
	Name  string
	Main  string  `maago:"media"`
	Thumb *string `maago:"media"`
}
