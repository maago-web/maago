package maago

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Media is a file SaveMedia kept in the media store: its key,
// the name and type it came with, and its size. Keep the four in a
// model's row, and give the value back to ServeMedia.
type Media struct {
	Key  string // Maago's random name for it: 32 lowercase hex digits
	Name string // the name it was sent under, for showing and downloading
	Type string // its media type, as its content told (File.Type)
	Size int64
}

// MediaStore is where media is kept: this machine's disk (LocalMedia,
// for one instance) or a bucket (contrib/s3). Maago makes the keys and
// checks them before a store sees one.
type MediaStore interface {
	// Put keeps r's bytes under key, of media type typ, whole or not at
	// all, and returns how many it kept. A failed Put leaves nothing.
	Put(ctx context.Context, key, typ string, r io.Reader) (int64, error)
	// Open opens key's file. None is an error that is fs.ErrNotExist.
	Open(ctx context.Context, key string) (MediaFile, error)
	// Delete deletes the file of name, a name List gave; none is no error.
	Delete(ctx context.Context, name string) error
	// List returns the files whose names sort after after, in byte order,
	// a page at a time; none when there are no more. It gives every file,
	// keys or not (a half-written one's .tmp- name, a .gitignore).
	List(ctx context.Context, after string) ([]fs.FileInfo, error)
	// URL is a link to m's file, to download, that anyone may follow
	// for ttl, whole seconds.
	URL(m Media, ttl time.Duration) (string, error)
}

// MediaFile is a media file open to read from any offset, as
// http.ServeContent reads a Range. Stat tells its size and time.
type MediaFile interface {
	fs.File
	io.Seeker
}

type mediaKey struct{}

// WithMedia returns ctx carrying the media store s, as Handler gives each
// request Config's. A worker, or a view tested without HTTP, gets it this
// way.
func WithMedia(ctx context.Context, s MediaStore) context.Context {
	return context.WithValue(ctx, mediaKey{}, s)
}

// mediaStore is ctx's media store.
func mediaStore(ctx context.Context) (MediaStore, error) {
	s, _ := ctx.Value(mediaKey{}).(MediaStore)
	if s == nil {
		return nil, errors.New("maago: no media store in the context; Handler puts Config's there, and a test or a worker uses maago.WithMedia")
	}
	return s, nil
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

// SaveMedia keeps f in the media store under a random key, never under
// the name it was sent with, and returns what to keep in a row. No one
// opens half a file.
// If the row is not written after, delete the file with DeleteMedia.
func SaveMedia(ctx context.Context, f File) (Media, error) {
	s, err := mediaStore(ctx)
	if err != nil {
		return Media{}, err
	}
	in, err := f.Open()
	if err != nil {
		return Media{}, fmt.Errorf("maago: saving %q: %w", f.Name, err)
	}
	defer in.Close()
	b := make([]byte, 16)
	rand.Read(b) // never fails (crypto/rand)
	key := hex.EncodeToString(b)
	if _, err := s.Put(ctx, key, f.Type, in); err != nil {
		return Media{}, fmt.Errorf("maago: saving %q: %w", f.Name, err)
	}
	return Media{Key: key, Name: f.Name, Type: f.Type, Size: f.Size}, nil
}

// OpenMedia opens the file of key. A key SaveMedia would not make, or of
// no file, is an error that is fs.ErrNotExist.
func OpenMedia(ctx context.Context, key string) (MediaFile, error) {
	if !validKey(key) {
		return nil, fmt.Errorf("maago: media key %q: %w", key, fs.ErrNotExist)
	}
	s, err := mediaStore(ctx)
	if err != nil {
		return nil, err
	}
	return s.Open(ctx, key)
}

// DeleteMedia deletes the file of key; one already gone is no error, so
// deleting a row's file needs no look first.
func DeleteMedia(ctx context.Context, key string) error {
	if !validKey(key) {
		return fmt.Errorf("maago: media key %q is not one SaveMedia makes", key)
	}
	s, err := mediaStore(ctx)
	if err != nil {
		return err
	}
	return s.Delete(ctx, key)
}

// MediaURL is a link to m's file, to download, that anyone may follow
// for ttl: from a second to 7 days, cut to whole seconds. Hand it to
// another service, or to a browser, once the view or worker has checked
// that the user may have the file: until it expires, whoever holds the
// URL may download the file.
func MediaURL(ctx context.Context, m Media, ttl time.Duration) (string, error) {
	if !validKey(m.Key) {
		return "", fmt.Errorf("maago: media key %q is not one SaveMedia makes", m.Key)
	}
	if ttl < time.Second || ttl > 7*24*time.Hour {
		return "", fmt.Errorf("maago: a media URL lasts from a second to 7 days, not %v", ttl)
	}
	s, err := mediaStore(ctx)
	if err != nil {
		return "", err
	}
	return s.URL(m, ttl.Truncate(time.Second))
}

// ServeMedia answers r with m's file, to download: never shown
// as a page of the site, so an HTML or SVG file someone sent does not
// run there, while <img> still draws an image. A view calls it once it
// has checked that the user may have the file, and returns its error:
// one for no such file is a 404, any other a 500 that Config.OnError
// hears. Either comes before anything is written. A func(w, r) view,
// which returns nothing, answers it itself.
func ServeMedia(w http.ResponseWriter, r *http.Request, m Media) error {
	file, err := OpenMedia(r.Context(), m.Key)
	if errors.Is(err, fs.ErrNotExist) {
		return Status(http.StatusNotFound, err)
	}
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
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
	return nil
}

// LocalMedia keeps media in Dir on this machine's disk: for one
// instance, since another machine does not see the files.
type LocalMedia struct {
	Dir     string // the directory, which must exist
	SiteURL string // the site's address as browsers and other services reach it, Config.Prefix included: https://example.com/app; URL needs it
}

// root opens l.Dir once name is a file's name in it, no path: what it
// opens stays inside, links included.
func (l LocalMedia) root(name string) (*os.Root, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return nil, fmt.Errorf("maago: %q is not the name of a file in the media directory", name)
	}
	return os.OpenRoot(l.Dir)
}

