package rt

import (
	"bytes"
	"net/http"
)

// Buffer holds a response until the transaction commits, so that a page
// that fails to render rolls it back. Only generated code uses it.
type Buffer struct {
	header http.Header
	body   bytes.Buffer
}

// NewBuffer is an empty Buffer.
func NewBuffer() *Buffer { return &Buffer{header: http.Header{}} }

// Header is the header of the response to come.
func (b *Buffer) Header() http.Header { return b.header }

// Write adds p to the body.
func (b *Buffer) Write(p []byte) (int, error) { return b.body.Write(p) }

// WriteHeader is not for a Buffer: the response is a 200.
func (b *Buffer) WriteHeader(int) { panic("maago: WriteHeader on a Buffer") }

// Send writes the response to w.
func (b *Buffer) Send(w http.ResponseWriter) error {
	for k, v := range b.header {
		w.Header()[k] = v
	}
	_, err := w.Write(b.body.Bytes())
	return err
}
