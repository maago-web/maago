package maago

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/maago-web/maago/internal/rtcore"
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
	// A 4xx is not an error to report: a bad path, a form, the cross-origin
	// check, a login. Every request that fails with a 5xx calls it, at the same time as others, so it
	// must be safe for that. To
	// send errors to a service such as Sentry, report from here:
	//
	//	OnError: func(ctx context.Context, r *http.Request, err error) {
	//		log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
	//		reporter.Capture(ctx, err) // the service's client
	//	},
	//
	// What must see every request, such as timing, wraps the Handler as
	// middleware (D-46).
	OnError func(ctx context.Context, r *http.Request, err error)

	// Messages are the errors forms show; an empty one keeps the English
	// default (D-119).
	Messages Messages

	// TimeZone is where the times people enter and see are; nil is UTC.
	TimeZone *time.Location

	// Auth tells who sent r (D-121): a user, which views take as their u
	// (and func(w, r) views read with UserFrom), or nil for nobody. It runs once for each request, before
	// the route. Its error answers as a view's does: a Status's code, else
	// a 500. A nil pointer returned as the user is a user: return nil
	// itself for nobody, also for a missing or expired session or token
	// (an sql.ErrNoRows would answer 404, a Status(401) would not send a
	// page to the login page; a route whose view takes u U answers an
	// API request without a user with 401 and WWW-Authenticate: Bearer). The
	// database is in r's context: DBFrom(r.Context()).
	Auth func(r *http.Request) (any, error)

	// Prefix is the path the site is under, such as "/app", or "" (D-122).
	// The proxy in front takes it off before passing a request on, so routes
	// stay as the table says; if it does not, main wraps the handler in
	// http.StripPrefix(cfg.Prefix, h). It goes before the paths the
	// generated URL functions, {{url}} and the login page's next make. The
	// proxy must pass on the Host the browser asked for: the cross-origin
	// check compares it with Origin. URLs given as such (LoginURL, the URL of
	// SeeOther) are left as they are. It holds for the process: the last
	// Handler made sets it, with TimeZone, for the URL and template functions.
	Prefix string

	// LoginURL is where an HTML route that requires login sends a GET or
	// HEAD without a user, with the request's path and query in next.
	LoginURL string

	// MaxBody is the most bytes the body of a request may hold, 0 for
	// 10MB. A route that reads more answers 413, whatever the body: a
	// form, JSON, or what a func(w, r) view reads, when the view returns
	// the read's error before writing (D-139).
	MaxBody int64

	// MediaDir is the directory SaveMedia keeps files in (D-140), relative
	// to the working directory if not absolute. A project whose forms take
	// files must set it, and it must exist: Handler does not make it.
	MediaDir string
}

// App is an app's routes, as maago gen generates them into the app's App
// value: board.App. contrib/admin makes the admin's (D-131).
type App = rtcore.App

