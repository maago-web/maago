// Package plain is an app without times, choices or slices: its generated
// files import only what they use.
package plain

import "github.com/maago-web/maago"

type Note struct {
	maago.Model `maago:"table=notes"`

	ID   int64
	Body string
}
