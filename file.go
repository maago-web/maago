package maago

import (
	"errors"
	"mime/multipart"
)

// File is a file a form's file field received. It lives as long
// as the request: keep it with SaveMedia, or Open it, in the view.
type File struct {
	Name string // the name it was sent under, for showing: never a path to write to
	Size int64
	Type string // its media type, as its first bytes tell (http.DetectContentType), not as the browser said

	header *multipart.FileHeader
}

// Open opens the file's content.
func (f File) Open() (multipart.File, error) {
	if f.header == nil {
		return nil, errors.New("maago: no file")
	}
	return f.header.Open()
}
