package maago

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testUser struct{ Name string }

type named interface{ UserName() string }

func (u *testUser) UserName() string { return u.Name }

// fakeAuth logs in a request that says X-User: name.
func fakeAuth(r *http.Request) (any, error) {
	switch name := r.Header.Get("X-User"); name {
	case "":
		return nil, nil
	case "broken":
		return nil, errors.New("session store down")
	case "badtoken":
		return nil, Status(http.StatusUnauthorized, errors.New("token expired"))
	default:
		return &testUser{name}, nil
	}
}

// whoAmI writes the request's user, or "nobody".
func whoAmI(w http.ResponseWriter, r *http.Request) error {
	if u, ok := UserFrom[*testUser](r.Context()); ok {
		w.Write([]byte("hello " + u.Name))
	} else {
		w.Write([]byte("nobody"))
	}
	return nil
}

func TestLoginRequired(t *testing.T) {
	login := func(method, path string, api bool) AppRoute {
		r := route(method, path, api, whoAmI)
		r.LoginRequired = true
		return r
	}
	app := NewApp("a", login("GET", "/secret", false), login("POST", "/secret", false), login("GET", "/api/secret", true),
		route("GET", "/public", false, whoAmI))
	cfg := Config{Apps: []App{app}, Auth: fakeAuth, LoginURL: "/login?lang=ko", Origins: nil}
	for _, c := range []struct {
		name, method, path, user string
		header                   map[string]string
		code                     int
		body, location           string
		reported                 bool
	}{
		{"GET without a user", "GET", "/secret?page=2", "", nil, 303, "", "/login?lang=ko&next=%2Fsecret%3Fpage%3D2", false},
		{"HEAD without a user", "HEAD", "/secret", "", nil, 303, "", "/login?lang=ko&next=%2Fsecret", false},
		{"POST without a user", "POST", "/secret", "", nil, 401, "Unauthorized\n", "", false},
		{"API without a user", "GET", "/api/secret", "", nil, 401,
			`{"type":"about:blank","title":"Unauthorized","status":401,"detail":"login required"}` + "\n", "", false},
		{"logged in", "GET", "/secret", "ann", nil, 200, "hello ann", "", false},
		{"public, nobody", "GET", "/public", "", nil, 200, "nobody", "", false},
		{"public knows the user", "GET", "/public", "ann", nil, 200, "hello ann", "", false},
		{"Auth fails, even on a public page", "GET", "/public", "broken", nil, 500, "Internal Server Error\n", "", true},
		{"Auth says 401", "GET", "/api/secret", "badtoken", nil, 401,
			`{"type":"about:blank","title":"Unauthorized","status":401,"detail":"token expired"}` + "\n", "", false},
		{"cross-origin first: 403, not the login page", "POST", "/secret", "", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"},
			403, "Forbidden\n", "", false},
	} {
		r := httptest.NewRequest(c.method, c.path, nil)
		if c.user != "" {
			r.Header.Set("X-User", c.user)
		}
		for k, v := range c.header {
			r.Header.Set(k, v)
		}
		w, reported := serve(t, cfg, r)
		if w.Code != c.code || c.method != "HEAD" && c.code != 303 && w.Body.String() != c.body || w.Header().Get("Location") != c.location {
			t.Errorf("%s: %d %q Location %q; want %d %q %q", c.name, w.Code, w.Body, w.Header().Get("Location"), c.code, c.body, c.location)
		}
		if (len(reported) > 0) != c.reported {
			t.Errorf("%s: OnError heard %v", c.name, reported)
		}
		if want := map[bool]string{true: "Bearer"}[c.code == 401 && strings.Contains(c.path, "api")]; w.Header().Get("WWW-Authenticate") != want {
			t.Errorf("%s: WWW-Authenticate %q, want %q", c.name, w.Header().Get("WWW-Authenticate"), want)
		}
	}

	// A preflight is answered before the routes: Auth is not asked.
	asked := false
	pre := cfg
	pre.Origins = []string{"https://app.example"}
	pre.Auth = func(r *http.Request) (any, error) { asked = true; return nil, nil }
	r := httptest.NewRequest("OPTIONS", "/secret", nil)
	r.Header.Set("Origin", "https://app.example")
	r.Header.Set("Access-Control-Request-Method", "POST")
	if w, _ := serve(t, pre, r); w.Code != 204 || asked {
		t.Errorf("preflight: %d, Auth asked: %v", w.Code, asked)
	}
}

func TestAuthConfig(t *testing.T) {
	login := route("GET", "/secret", false, ok)
	login.LoginRequired = true
	api := route("GET", "/api", true, ok)
	api.LoginRequired = true
	for _, c := range []struct {
		cfg  Config
		want string
	}{
		{Config{DB: testDB, Apps: []App{NewApp("a", login)}, LoginURL: "/login"}, "Config.Auth is nil"},
		{Config{DB: testDB, Apps: []App{NewApp("a", login)}, Auth: fakeAuth}, "Config.LoginURL is empty"},
		{Config{DB: testDB, Apps: []App{NewApp("a", login)}, Auth: fakeAuth, LoginURL: "%zz"}, "Config.LoginURL"},
		{Config{DB: testDB, Apps: []App{NewApp("a", login)}, Auth: fakeAuth, LoginURL: "login"}, "is relative"},
	} {
		if _, err := Handler(c.cfg); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("got %v, want %q", err, c.want)
		}
	}
	// An API-only app needs no login page.
	if _, err := Handler(Config{DB: testDB, Apps: []App{NewApp("a", api)}, Auth: fakeAuth}); err != nil {
		t.Errorf("API only: %v", err)
	}
}

func TestUserFrom(t *testing.T) {
	ctx := WithUser(context.Background(), &testUser{"ann"})
	if u, ok := UserFrom[*testUser](ctx); !ok || u.Name != "ann" {
		t.Errorf("pointer: %v %v", u, ok)
	}
	if u, ok := UserFrom[named](ctx); !ok || u.UserName() != "ann" {
		t.Errorf("an interface the user has: %v %v", u, ok)
	}
	if _, ok := UserFrom[*testUser](context.Background()); ok {
		t.Error("no user: ok")
	}
	defer func() {
		if p := recover(); p == nil || !strings.Contains(p.(string), "not a maago.testUser") {
			t.Errorf("a value for a pointer: %v", p)
		}
	}()
	UserFrom[testUser](ctx)
}

// Without Auth, a public route serves nobody; Auth sees the database and
// a panic in it is a 500.
func TestAuthEdges(t *testing.T) {
	app := NewApp("a", route("GET", "/public", false, whoAmI))
	if w, _ := serve(t, Config{Apps: []App{app}}, httptest.NewRequest("GET", "/public", nil)); w.Body.String() != "nobody" {
		t.Errorf("no Auth: %q", w.Body)
	}
	var db bool
	cfg := Config{Apps: []App{app}, Auth: func(r *http.Request) (any, error) { db = DBFrom(r.Context()) == testDB; return nil, nil }}
	if serve(t, cfg, httptest.NewRequest("GET", "/public", nil)); !db {
		t.Error("Auth does not see the database")
	}
	cfg.Auth = func(r *http.Request) (any, error) { panic("auth broke") }
	if w, reported := serve(t, cfg, httptest.NewRequest("GET", "/public", nil)); w.Code != 500 || len(reported) != 1 || !strings.HasPrefix(reported[0].Error(), "panic: auth broke") {
		t.Errorf("panic in Auth: %d %v", w.Code, reported)
	}
}
