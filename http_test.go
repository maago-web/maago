package maago

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/textproto"
	"strings"
	"testing"
)

// noDriver is a connector that is never connected: Handler only carries
// the *sql.DB to the views.
type noDriver struct{}

func (noDriver) Connect(context.Context) (driver.Conn, error) { return nil, errors.New("no driver") }
func (noDriver) Driver() driver.Driver                        { return nil }

var testDB = sql.OpenDB(noDriver{})

// serve answers one request on a handler of apps, and returns the
// response and what OnError heard.
func serve(t *testing.T, cfg Config, r *http.Request) (*httptest.ResponseRecorder, []error) {
	t.Helper()
	var reported []error
	cfg.DB = testDB
	cfg.OnError = func(_ context.Context, _ *http.Request, err error) { reported = append(reported, err) }
	h, err := Handler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w, reported
}

func route(method, path string, api bool, serve func(w http.ResponseWriter, r *http.Request) error) AppRoute {
	return AppRoute{Method: method, Path: path, Name: strings.Trim(path, "/{}"), API: api, Serve: serve}
}

func ok(w http.ResponseWriter, r *http.Request) error { fmt.Fprint(w, "ok"); return nil }

func TestHandlerRoutes(t *testing.T) {
	var got *sql.DB
	apps := []App{
		NewApp("board", route("GET", "/boards", false, func(w http.ResponseWriter, r *http.Request) error {
			got = DBFrom(r.Context())
			return ok(w, r)
		})),
		NewApp("blog", route("POST", "/posts", false, ok)),
	}
	if w, _ := serve(t, Config{Apps: apps}, httptest.NewRequest("GET", "/boards", nil)); w.Body.String() != "ok" || got != testDB {
		t.Errorf("GET /boards: %d %q, db %v", w.Code, w.Body, got)
	}
	if w, _ := serve(t, Config{Apps: apps}, httptest.NewRequest("POST", "/posts", nil)); w.Body.String() != "ok" {
		t.Errorf("POST /posts: %d %q", w.Code, w.Body)
	}
}

func TestHandlerConfig(t *testing.T) {
	app := NewApp("board", route("GET", "/x", false, ok))
	for _, c := range []struct {
		cfg  Config
		want string
	}{
		{Config{Apps: []App{app}}, "maago: Config.DB is nil; open it with maago.OpenDB"},
		{Config{DB: testDB}, "maago: Config.Apps is empty; add each app's App, such as board.App"},
		{Config{DB: testDB, Apps: []App{app, app}}, "maago: app board is in Config.Apps twice"},
		{Config{DB: testDB, Apps: []App{{}}}, "maago: Config.Apps holds an empty App; use the one maago gen makes, such as board.App, and run maago gen"},
		{Config{DB: testDB, Apps: []App{NewApp("a", AppRoute{Method: "GET", Path: "/y", Name: "y"})}}, "maago: route y of app a serves nothing; run maago gen"},
		{Config{DB: testDB, Apps: []App{app}, Origins: []string{"https://a.com/"}}, "maago: Config.Origins: "},
	} {
		_, err := Handler(c.cfg)
		if err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("got %v, want %q", err, c.want)
		}
	}
}

func TestDBFrom(t *testing.T) {
	if DBFrom(WithDB(context.Background(), testDB)) != testDB {
		t.Error("WithDB")
	}
	defer func() {
		if p := recover(); p != "maago: no database in the context; Handler puts Config.DB there, and a test uses maago.WithDB" {
			t.Errorf("panic %v", p)
		}
	}()
	DBFrom(context.Background())
}