// Handler serves the apps of cfg. It rejects what cfg gets wrong; the
// routes themselves maago gen has checked.
func Handler(cfg Config) (http.Handler, error) {
	if cfg.DB == nil {
		return nil, errors.New("maago: Config.DB is nil; open it with maago.OpenDB")
	}
	if len(cfg.Apps) == 0 {
		return nil, errors.New("maago: Config.Apps is empty; add each app's App, such as board.App")
	}
	h := &handler{cfg: cfg, mux: http.NewServeMux(), routes: map[string]*rtcore.Route{}, forms: rtcore.NewSettings(rtcore.Messages(cfg.Messages), rtcore.Messages(DefaultMessages), cfg.TimeZone)}
	if err := Messages(h.forms.Msgs).check(); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	static := map[string]*rtcore.StaticFiles{}
	staticOf := map[string]string{}  // the app, by the package name of its static files
	login, htmlLogin := false, false // some route requires login; some HTML one does
	files := false                   // some route reads a form with a file
	for _, app := range cfg.Apps {
		name, routes, s := rtcore.Parts(app)
		switch {
		case name == "":
			return nil, errors.New("maago: Config.Apps holds an empty App; use the one maago gen makes, such as board.App, and run maago gen")
		case seen[name]:
			return nil, fmt.Errorf("maago: app %s is in Config.Apps twice", name)
		}
		seen[name] = true
		if s != nil {
			if other, ok := staticOf[s.App()]; ok {
				return nil, fmt.Errorf("maago: apps %s and %s both serve static files at /static/%s/; rename one package", other, name, s.App())
			}
			staticOf[s.App()], static[s.App()] = name, s
			h.mux.HandleFunc("GET /static/"+s.App()+"/{name}", s.Serve)
		}
		for i := range routes {
			r := &routes[i]
			if r.Serve == nil {
				return nil, fmt.Errorf("maago: route %s of app %s serves nothing; run maago gen", r.Name, name)
			}
			pattern := r.Method + " " + r.Path
			h.routes[pattern] = r
			h.mux.Handle(pattern, h.serve(r))
			login, files = login || r.LoginRequired || r.User, files || r.Files
			htmlLogin = htmlLogin || r.LoginRequired && !r.API
		}
	}
	switch {
	case login && cfg.Auth == nil:
		return nil, errors.New("maago: routes require login or take the user, but Config.Auth is nil, so none has a user; set it to what tells the user of a request")
	case htmlLogin && cfg.LoginURL == "":
		return nil, errors.New("maago: HTML routes require login, but Config.LoginURL is empty; set it to the login page")
	}
	switch info, err := os.Stat(cfg.MediaDir); {
	case files && cfg.MediaDir == "":
		return nil, errors.New("maago: forms take files, but Config.MediaDir is empty; set it to the directory to keep them in, such as media")
	case cfg.MediaDir != "" && err != nil:
		return nil, fmt.Errorf("maago: Config.MediaDir: %w; create the directory", err)
	case cfg.MediaDir != "" && !info.IsDir():
		return nil, fmt.Errorf("maago: Config.MediaDir %q is not a directory", cfg.MediaDir)
	}
	if h.cfg.MaxBody < 0 {
		return nil, fmt.Errorf("maago: Config.MaxBody is %d; give the most bytes a body may hold, or 0 for 10MB", h.cfg.MaxBody)
	}
	if h.cfg.MaxBody == 0 {
		h.cfg.MaxBody = 10 << 20
	}
	if cfg.Prefix != "" && (!strings.HasPrefix(cfg.Prefix, "/") || slices.ContainsFunc(strings.Split(cfg.Prefix, "/")[1:], func(seg string) bool { return seg == "" || url.PathEscape(seg) != seg })) {
		return nil, fmt.Errorf("maago: Config.Prefix %q is not a path such as /app: start it with /, end it without, and use no character a URL path escapes", cfg.Prefix)
	}
	if u, err := url.Parse(cfg.LoginURL); err != nil {
		return nil, fmt.Errorf("maago: Config.LoginURL: %w", err)
	} else if cfg.LoginURL != "" && !u.IsAbs() && !strings.HasPrefix(u.Path, "/") {
		return nil, fmt.Errorf("maago: Config.LoginURL %q is relative, so it would move with each page; start it with / or give a whole URL", cfg.LoginURL)
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
	tz := cfg.TimeZone
	if tz == nil {
		tz = time.UTC
	}
	rtcore.Current.Store(&rtcore.Process{Prefix: cfg.Prefix, TZ: tz, Static: static}) // the last Handler wins (D-122)
	return h, nil
}

type handler struct {
	cfg    Config
	mux    *http.ServeMux
	routes map[string]*rtcore.Route // by pattern
	forms  *rtcore.Settings
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
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.WriteHeader(http.StatusNoContent)
			return
		}
	} else if trusted {
		allow(w, origin)
	}
	h.next.ServeHTTP(w, r)
}

