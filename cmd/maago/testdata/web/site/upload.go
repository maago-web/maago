package site

import (
	"context"
	"net/http"

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

// UploadPage draws a second form, NewPostForm, which its In does not
// read: NewNewPostForm describes it.
type UploadPage struct {
	Form UploadForm
	Post NewPostForm
	Got  string
}

func Upload(ctx context.Context, in UploadIn) (UploadPage, error) {
	p := UploadPage{Form: in.Form, Post: NewNewPostForm()}
	if in.Form.Valid() {
		m, err := maago.SaveMedia(ctx, in.Form.Doc.Value)
		if err != nil {
			return p, err
		}
		p.Got = m.Name + " " + m.Type + " " + m.Key
	}
	return p, nil
}

// MediaDownload sends a saved file down; a real view would read its row
// and check the user may have it first (D-140).
func MediaDownload(w http.ResponseWriter, r *http.Request) {
	maago.ServeMedia(w, r, maago.Media{Key: r.PathValue("key"), Name: "a.png", Type: "image/png"})
}