func TestErrorStatus(t *testing.T) {
	cases := []struct {
		name     string
		api      bool
		serve    func(w http.ResponseWriter, r *http.Request) error
		code     int
		body     string
		reported string // the start of what OnError hears
	}{
		{"no rows", false, func(http.ResponseWriter, *http.Request) error { return fmt.Errorf("get post: %w", sql.ErrNoRows) }, 404, "Not Found\n", ""},
		{"status", false, func(http.ResponseWriter, *http.Request) error { return Status(400, errors.New("bad page")) }, 400, "Bad Request\n", ""},
		{"status wrapped", false, func(http.ResponseWriter, *http.Request) error {
			return fmt.Errorf("x: %w", Status(403, errors.New("no")))
		}, 403, "Forbidden\n", ""},
		{"status before no rows", false, func(http.ResponseWriter, *http.Request) error { return Status(409, sql.ErrNoRows) }, 409, "Conflict\n", ""},
		{"other", false, func(http.ResponseWriter, *http.Request) error { return errors.New("secret detail") }, 500, "Internal Server Error\n", "secret detail"},
		{"status 500", false, func(http.ResponseWriter, *http.Request) error { return Status(500, errors.New("down")) }, 500, "Internal Server Error\n", "down"},
		{"api 4xx", true, func(http.ResponseWriter, *http.Request) error { return Status(400, errors.New("bad page")) }, 400,
			`{"type":"about:blank","title":"Bad Request","status":400,"detail":"bad page"}` + "\n", ""},
		{"api 500", true, func(http.ResponseWriter, *http.Request) error { return errors.New("secret detail") }, 500,
			`{"type":"about:blank","title":"Internal Server Error","status":500}` + "\n", "secret detail"},
		{"api 422 of a form", true, func(http.ResponseWriter, *http.Request) error {
			return Unprocessable([]FieldError{{"title", ""}, {"count", "Enter a number."}, {"days", "Enter 30 or less."}}, []string{"overlaps"})
		}, 422, `{"type":"about:blank","title":"Unprocessable Entity","status":422,"errors":[{"field":"count","detail":"Enter a number."},{"field":"days","detail":"Enter 30 or less."},{"detail":"overlaps"}]}` + "\n", ""},
		{"api 422 joined", true, func(http.ResponseWriter, *http.Request) error {
			return fmt.Errorf("save: %w", errors.Join(Invalid(&Field[string]{Name: "title"}, "taken"), InvalidForm("closed")))
		}, 422, `{"type":"about:blank","title":"Unprocessable Entity","status":422,"errors":[{"field":"title","detail":"taken"},{"detail":"closed"}]}` + "\n", ""},
		{"api 422 without a form", true, func(http.ResponseWriter, *http.Request) error { return Status(422, errors.New("cannot")) }, 422,
			`{"type":"about:blank","title":"Unprocessable Entity","status":422,"detail":"cannot"}` + "\n", ""},
		{"html 422", false, func(http.ResponseWriter, *http.Request) error { return InvalidForm("closed") }, 422, "Unprocessable Entity\n", ""},
		{"panic", false, func(http.ResponseWriter, *http.Request) error { panic("boom") }, 500, "Internal Server Error\n", "panic: boom\n"},
		{"panic with an error", false, func(http.ResponseWriter, *http.Request) error { panic(sql.ErrConnDone) }, 500, "Internal Server Error\n", "panic: sql: connection is already closed\n"},
		{"status 503", false, func(http.ResponseWriter, *http.Request) error { return Status(503, errors.New("db down")) }, 503, "Service Unavailable\n", "db down"},
		{"error after a header", false, func(w http.ResponseWriter, _ *http.Request) error { w.WriteHeader(204); return errors.New("late") }, 204, "", "late"},
		// The recorder keeps the 1xx code; the body shows the 500 was written.
		{"error after a 1xx", false, func(w http.ResponseWriter, _ *http.Request) error { w.WriteHeader(103); return errors.New("early") }, 103, "Internal Server Error\n", "early"},
		{"error after writing", false, func(w http.ResponseWriter, _ *http.Request) error {
			w.WriteHeader(202)
			fmt.Fprint(w, "partial")
			return errors.New("late")
		}, 202, "partial", "late"},
		{"panic after writing", false, func(w http.ResponseWriter, _ *http.Request) error {
			fmt.Fprint(w, "partial")
			panic("late")
		}, 200, "partial", "panic: late\n"},
	}
	for _, c := range cases {
		app := NewApp("a", route("GET", "/x", c.api, c.serve))
		w, reported := serve(t, Config{Apps: []App{app}}, httptest.NewRequest("GET", "/x", nil))
		if w.Code != c.code || w.Body.String() != c.body {
			t.Errorf("%s: got %d %q, want %d %q", c.name, w.Code, w.Body, c.code, c.body)
		}
		if ct := w.Header().Get("Content-Type"); c.api && ct != "application/problem+json" {
			t.Errorf("%s: Content-Type %q", c.name, ct)
		}
		switch {
		case c.reported == "" && len(reported) > 0:
			t.Errorf("%s: OnError heard %v", c.name, reported)
		case c.reported != "" && (len(reported) != 1 || !strings.HasPrefix(reported[0].Error(), c.reported)):
			t.Errorf("%s: OnError heard %v, want one starting %q", c.name, reported, c.reported)
		}
	}
}