// preflightOK reports whether a route serves method at r's path, and the
// request asks for no header but Content-Type and Authorization.
func (h *handler) preflightOK(r *http.Request, method string) bool {
	for _, name := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
		if name = strings.TrimSpace(name); name != "" && !strings.EqualFold(name, "Content-Type") && !strings.EqualFold(name, "Authorization") {
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
func (h *handler) route(r *http.Request) *rtcore.Route {
	_, pattern := h.mux.Handler(r)
	return h.routes[pattern]
}

// serve runs a route with the database in its context, and turns its
// error or panic into the response.
func (h *handler) serve(route *rtcore.Route) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := &written{ResponseWriter: w}
		ctx := rtcore.WithSettings(WithDB(r.Context(), h.cfg.DB), h.forms)
		ctx = context.WithValue(ctx, remoteAddrKey{}, r.RemoteAddr)
		if h.cfg.MediaDir != "" {
			ctx = WithMediaDir(ctx, h.cfg.MediaDir)
		}
		r = r.WithContext(context.WithValue(ctx, cookiesKey{}, ww))
		r.Body = http.MaxBytesReader(w, r.Body, h.cfg.MaxBody) // w, not ww: net/http closes the connection only when its own writer hears of it
		// The multipart files of r as it is last: net/http removes only
		// those of its own request, not of this copy.
		defer func() {
			if r.MultipartForm == nil {
				return
			}
			if err := r.MultipartForm.RemoveAll(); err != nil { // the request is answered; a file left on disk is still news
				if h.cfg.OnError != nil {
					h.cfg.OnError(r.Context(), r, fmt.Errorf("removing the temporary files of a form: %w", err))
				} else {
					log.Printf("maago: %s %s: removing the temporary files of a form: %v", r.Method, r.URL.Path, err)
				}
			}
		}()
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
		if h.cfg.Auth != nil {
			u, err := h.cfg.Auth(r)
			if err != nil {
				h.report(ww, r, route, rtcore.Code(err), err)
				return
			}
			if u != nil {
				r = r.WithContext(WithUser(r.Context(), u))
			}
		}
		if route.LoginRequired && r.Context().Value(userKey{}) == nil {
			if !route.API && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
				http.Redirect(ww, r, h.loginURL(r), http.StatusSeeOther)
				return
			}
			h.report(ww, r, route, http.StatusUnauthorized, errors.New("login required"))
			return
		}
		if err := route.Serve(ww, r); err != nil {
			h.report(ww, r, route, rtcore.Code(err), err)
		}
	})
}

// report answers with code, unless the route wrote already, and tells
// OnError of a 5xx.
func (h *handler) report(w *written, r *http.Request, route *rtcore.Route, code int, err error) {
	if code >= 500 {
		if h.cfg.OnError != nil {
			h.cfg.OnError(r.Context(), r, err)
		} else {
			log.Printf("maago: %s %s: %v", r.Method, r.URL.Path, err)
		}
	}
	if !w.wrote {
		w.cookies = nil // a failed view leaves no session behind
		h.fail(w, route, code, err)
	}
}

// fail writes the error response: an RFC 9457 problem for an API route
// (D-44), plain text otherwise. Only a 4xx says why: a 500's error stays
// inside, with OnError.
func (h *handler) fail(w http.ResponseWriter, route *rtcore.Route, code int, err error) {
	if route == nil || !route.API {
		http.Error(w, http.StatusText(code), code)
		return
	}
	p := struct {
		Type   string              `json:"type"`
		Title  string              `json:"title"`
		Status int                 `json:"status"`
		Detail string              `json:"detail,omitempty"`
		Errors []rtcore.FieldError `json:"errors,omitempty"` // a form's, for a 422 (D-120)
	}{"about:blank", http.StatusText(code), code, "", nil}
	if code == http.StatusUnprocessableEntity {
		p.Errors = rtcore.FieldErrors(err)
	}
	if code < 500 && p.Errors == nil {
		p.Detail = err.Error()
	}
	if code == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", "Bearer") // D-129
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(p)
}

// written records whether anything was written, and holds the cookies
// of SetCookie until the response starts.
type written struct {
	http.ResponseWriter
	wrote   bool
	cookies []*http.Cookie
}

// start marks the response written, its cookies set.
func (w *written) start() {
	if !w.wrote {
		for _, c := range w.cookies {
			http.SetCookie(w.ResponseWriter, c)
		}
		w.wrote = true
	}
}

