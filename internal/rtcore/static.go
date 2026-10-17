package rtcore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// StaticFiles is rt.StaticFiles.
type StaticFiles struct {
	app   string
	files map[string]staticFile // by the name and by the hashed name
}

type staticFile struct {
	name   string // the name in static/, which tells the type
	hashed string
	etag   string
	body   []byte
}

// Static is rt.Static.
func Static(app string, fsys fs.FS, names ...string) *StaticFiles {
	entries, err := fs.ReadDir(fsys, "static")
	if err != nil {
		panic(fmt.Sprintf("maago: static files of app %s: %v", app, err))
	}
	s := &StaticFiles{app, map[string]staticFile{}}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			panic(fmt.Sprintf("maago: static/%s of app %s: directory in static/; run maago gen", name, app))
		}
		if err := StaticName(name); err != nil {
			panic(fmt.Sprintf("maago: static/%s of app %s: %v; run maago gen", name, app, err))
		}
		body, err := fs.ReadFile(fsys, "static/"+name)
		if err != nil {
			panic(fmt.Sprintf("maago: static files of app %s: %v", app, err))
		}
		sum := sha256.Sum256(body)
		f := staticFile{name, hashedName(name, hex.EncodeToString(sum[:8])), `"` + hex.EncodeToString(sum[:]) + `"`, body}
		for _, key := range []string{f.name, f.hashed} {
			if other, ok := s.files[key]; ok {
				panic(fmt.Sprintf("maago: static/%s and static/%s of app %s would share the name %s; rename one", other.name, name, app, key))
			}
			s.files[key] = f
		}
	}
	for _, n := range names {
		if f, ok := s.files[n]; !ok || f.name != n {
			panic(fmt.Sprintf("maago: app %s has no static/%s; run maago gen", app, n))
		}
	}
	return s
}

// StaticName says what is wrong with name as the name of a static file,
// if anything: it starts with a letter or digit, and holds letters,
// digits, '.', '-' and '_' only. So //go:embed keeps the file and a URL
// keeps the name as it is. maago gen checks with it too.
func StaticName(name string) error {
	for i, c := range name {
		ok := 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
		if !ok && (i == 0 || c != '.' && c != '-' && c != '_') {
			return fmt.Errorf("static file name %q does not start with a letter or digit and hold only letters, digits, '.', '-' and '_'", name)
		}
	}
	if name == "" {
		return errors.New("empty static file name")
	}
	return nil
}

// hashedName puts the hash before the last extension: app.css is
// app.<hash>.css; a name with no extension ends in .<hash>.
func hashedName(name, hash string) string {
	ext := path.Ext(name)
	return strings.TrimSuffix(name, ext) + "." + hash + ext
}

// Serve answers a file by either name: the hashed one for a year, the
// other always asked again. Neither runs Config.Auth: these files are
// for anyone.
func (s *StaticFiles) Serve(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	f, ok := s.files[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if name == f.hashed {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.Header().Set("ETag", f.etag)
	http.ServeContent(w, r, f.name, time.Time{}, bytes.NewReader(f.body))
}

// StaticFunc is rt.StaticFunc.
func StaticFunc(ref string) (string, error) {
	app, name, _ := strings.Cut(ref, "/")
	p := Current.Load()
	if p == nil {
		return "", fmt.Errorf("static %q: no Handler made yet", ref)
	}
	s, ok := p.Static[app]
	if !ok {
		return "", fmt.Errorf("static %q: no app %s with static files in Config.Apps", ref, app)
	}
	f, ok := s.files[name]
	if !ok || f.name != name {
		return "", fmt.Errorf("static %q: app %s has no static/%s", ref, app, name)
	}
	return p.Prefix + "/static/" + app + "/" + f.hashed, nil
}

// App is the package name of the app whose static files these are.
func (s *StaticFiles) App() string { return s.app }
