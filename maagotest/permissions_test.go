package maagotest

import (
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/internal/rtcore"
	"github.com/maago-web/maago/rt"
)

// errorsTB records what Errorf says.
type errorsTB struct {
	testing.TB
	errs []string
}

func (e *errorsTB) Helper() {}
func (e *errorsTB) Errorf(format string, args ...any) {
	e.errs = append(e.errs, fmt.Sprintf(format, args...))
}

// permSite is a site of three routes: a page for anyone, a delete for a
// user only, an API that echoes what it was sent. sent records each
// request as "role method path content-type body".
func permSite(sent *[]string) ([]maago.App, []Role) {
	serve := func(w http.ResponseWriter, r *http.Request) error { return nil }
	app := rt.NewApp("x", nil,
		rt.AppRoute{Method: "GET", Path: "/{$}", Name: "home", Serve: serve},
		rt.AppRoute{Method: "POST", Path: "/items/{id}/delete/{$}", Name: "item_delete", Serve: serve},
		rt.AppRoute{Method: "POST", Path: "/api/items", Name: "api_item_create", API: true, Serve: serve})
	h := http.NewServeMux()
	record := func(r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		who := "nobody"
		if c, err := r.Cookie("s"); err == nil {
			who = c.Value
		}
		*sent = append(*sent, strings.Join([]string{who, r.Method, r.URL.Path, r.Header.Get("Content-Type"), string(body)}, " "))
	}
	h.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { record(r) })
	h.HandleFunc("POST /items/{id}/delete/{$}", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		if _, err := r.Cookie("s"); err != nil {
			w.WriteHeader(401)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	h.HandleFunc("POST /api/items", func(w http.ResponseWriter, r *http.Request) { record(r); w.WriteHeader(201) })
	return []maago.App{app}, []Role{{Name: "nobody", Handler: h}, {Name: "ann", Handler: As(h, &http.Cookie{Name: "s", Value: "ann"})}}
}

// rows is a right table for permSite.
func rows() []Row {
	return []Row{
		{Route: "home", URL: "/", Want: map[string]int{"nobody": 200, "ann": 200}},
		{Route: "item_delete", URL: "/items/1/delete/", Want: map[string]int{"nobody": 401, "ann": 303}},
		{Route: "api_item_create", URL: "/api/items", Body: `{"name":"a"}`, Want: map[string]int{"nobody": 201, "ann": Skip}},
		{Route: "api_item_create", URL: "/api/items", Body: "x", ContentType: "text/plain", Want: map[string]int{"nobody": Skip, "ann": 201}},
	}
}

func permissions(change func(apps *[]maago.App, roles *[]Role, rows *[]Row)) (errs, sent []string) {
	apps, roles := permSite(&sent)
	rs := rows()
	if change != nil {
		change(&apps, &roles, &rs)
	}
	e := &errorsTB{}
	Permissions(e, apps, roles, rs...)
	return e.errs, sent
}

func TestPermissions(t *testing.T) {
	errs, sent := permissions(nil)
	if len(errs) != 0 {
		t.Errorf("a right table: %q", errs)
	}
	want := []string{
		"nobody GET /  ",
		"ann GET /  ",
		"nobody POST /items/1/delete/ application/x-www-form-urlencoded ",
		"ann POST /items/1/delete/ application/x-www-form-urlencoded ",
		`nobody POST /api/items application/json {"name":"a"}`,
		"ann POST /api/items text/plain x",
	}
	if !slices.Equal(sent, want) {
		t.Errorf("sent, in order, Skip not sent:\n%q\nwant\n%q", sent, want)
	}
}

func TestPermissionsFails(t *testing.T) {
	for _, c := range []struct {
		name   string
		change func(*[]maago.App, *[]Role, *[]Row)
		want   []string
	}{
		{"a route without a row", func(_ *[]maago.App, _ *[]Role, rs *[]Row) { *rs = (*rs)[:2] },
			[]string{"maagotest.Permissions: route api_item_create (POST /api/items) has no row; add one with each role's status"}},
		{"a role missing from a row, though another row has it", func(_ *[]maago.App, _ *[]Role, rs *[]Row) { delete((*rs)[1].Want, "nobody") },
			[]string{"item_delete POST /items/1/delete/: no status for nobody; give it one, or Skip"}},
		{"a wrong status, with where it redirects", func(_ *[]maago.App, _ *[]Role, rs *[]Row) { (*rs)[1].Want["ann"] = 403 },
			[]string{"item_delete POST /items/1/delete/ as ann: 303 to /, want 403"}},
		{"a URL of another route", func(_ *[]maago.App, _ *[]Role, rs *[]Row) {
			(*rs)[0] = Row{Route: "home", URL: "/api/items", Want: map[string]int{"nobody": 200, "ann": 200}}
		},
			[]string{"home GET /api/items: the URL reaches no route (a 404 or a redirect), not home"}},
		{"a URL of no route", func(_ *[]maago.App, _ *[]Role, rs *[]Row) { (*rs)[2].URL = "/api/item" },
			[]string{"api_item_create POST /api/item: the URL reaches no route (a 404 or a redirect), not api_item_create"}},
		{"a URL that redirects: no final /", func(_ *[]maago.App, _ *[]Role, rs *[]Row) { (*rs)[1].URL = "/items/1/delete" },
			[]string{"item_delete POST /items/1/delete: the URL reaches no route (a 404 or a redirect), not item_delete"}},
		{"an unknown route and role", func(_ *[]maago.App, _ *[]Role, rs *[]Row) {
			*rs = append(*rs, Row{Route: "nope", URL: "/"})
			(*rs)[0].Want["bob"] = 200
		}, []string{"home GET /: no role named bob", "maagotest.Permissions: no route named nope in apps"}},
		{"a row of Skips", func(_ *[]maago.App, _ *[]Role, rs *[]Row) {
			(*rs)[2].Want = map[string]int{"nobody": Skip, "ann": Skip}
		},
			[]string{"api_item_create POST /api/items: every role is Skip; a row sends at least one"}},
		{"no apps", func(apps *[]maago.App, _ *[]Role, rs *[]Row) { *apps, *rs = nil, nil },
			[]string{"maagotest.Permissions: apps have no routes; give the App values the handler serves, such as board.App"}},
		{"no roles", func(_ *[]maago.App, roles *[]Role, _ *[]Row) { *roles = nil },
			[]string{"maagotest.Permissions: no roles; give one for nobody and one for each kind of user"}},
		{"a role twice", func(_ *[]maago.App, roles *[]Role, _ *[]Row) { *roles = append(*roles, (*roles)[1]) },
			[]string{"maagotest.Permissions: role ann given twice"}},
		{"a route name in two apps", func(apps *[]maago.App, _ *[]Role, _ *[]Row) {
			*apps = append(*apps, rt.NewApp("y", nil, rt.AppRoute{Method: "GET", Path: "/y", Name: "home", Serve: func(http.ResponseWriter, *http.Request) error { return nil }}))
		}, []string{"maagotest.Permissions: two routes are named home; a row could not tell them apart"}},
		{"a route path in two apps", func(apps *[]maago.App, _ *[]Role, _ *[]Row) {
			*apps = append(*apps, rt.NewApp("y", nil, rt.AppRoute{Method: "GET", Path: "/{$}", Name: "y_home", Serve: func(http.ResponseWriter, *http.Request) error { return nil }}))
		}, []string{"maagotest.Permissions: routes home and y_home are both GET /{$}"}},
	} {
		if errs, _ := permissions(c.change); !slices.Equal(errs, c.want) {
			t.Errorf("%s:\n%q\nwant\n%q", c.name, errs, c.want)
		}
	}
}

// A URL function puts Config.Prefix first; the routes and the mux that
// matches a row do not have it.
func TestPermissionsPrefix(t *testing.T) {
	defer func(p *rtcore.Process) { rtcore.Current.Store(p) }(rtcore.Current.Load())
	rtcore.Current.Store(&rtcore.Process{Prefix: "/app"})
	var sent []string
	apps, roles := permSite(&sent)
	prefixed := func(h http.Handler) http.Handler { return http.StripPrefix("/app", h) }
	for i := range roles {
		roles[i].Handler = prefixed(roles[i].Handler)
	}
	rs := rows()
	for i := range rs {
		rs[i].URL = "/app" + rs[i].URL
	}
	e := &errorsTB{}
	Permissions(e, apps, roles, rs...)
	if len(e.errs) != 0 || len(sent) != 6 {
		t.Errorf("%q, sent %q", e.errs, sent)
	}
	// Only a whole segment is the prefix: /apple is not /app's.
	if got := unprefixed("/apple/items"); got != "/apple/items" {
		t.Errorf("/apple/items: %s", got)
	}
	for u, want := range map[string]string{"/app": "/", "/app/": "/", "/app?x=1": "/?x=1", "/app/items/1/delete/": "/items/1/delete/"} {
		if got := unprefixed(u); got != want {
			t.Errorf("%s: %s, want %s", u, got, want)
		}
	}
}
