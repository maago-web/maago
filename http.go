package maago

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"time"
)

// Config is how main puts the apps together into one handler (D-114).
type Config struct {
	DB   *sql.DB
	Apps []App // each app's generated App value: board.App

	// Origins are the other origins trusted like this one, such as
	// "https://app.example.com": their requests pass the cross-origin
	// check, and CORS lets their pages read the answers (D-47). Write each
	// as browsers send it in Origin: lower case, no default port.
	Origins []string

	// OnError hears of every 5xx: a view's error or a panic. Nil logs.
	OnError func(ctx context.Context, r *http.Request, err error)

	// Messages are the errors forms show; an empty one keeps the English
	// default (D-119).
	Messages Messages

	// TimeZone is where the times people enter and see are; nil is UTC.
	TimeZone *time.Location
}

// App is an app's routes, as maago gen generates them into the app's App
// value. Only generated code makes one.
type App struct {
	name   string
	routes []AppRoute
}

// AppRoute is one route of an App. Serve answers the request; when it
// returns an error it must not have written anything yet, and the error
// becomes the response (see Status). Once it has written, it handles its
// own failures and returns nil. The writer is an http.Flusher, for streaming.
type AppRoute struct {
	Method, Path, Name string
	API                bool
	Serve              func(w http.ResponseWriter, r *http.Request) error
}

// NewApp is the App of the app named name. Only generated code calls it.
func NewApp(name string, routes ...AppRoute) App { return App{name, routes} }

// Handler serves the apps of cfg. It rejects what cfg gets wrong; the
// routes themselves maago gen has checked.
func Handler(cfg Config) (http.Handler, error) {
	if cfg.DB == nil {
		return nil, errors.New("maago: Config.DB is nil; open it with maago.OpenDB")
	}
	if len(cfg.Apps) == 0 {
		return nil, errors.New("maago: Config.Apps is empty; add each app's App, such as board.App")
	}
	h := &handler{cfg: cfg, mux: http.NewServeMux(), routes: map[string]*AppRoute{}, forms: newFormSettings(cfg.Messages, cfg.TimeZone)}
	if err := h.forms.msgs.check(); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, app := range cfg.Apps {
		switch {
		case app.name == "":
			return nil, errors.New("maago: Config.Apps holds an empty App; use the one maago gen makes, such as board.App, and run maago gen")
		case seen[app.name]:
			return nil, fmt.Errorf("maago: app %s is in Config.Apps twice", app.name)
		}
		seen[app.name] = true
		for i := range app.routes {
			r := &app.routes[i]
			if r.Serve == nil {
				return nil, fmt.Errorf("maago: route %s of app %s serves nothing; run maago gen", r.Name, app.name)
			}
			pattern := r.Method + " " + r.Path
			h.routes[pattern] = r
			h.mux.Handle(pattern, h.serve(r))
		}
	}
	csrf := http.NewCrossOriginProtection()
	for _, o := range cfg.Origins {
		if err := csrf.AddTrustedOrigin(o); err != nil {
			return nil, fmt.Errorf("maago: Config.Origins: %w", err)
		}
	}
	csrf.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.fail(w, h.route(r), http.StatusForbidden, errors.New("cross-origin request from an origin not in Config.Origins"))
	}))
	h.next = csrf.Handler(h.mux)
	return h, nil
}

type handler struct {
	cfg    Config
	mux    *http.ServeMux
	routes map[string]*AppRoute // by pattern
	forms  *formSettings
	next   http.Handler // CSRF, then the routes
}

// ServeHTTP answers CORS first, since a preflight never reaches a route,
// then lets the cross-origin check and the routes have the request.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		h.next.ServeHTTP(w, r)
		return
	}
	w.Header().Add("Vary", "Origin")
	trusted := slices.Contains(h.cfg.Origins, origin)
	if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
		w.Header().Add("Vary", "Access-Control-Request-Method")
		w.Header().Add("Vary", "Access-Control-Request-Headers")
		method := r.Header.Get("Access-Control-Request-Method")
		if trusted && h.preflightOK(r, method) {
			allow(w, origin)
			w.Header().Set("Access-Control-Allow-Methods", method)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
	} else if trusted {
		allow(w, origin)
	}
	h.next.ServeHTTP(w, r)
}

// preflightOK reports whether a route serves method at r's path, and the
// request asks for no header but Content-Type.
func (h *handler) preflightOK(r *http.Request, method string) bool {
	for _, name := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
		if name = strings.TrimSpace(name); name != "" && !strings.EqualFold(name, "Content-Type") {
			return false
		}
	}
	probe := r.Clone(r.Context())
	probe.Method = method
	_, pattern := h.mux.Handler(probe) // "" when no route has this method
	return h.routes[pattern] != nil
}

func allow(w http.ResponseWriter, origin string) {
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Credentials", "true")
}

// route is the route r reaches, or nil.
func (h *handler) route(r *http.Request) *AppRoute {
	_, pattern := h.mux.Handler(r)
	return h.routes[pattern]
}

