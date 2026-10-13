package site

import (
	"context"

	"github.com/maago-web/maago"
)

// UploadForm is a form of a file alone (D-139).
type UploadForm struct {
	maago.Form
	Doc maago.Field[maago.File] `maago:"accept=png"`
}

type UploadIn struct {
	Form UploadForm `maago:"form"`
}

type UploadPage struct {
	Form UploadForm
	Got  string
}

func Upload(ctx context.Context, in UploadIn) (UploadPage, error) {
	p := UploadPage{Form: in.Form}
	if in.Form.Valid() {
		f := in.Form.Doc.Value
		p.Got = f.Name + " " + f.Type
	}
	return p, nil
}