// Put writes a temporary file and renames it, so no one opens half a
// file.
func (l LocalMedia) Put(ctx context.Context, key, typ string, r io.Reader) (int64, error) {
	root, err := l.root(key)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	tmp := ".tmp-" + key // not a key: never opened as one
	out, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, r)
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
		return 0, err
	}
	return n, nil
}

func (l LocalMedia) Open(ctx context.Context, key string) (MediaFile, error) {
	root, err := l.root(key)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(key)
	if err != nil {
		return nil, err // not a nil *os.File in a MediaFile
	}
	return f, nil
}

func (l LocalMedia) Delete(ctx context.Context, name string) error {
	root, err := l.root(name)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// List gives all the files in one page: the next is none.
func (l LocalMedia) List(ctx context.Context, after string) ([]fs.FileInfo, error) {
	entries, err := os.ReadDir(l.Dir) // in name order
	if err != nil {
		return nil, err
	}
	var infos []fs.FileInfo
	for _, e := range entries {
		if !e.Type().IsRegular() || e.Name() <= after {
			continue
		}
		info, err := e.Info()
		if errors.Is(err, fs.ErrNotExist) { // deleted since
			continue
		}
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

// URL is a link to the route Handler serves signed media at, under
// SiteURL. Only the process that made a URL serves it, and not after a
// restart: the signing key is the process's own.
func (l LocalMedia) URL(m Media, ttl time.Duration) (string, error) {
	if !validKey(m.Key) {
		return "", fmt.Errorf("maago: media key %q is not one SaveMedia makes", m.Key)
	}
	if l.SiteURL == "" {
		return "", errors.New("maago: a local media store makes URLs only with its SiteURL: set Config.Media to maago.LocalMedia{Dir: …, SiteURL: …} in place of Config.MediaDir")
	}
	u, err := l.site()
	if err != nil {
		return "", err
	}
	q := url.Values{"key": {m.Key}, "name": {m.Name}, "type": {m.Type}, "expires": {strconv.FormatInt(time.Now().Add(ttl).Unix(), 10)}}
	q.Set("sig", sign(q))
	u = u.JoinPath("maago", "media")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// site is SiteURL without a last /, once it is a site's address.
func (l LocalMedia) site() (*url.URL, error) {
	u, err := url.Parse(strings.TrimSuffix(l.SiteURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("maago: LocalMedia.SiteURL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, fmt.Errorf("maago: LocalMedia.SiteURL %q is not a site's address such as https://example.com/app: http or https, a host, and no user, query or fragment", l.SiteURL)
	}
	return u, nil
}

// signKey signs the local URLs of this process.
var signKey = func() []byte {
	b := make([]byte, 32)
	rand.Read(b) // never fails (crypto/rand)
	return b
}()

// sign is the signature of a local URL's query q, all of it but sig.
func sign(q url.Values) string {
	mac := hmac.New(sha256.New, signKey)
	io.WriteString(mac, q.Encode())
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// serveSigned serves the file of a local URL while it lasts, as made.
func serveSigned(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	sig := q.Get("sig")
	q.Del("sig")
	if !hmac.Equal([]byte(sig), []byte(sign(q))) {
		return Status(http.StatusForbidden, errors.New("the media URL is not one this process signed"))
	}
	expires, err := strconv.ParseInt(q.Get("expires"), 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return Status(http.StatusForbidden, errors.New("the media URL has expired"))
	}
	return ServeMedia(w, r, Media{Key: q.Get("key"), Name: q.Get("name"), Type: q.Get("type")})
}
