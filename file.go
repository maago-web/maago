package maago

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"slices"
)

// File is a file a form's file field received (D-139). It lives as long
// as the request: Open it in the view.
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

// fileKinds are the kinds of file a field's accept= tag names, and the
// media type http.DetectContentType gives each. zip includes what is a
// zip inside, such as .docx and .xlsx.
var fileKinds = map[string]string{
	"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif", "webp": "image/webp",
	"pdf": "application/pdf", "zip": "application/zip",
}

// FileKinds are the names accept= takes. maago gen checks with them.
func FileKinds() []string { return slices.Sorted(maps.Keys(fileKinds)) }

// multipartMemory is how much of the files a form sent is kept in memory;
// the rest goes to temporary files, which Handler removes after the route.
const multipartMemory = 1 << 20

// MultipartValues is FormValues for a form with a file field (D-139): the
// body must be multipart/form-data, since a file sent any other way is
// lost; a 400 otherwise. Each of the form's keys may come once, as text or
// as a file. The values it returns are the text ones; BindFile reads the
// files from r.
func MultipartValues(r *http.Request, keys ...string) (url.Values, error) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return nil, nil
	}
	if err := r.ParseMultipartForm(multipartMemory); err != nil {
		return nil, Status(http.StatusBadRequest, fmt.Errorf("form: %w", err)) // a body past Config.MaxBody is a 413 still
	}
	mf := r.MultipartForm
	for _, k := range keys {
		if n := len(mf.Value[k]) + len(mf.File[k]); n > 1 {
			return nil, Status(http.StatusBadRequest, fmt.Errorf("form key %q is given %d times", k, n))
		}
	}
	return url.Values(mf.Value), nil
}

// BindFile reads a file field. Unlike the other Bind functions it takes
// the request, which holds the files; vals are what MultipartValues gave,
// nil for a GET or HEAD. A field the browser sent with no file chosen is
// not given. Raw is the name the file was sent under.
func BindFile(r *http.Request, vals url.Values, f *Field[File], rule Rule) {
	ctx := r.Context()
	f.Name, f.Label, f.Help, f.Choices = rule.Name, rule.Label, rule.Help, rule.Choices
	if vals == nil {
		return
	}
	var h *multipart.FileHeader
	if r.MultipartForm != nil && len(r.MultipartForm.File[rule.Name]) == 1 {
		h = r.MultipartForm.File[rule.Name][0]
	}
	m := settings(ctx).msgs
	if h == nil {
		if !rule.Optional {
			f.Error = m.Required
		}
		return
	}
	f.Raw, f.Given = h.Filename, true
	file, err := h.Open()
	if err != nil {
		f.Error = m.Required // a temporary file gone: as if not sent
		return
	}
	defer file.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(file, head)
	typ, _, _ := mime.ParseMediaType(http.DetectContentType(head[:n]))
	if rule.Accept != nil && !slices.ContainsFunc(rule.Accept, func(k string) bool { return fileKinds[k] == typ }) {
		f.Error = m.NotKind
		return
	}
	f.Value = File{Name: h.Filename, Size: h.Size, Type: typ, header: h}
}
