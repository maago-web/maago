// Package notes is a second app with a media key, which a sweep from
// another app's program keeps too.
package notes

import "github.com/maago-web/maago"

type Note struct {
	maago.Model

	ID         int64
	Attachment string `maago:"media"`
}