// serve runs a route with the database in its context, and turns its
// error or panic into the response.
func (h *handler) serve(route *AppRoute) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(WithDB(r.Context(), h.cfg.DB), formKey{}, h.forms))
		ww := &written{ResponseWriter: w}
		defer func() {
			p := recover()
			if p == nil {
				return
			}
			if p == http.ErrAbortHandler {
				panic(p)
			}
			verb := "%v" // an error panic stays one for errors.Is
			if _, ok := p.(error); ok {
				verb = "%w"
			}
			err := fmt.Errorf("panic: "+verb+"\n%s", p, debug.Stack())
			h.report(ww, r, route, http.StatusInternalServerError, err)
		}()
		if err := route.Serve(ww, r); err != nil {
			h.report(ww, r, route, status(err), err)
		}
	})
}

// report answers with code, unless the route wrote already, and tells
// OnError of a 5xx.
func (h *handler) report(w *written, r *http.Request, route *AppRoute, code int, err error) {
	if code >= 500 {
		if h.cfg.OnError != nil {
			h.cfg.OnError(r.Context(), r, err)
		} else {
			log.Printf("maago: %s %s: %v", r.Method, r.URL.Path, err)
		}
	}
	if !w.wrote {
		h.fail(w, route, code, err)
	}
}

// status is the response's code for err: a Status first, then
// sql.ErrNoRows, which is a row that is not there; else 500.
func status(err error) int {
	var s *statusError
	switch {
	case errors.As(err, &s):
		return s.code
	case errors.Is(err, sql.ErrNoRows):
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

// fail writes the error response: an RFC 9457 problem for an API route
// (D-44), plain text otherwise. Only a 4xx says why: a 500's error stays
// inside, with OnError.
func (h *handler) fail(w http.ResponseWriter, route *AppRoute, code int, err error) {
	if route == nil || !route.API {
		http.Error(w, http.StatusText(code), code)
		return
	}
	p := struct {
		Type   string `json:"type"`
		Title  string `json:"title"`
		Status int    `json:"status"`
		Detail string `json:"detail,omitempty"`
	}{"about:blank", http.StatusText(code), code, ""}
	if code < 500 {
		p.Detail = err.Error()
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(p)
}

// written records whether anything was written.
type written struct {
	http.ResponseWriter
	wrote bool
}

// A 1xx header is not the response yet.
func (w *written) WriteHeader(code int) {
	w.ResponseWriter.WriteHeader(code)
	w.wrote = w.wrote || code >= 200
}

var _ http.Flusher = (*written)(nil) // streaming views flush (task 8)

func (w *written) Flush() { w.wrote = true; http.NewResponseController(w.ResponseWriter).Flush() }

func (w *written) Write(b []byte) (int, error) { w.wrote = true; return w.ResponseWriter.Write(b) }

func (w *written) Unwrap() http.ResponseWriter { return w.ResponseWriter } // for http.ResponseController

// Status makes err, which is not nil, answer with code, from 400 to 599: a view returns
// maago.Status(http.StatusForbidden, err) to refuse. The generated code
// uses it for what it cannot parse. An error wrapping it keeps the code.
func Status(code int, err error) error {
	if code < 400 || code > 599 {
		panic(fmt.Sprintf("maago: Status(%d): the code must be from 400 to 599", code))
	}
	if err == nil {
		panic("maago: Status with a nil error")
	}
	return &statusError{code, err}
}

type statusError struct {
	code int
	err  error
}

func (e *statusError) Error() string { return e.err.Error() }
func (e *statusError) Unwrap() error { return e.err }

type dbKey struct{}

// WithDB returns ctx carrying db, as Handler gives each request. A view
// tested without HTTP gets its database this way.
func WithDB(ctx context.Context, db *sql.DB) context.Context {
	return context.WithValue(ctx, dbKey{}, db)
}

// DBFrom is the database of ctx. It panics without one: a request from
// Handler has it, and a test adds it with WithDB.
func DBFrom(ctx context.Context) *sql.DB {
	db, _ := ctx.Value(dbKey{}).(*sql.DB)
	if db == nil {
		panic("maago: no database in the context; Handler puts Config.DB there, and a test uses maago.WithDB")
	}
	return db
}

// Result is what a view that may redirect returns: Show(page) to render
// the page, or SeeOther[Page](url) to send the browser elsewhere (D-116).
type Result[P any] struct {
	page P
	to   string
}

// Show renders p.
func Show[P any](p P) Result[P] { return Result[P]{page: p} }

// SeeOther redirects with 303 See Other to url. Go cannot infer P from
// the return, so it is written: maago.SeeOther[PostFormPage](url).
func SeeOther[P any](url string) Result[P] {
	if url == "" {
		panic("maago: SeeOther with an empty URL")
	}
	return Result[P]{to: url}
}

// Page is the page to render, or the URL to redirect to. Only generated
// code reads it.
func (r Result[P]) Page() (P, string) { return r.page, r.to }
