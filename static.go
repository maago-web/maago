package maago

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// StaticFiles is an app's static/ (D-136): each file under its own name
// and a name with the hash of its content, which never changes, so
// browsers keep it.
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

// Static reads static/ of fsys, the app's embedded files, for the generated
// code of the app whose package is named app. names are the files the
// project's templates ask for with {{static}}; maago gen has checked them,
// and a missing one panics here, as the process starts (D-136).
func Static(app string, fsys fs.FS, names ...string) *StaticFiles {
	entries, err := fs.ReadDir(fsys, "static")
	if err != nil {
		panic(fmt.Sprintf("maago: static files of app %s: %v", app, err))
	}
	s := &StaticFiles{app, map[string]staticFile{}}
	for _, e := range entries {
		name := e.Name()
		switch {
		case e.IsDir():
			panic(fmt.Sprintf("maago: static/%s of app %s is a directory; static/ holds files only; run maago gen", name, app))
		case url.PathEscape(name) != name:
			panic(fmt.Sprintf("maago: static/%s of app %s: a URL would escape the name; run maago gen", name, app))
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

// hashedName puts the hash before the last extension: app.css is
// app.<hash>.css; a name with no extension ends in .<hash>.
func hashedName(name, hash string) string {
	ext := path.Ext(name)
	if ext == name { // .env: all extension, no stem
		ext = ""
	}
	return strings.TrimSuffix(name, ext) + "." + hash + ext
}

// WithStatic is the App with its static files, which Handler serves at
// /static/APP/ (D-137). Only generated code calls it.
func (a App) WithStatic(s *StaticFiles) App {
	a.static = s
	return a
}

// serve answers a file by either name: the hashed one for a year, the
// other always asked again. Neither runs Config.Auth: these files are
// for anyone.
func (s *StaticFiles) serve(w http.ResponseWriter, r *http.Request) {
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

// StaticFunc is the static function of templates: {{static "board/app.css"}}
// is the path of the file app.css in static/ of app board under its hashed
// name, after Config.Prefix (D-137). maago gen has checked the app and the
// file; the files are those of the last Handler made, so before any it is
// an error, as is a name maago gen would not let through.
func StaticFunc(ref string) (string, error) {
	app, name, _ := strings.Cut(ref, "/")
	p := current.Load()
	if p == nil {
		return "", fmt.Errorf("static %q: no Handler made yet", ref)
	}
	s, ok := p.static[app]
	if !ok {
		return "", fmt.Errorf("static %q: no app %s with static files in Config.Apps", ref, app)
	}
	f, ok := s.files[name]
	if !ok || f.name != name {
		return "", fmt.Errorf("static %q: app %s has no static/%s", ref, app, name)
	}
	return p.prefix + "/static/" + app + "/" + f.hashed, nil
}
