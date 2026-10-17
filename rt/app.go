package rt

import (
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/internal/rtcore"
)

// AppRoute is one route of an App. Serve answers the request; when it
// returns an error it must not have written anything yet, and the error
// becomes the response (see maago.Status). Once it has written, it handles
// its own failures and returns nil. The writer is an http.Flusher, for
// streaming.
type AppRoute struct {
	Method, Path, Name string
	API                bool
	LoginRequired      bool // a request without a user does not reach Serve
	User               bool // its view takes the user: Config.Auth is needed
	Files              bool // it reads a form with a file: Config.MediaDir is needed
	Serve              func(w http.ResponseWriter, r *http.Request) error
}

// NewApp is the App of the app named name, with its static files, nil
// when it has none. Only generated code and contrib/admin call it.
func NewApp(name string, static *StaticFiles, routes ...AppRoute) maago.App {
	rs := make([]rtcore.Route, len(routes))
	for i, r := range routes {
		rs[i] = rtcore.Route(r) // the same fields: a field added to one alone does not compile
	}
	return rtcore.NewApp(name, static, rs...)
}

// StaticFiles are an app's static/: each file under its own name
// and a name with the hash of its content, which never changes, so
// browsers keep it.
type StaticFiles = rtcore.StaticFiles

// Static reads static/ of fsys, the app's embedded files, for the generated
// code of the app whose package is named app. names are the files the
// project's templates ask for with {{static}}; maago gen has checked them,
// and a missing one panics here, as the process starts.
func Static(app string, fsys fs.FS, names ...string) *StaticFiles {
	return rtcore.Static(app, fsys, names...)
}

// StaticFunc is the static function of templates: {{static "board/app.css"}}
// is the path of the file app.css in static/ of app board under its hashed
// name, after Config.Prefix.
func StaticFunc(ref string) (string, error) { return rtcore.StaticFunc(ref) }

// DateFunc is the date function of templates: {{date "2006-01-02
// 15:04" .CreatedAt}}, or {{.CreatedAt | date "2006-01-02"}}, shows a
// time.Time or *time.Time in Config.TimeZone of the last Handler made (UTC
// before any); a nil or zero time shows nothing. maago gen checks that the layout is a string literal.
func DateFunc(layout string, t any) (string, error) {
	switch v := t.(type) {
	case time.Time:
		if v.IsZero() {
			return "", nil
		}
		return v.In(rtcore.TimeZone()).Format(layout), nil
	case *time.Time:
		if v == nil {
			return "", nil
		}
		return DateFunc(layout, *v)
	}
	return "", fmt.Errorf("date: %T is not a time", t)
}