func TestPanicIsAnError(t *testing.T) {
	app := NewApp("a", route("GET", "/x", false, func(http.ResponseWriter, *http.Request) error { panic(sql.ErrConnDone) }))
	_, reported := serve(t, Config{Apps: []App{app}}, httptest.NewRequest("GET", "/x", nil))
	if len(reported) != 1 || !errors.Is(reported[0], sql.ErrConnDone) {
		t.Errorf("OnError heard %v", reported)
	}
}

func TestAbortHandler(t *testing.T) {
	app := NewApp("a", route("GET", "/x", false, func(http.ResponseWriter, *http.Request) error { panic(http.ErrAbortHandler) }))
	defer func() {
		if p := recover(); p != http.ErrAbortHandler {
			t.Errorf("panic %v", p)
		}
	}()
	serve(t, Config{Apps: []App{app}}, httptest.NewRequest("GET", "/x", nil))
	t.Error("no panic")
}

func TestOnErrorNilLogs(t *testing.T) {
	var log strings.Builder
	logOutput(t, &log)
	h, err := Handler(Config{DB: testDB, Apps: []App{NewApp("a", route("GET", "/x", false, func(http.ResponseWriter, *http.Request) error { return errors.New("down") }))}})
	if err != nil {
		t.Fatal(err)
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
	if !strings.Contains(log.String(), "maago: GET /x: down") {
		t.Errorf("log %q", log.String())
	}
}

func TestCrossOrigin(t *testing.T) {
	apps := []App{NewApp("a",
		route("POST", "/form", false, ok),
		route("POST", "/api/posts", true, ok),
		route("GET", "/api/posts", true, ok),
		route("DELETE", "/api/posts", true, ok),
	)}
	const trusted = "https://app.example.com"
	cfg := Config{Apps: apps, Origins: []string{trusted}}
	req := func(method, path string, header ...string) *http.Request {
		r := httptest.NewRequest(method, "http://site.example.com"+path, nil)
		for i := 0; i+1 < len(header); i += 2 {
			r.Header.Set(header[i], header[i+1])
		}
		return r
	}
	cases := []struct {
		name   string
		cfg    Config
		r      *http.Request
		code   int
		allow  string // Access-Control-Allow-Origin
		vary   string
		header string // the start of Content-Type
	}{
		{"cross-site POST", cfg, req("POST", "/form", "Sec-Fetch-Site", "cross-site"), 403, "", "", "text/plain"},
		{"cross-site POST to an API", cfg, req("POST", "/api/posts", "Sec-Fetch-Site", "cross-site"), 403, "", "", "application/problem+json"},
		{"other Origin, no Sec-Fetch-Site", cfg, req("POST", "/form", "Origin", "https://evil.example.com"), 403, "", "Origin", ""},
		{"trusted Origin", cfg, req("POST", "/form", "Origin", trusted, "Sec-Fetch-Site", "cross-site"), 200, trusted, "Origin", ""},
		{"trusted host, other port", cfg, req("POST", "/form", "Origin", trusted+":8443", "Sec-Fetch-Site", "cross-site"), 403, "", "Origin", ""},
		{"no headers", cfg, req("POST", "/form"), 200, "", "", ""},
		{"same origin", Config{Apps: apps}, req("POST", "/form", "Sec-Fetch-Site", "same-origin"), 200, "", "", ""},
		{"same host in Origin", Config{Apps: apps}, req("POST", "/form", "Origin", "http://site.example.com"), 200, "", "Origin", ""},
		{"trusted GET", cfg, req("GET", "/api/posts", "Origin", trusted), 200, trusted, "Origin", ""},
		{"untrusted GET", cfg, req("GET", "/api/posts", "Origin", "https://evil.example.com"), 200, "", "Origin", ""},
		{"preflight", cfg, req("OPTIONS", "/api/posts", "Origin", trusted, "Access-Control-Request-Method", "POST", "Access-Control-Request-Headers", "content-type"),
			204, trusted, "Origin, Access-Control-Request-Method, Access-Control-Request-Headers", ""},
		{"preflight with a token", cfg, req("OPTIONS", "/api/posts", "Origin", trusted, "Access-Control-Request-Method", "DELETE", "Access-Control-Request-Headers", "authorization,content-type"),
			204, trusted, "Origin, Access-Control-Request-Method, Access-Control-Request-Headers", ""},
		{"preflight without request headers", cfg, req("OPTIONS", "/api/posts", "Origin", trusted, "Access-Control-Request-Method", "DELETE"),
			204, trusted, "Origin, Access-Control-Request-Method, Access-Control-Request-Headers", ""},
		{"cross-site POST to no route", cfg, req("POST", "/nothing", "Sec-Fetch-Site", "cross-site"), 403, "", "", "text/plain"},
		{"a POST is no preflight", cfg, req("POST", "/form", "Origin", trusted, "Access-Control-Request-Method", "POST"), 200, trusted, "Origin", ""},
		{"preflight, untrusted", cfg, req("OPTIONS", "/api/posts", "Origin", "https://evil.example.com", "Access-Control-Request-Method", "POST"),
			405, "", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers", ""},
		{"preflight, method of no route", cfg, req("OPTIONS", "/form", "Origin", trusted, "Access-Control-Request-Method", "DELETE"),
			405, "", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers", ""},
		{"preflight, header not allowed", cfg, req("OPTIONS", "/api/posts", "Origin", trusted, "Access-Control-Request-Method", "POST", "Access-Control-Request-Headers", "X-Secret"),
			405, "", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers", ""},
	}
	for _, c := range cases {
		w, _ := serve(t, c.cfg, c.r)
		h := w.Header()
		vary := strings.Join(h.Values("Vary"), ", ")
		if w.Code != c.code || h.Get("Access-Control-Allow-Origin") != c.allow || vary != c.vary || !strings.HasPrefix(h.Get("Content-Type"), c.header) {
			t.Errorf("%s: got %d, Allow-Origin %q, Vary %q, Content-Type %q", c.name, w.Code, h.Get("Access-Control-Allow-Origin"), vary, h.Get("Content-Type"))
		}
		if c.allow != "" && h.Get("Access-Control-Allow-Credentials") != "true" {
			t.Errorf("%s: no Allow-Credentials", c.name)
		}
		if c.code == 204 && (h.Get("Access-Control-Allow-Methods") != c.r.Header.Get("Access-Control-Request-Method") || h.Get("Access-Control-Allow-Headers") != "Content-Type, Authorization") {
			t.Errorf("%s: Allow-Methods %q, Allow-Headers %q", c.name, h.Get("Access-Control-Allow-Methods"), h.Get("Access-Control-Allow-Headers"))
		}
	}
}

// A cookie of SetCookie goes with the response the view writes, and not
// with the error a view returns or panics with (D-125).
func TestSetCookie(t *testing.T) {
	set := func(w http.ResponseWriter, r *http.Request) {
		SetCookie(r.Context(), &http.Cookie{Name: "session", Value: "s1"})
	}
	apps := []App{NewApp("a",
		route("GET", "/written", false, func(w http.ResponseWriter, r *http.Request) error { set(w, r); return ok(w, r) }),
		route("GET", "/early-hints", false, func(w http.ResponseWriter, r *http.Request) error {
			set(w, r)
			w.WriteHeader(http.StatusEarlyHints)
			w.WriteHeader(http.StatusCreated)
			return nil
		}),
		route("GET", "/error", false, func(w http.ResponseWriter, r *http.Request) error {
			set(w, r)
			return Status(http.StatusUnprocessableEntity, errors.New("bad form"))
		}),
		route("GET", "/panic", false, func(w http.ResponseWriter, r *http.Request) error { set(w, r); panic("boom") }),
		route("GET", "/flushed", false, func(w http.ResponseWriter, r *http.Request) error {
			set(w, r)
			http.NewResponseController(w).Flush()
			return errors.New("too late to say")
		}),
		route("GET", "/late", false, func(w http.ResponseWriter, r *http.Request) error { ok(w, r); set(w, r); return nil }),
		route("GET", "/invalid", false, func(w http.ResponseWriter, r *http.Request) error {
			SetCookie(r.Context(), &http.Cookie{Name: "bad name", Value: "x"})
			return ok(w, r)
		}),
	)}
	for _, c := range []struct {
		path   string
		code   int
		cookie string
		panics bool
	}{
		{"/written", 200, "session=s1", false},
		{"/error", 422, "", false},
		{"/panic", 500, "", true},
		{"/flushed", 200, "session=s1", false},
		{"/late", 200, "", true}, // the panic comes after the body: OnError hears it
		{"/invalid", 500, "", true},
	} {
		w, reported := serve(t, Config{Apps: apps}, httptest.NewRequest("GET", c.path, nil))
		panicked := len(reported) > 0 && strings.HasPrefix(reported[0].Error(), "panic: ")
		if w.Code != c.code || w.Header().Get("Set-Cookie") != c.cookie || panicked != c.panics {
			t.Errorf("%s: %d Set-Cookie %q panic %v, want %d %q %v", c.path, w.Code, w.Header().Get("Set-Cookie"), panicked, c.code, c.cookie, c.panics)
		}
	}
	// An early hint goes before the response, without its cookies.
	cfg := Config{Apps: apps, DB: testDB}
	h, err := Handler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	var hint http.Header
	trace := &httptrace.ClientTrace{Got1xxResponse: func(code int, h textproto.MIMEHeader) error { hint = http.Header(h); return nil }}
	r, _ := http.NewRequestWithContext(httptrace.WithClientTrace(context.Background(), trace), "GET", srv.URL+"/early-hints", nil)
	resp, err := srv.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if hint == nil || hint.Get("Set-Cookie") != "" || resp.StatusCode != 201 || resp.Header.Get("Set-Cookie") != "session=s1" {
		t.Errorf("early hints: 103 %v, then %d %v", hint, resp.StatusCode, resp.Header)
	}

	defer func() {
		if recover() == nil {
			t.Error("no panic outside Handler")
		}
	}()
	SetCookie(context.Background(), &http.Cookie{Name: "session", Value: "s1"})
}

func TestResult(t *testing.T) {
	type page struct{ N int }
	if p, to := Show(page{1}).Page(); p.N != 1 || to != "" {
		t.Errorf("Show: %v %q", p, to)
	}
	if _, to := SeeOther[page]("/x").Page(); to != "/x" {
		t.Errorf("SeeOther: %q", to)
	}
	for _, f := range []func(){func() { SeeOther[page]("") }, func() { Status(399, errors.New("x")) }, func() { Status(600, errors.New("x")) }, func() { Status(404, nil) }} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("no panic")
				}
			}()
			f()
		}()
	}
}

// logOutput sends the standard logger to w for the test.
func logOutput(t *testing.T, w io.Writer) {
	prev, flags := log.Writer(), log.Flags()
	log.SetOutput(w)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(prev); log.SetFlags(flags) })
}
