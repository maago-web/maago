package maago

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
)

// Media is a file SaveMedia kept in Config.MediaDir: its key,
// the name and type it came with, and its size. Keep the four in a
// model's row, and give the value back to ServeMedia.
type Media struct {
	Key  string // Maago's random name for it: 32 lowercase hex digits
	Name string // the name it was sent under, for showing and downloading
	Type string // its media type, as its content told (File.Type)
	Size int64
}

type mediaKey struct{}

// WithMediaDir returns ctx carrying the media directory, as Handler
// gives each request Config.MediaDir. A view tested without HTTP gets it
// this way.
func WithMediaDir(ctx context.Context, dir string) context.Context {
	return context.WithValue(ctx, mediaKey{}, dir)
}

// mediaRoot opens ctx's media directory: what it opens stays inside it,
// links included.
func mediaRoot(ctx context.Context) (*os.Root, error) {
	dir, _ := ctx.Value(mediaKey{}).(string)
	if dir == "" {
		return nil, errors.New("maago: no media directory in the context; Handler puts Config.MediaDir there, and a test uses maago.WithMediaDir")
	}
	return os.OpenRoot(dir)
}

// validKey reports whether key is one SaveMedia makes: so no other name,
// such as ../x, is ever opened.
func validKey(key string) bool {
	if len(key) != 32 {
		return false
	}
	for _, c := range key {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}

// SaveMedia keeps f in the media directory under a random key, never
// under the name it was sent with, and returns what to keep in a row. It
// writes a temporary file and renames it, so no one opens half a file.
// If the row is not written after, delete the file with DeleteMedia.
func SaveMedia(ctx context.Context, f File) (Media, error) {
	root, err := mediaRoot(ctx)
	if err != nil {
		return Media{}, err
	}
	defer root.Close()
	in, err := f.Open()
	if err != nil {
		return Media{}, fmt.Errorf("maago: saving %q: %w", f.Name, err)
	}
	defer in.Close()
	b := make([]byte, 16)
	rand.Read(b) // never fails (crypto/rand)
	key := hex.EncodeToString(b)
	tmp := ".tmp-" + key // not a key: never opened as one
	out, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return Media{}, fmt.Errorf("maago: saving %q: %w", f.Name, err)
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = root.Rename(tmp, key)
	}
	if err != nil {
		if rerr := root.Remove(tmp); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			err = errors.Join(err, rerr)
		}
		return Media{}, fmt.Errorf("maago: saving %q: %w", f.Name, err)
	}
	return Media{Key: key, Name: f.Name, Type: f.Type, Size: f.Size}, nil
}

// OpenMedia opens the file of key. A key SaveMedia would not make, or of
// no file, is an error that is fs.ErrNotExist.
func OpenMedia(ctx context.Context, key string) (*os.File, error) {
	if !validKey(key) {
		return nil, fmt.Errorf("maago: media key %q: %w", key, fs.ErrNotExist)
	}
	root, err := mediaRoot(ctx)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Open(key)
}

// DeleteMedia deletes the file of key; one already gone is no error, so
// deleting a row's file needs no look first.
func DeleteMedia(ctx context.Context, key string) error {
	if !validKey(key) {
		return fmt.Errorf("maago: media key %q is not one SaveMedia makes", key)
	}
	root, err := mediaRoot(ctx)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Remove(key); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// ServeMedia answers r with m's file, to download: never shown
// as a page of the site, so an HTML or SVG file someone sent does not
// run there, while <img> still draws an image. It answers 404 itself
// when there is no such file. A func(w, r) view calls it once it has
// checked that the user may have the file; any other error panics before
// anything is written, so the response is a 500 that Config.OnError hears.
func ServeMedia(w http.ResponseWriter, r *http.Request, m Media) {
	file, err := OpenMedia(r.Context(), m.Key)
	if errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		panic(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		panic(err)
	}
	typ := "application/octet-stream" // for a type that is none
	if _, _, err := mime.ParseMediaType(m.Type); err == nil {
		typ = m.Type
	}
	disposition := "attachment"
	if m.Name != "" {
		disposition = mime.FormatMediaType("attachment", map[string]string{"filename": m.Name})
	}
	h := w.Header()
	h.Set("Content-Type", typ)
	h.Set("Content-Disposition", disposition)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private")
	http.ServeContent(w, r, "", info.ModTime(), file)
}
