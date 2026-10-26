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

// MediaIn is the key of the file MediaDownload sends.
type MediaIn struct {
	Key string `maago:"path=key"`
}

// MediaDownload sends a saved file down: it writes the response itself
// (D-157), and returns ServeMedia's error. A real view would read its row
// and check the user may have it first (D-140).
func MediaDownload(ctx context.Context, in MediaIn, w http.ResponseWriter, r *http.Request) error {
	return maago.ServeMedia(w, r, maago.Media{Key: in.Key, Name: "a.png", Type: "image/png"})
}