// A 1xx header is not the response yet.
func (w *written) WriteHeader(code int) {
	if code >= 200 {
		w.start()
	}
	w.ResponseWriter.WriteHeader(code)
}

var _ http.Flusher = (*written)(nil) // streaming views flush (task 8)

func (w *written) Flush() { w.start(); http.NewResponseController(w.ResponseWriter).Flush() }

func (w *written) Write(b []byte) (int, error) { w.start(); return w.ResponseWriter.Write(b) }

func (w *written) Unwrap() http.ResponseWriter { return w.ResponseWriter } // for http.ResponseController

// Status makes err, which is not nil, answer with code, from 400 to 599: a view returns
// maago.Status(http.StatusForbidden, err) to refuse. The generated code
// uses it for what it cannot parse. An error wrapping it keeps the code.
func Status(code int, err error) error { return rtcore.Status(code, err) }

type cookiesKey struct{}

// SetCookie sets c on the response of the request ctx belongs to, when the
// response starts (D-125): a typed view signs in this way. A view that
// returns an error sets none. It panics outside a request from Handler,
// once the response has started, and on a cookie net/http would drop.
func SetCookie(ctx context.Context, c *http.Cookie) {
	w, _ := ctx.Value(cookiesKey{}).(*written)
	switch {
	case w == nil:
		panic("maago: SetCookie outside a request from Handler")
	case w.wrote:
		panic("maago: SetCookie after the response started")
	}
	if err := c.Valid(); err != nil {
		panic("maago: SetCookie: " + err.Error())
	}
	w.cookies = append(w.cookies, c)
}

type remoteAddrKey struct{}

// RemoteAddrFrom is the RemoteAddr of the request ctx belongs to, as the
// server set it: a typed view limits attempts by it (D-128). No header is
// read. Behind a proxy every request comes from the proxy, so main wraps
// the Handler in middleware that sets r.RemoteAddr from the proxy's header,
// such as X-Forwarded-For, only for a request from the proxy's own address:
// anyone else can send that header. It panics outside a request from
// Handler.
func RemoteAddrFrom(ctx context.Context) string {
	a, ok := ctx.Value(remoteAddrKey{}).(string)
	if !ok {
		panic("maago: RemoteAddrFrom outside a request from Handler")
	}
	return a
}

type dbKey struct{}

// WithDB returns ctx carrying db, as Handler gives each request: for
// testing without HTTP the code that reads DBFrom, such as Config.Auth or
// a func(w, r) view. A typed view takes the database as a parameter, so
// its test passes it (D-149).
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

// loginURL is Config.LoginURL with r's path and query as next, keeping
// the query the URL has.
func (h *handler) loginURL(r *http.Request) string {
	u, _ := url.Parse(h.cfg.LoginURL) // Handler parsed it
	q := u.Query()
	q.Set("next", h.cfg.Prefix+r.URL.RequestURI()) // the path the browser asked for
	u.RawQuery = q.Encode()
	return u.String()
}

type userKey struct{}

// WithUser returns ctx carrying user, as Handler gives each request
// Config.Auth's answer: for testing without HTTP the code that reads
// UserFrom. A typed view takes the user as a parameter (D-149).
func WithUser(ctx context.Context, user any) context.Context {
	return context.WithValue(ctx, userKey{}, user)
}

// UserFrom is the user of the request, as Config.Auth told it, and
// whether there is one (D-121). A user of another type than U is a
// mismatch between Config.Auth and the view, and panics. A typed view
// takes the user as a parameter instead, and maago gen refuses it there
// (D-149). A func(w, r) view that needs a sign-in reads it here, and
// answers nobody itself: a 401 to an API client, a redirect to the login
// page to a browser.
func UserFrom[U any](ctx context.Context) (U, bool) {
	var zero U
	v := ctx.Value(userKey{})
	if v == nil {
		return zero, false
	}
	u, ok := v.(U)
	if !ok {
		panic(fmt.Sprintf("maago: the user is a %T, not a %s; a view's user parameter (u T or u *T) and UserFrom must be the type Config.Auth returns, a value, not a pointer", v, fmt.Sprintf("%T", &zero)[1:]))
	}
	return u, true
}
