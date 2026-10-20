package maagotest

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/internal/rtcore"
)

// Role is who sends a row's request: a name for the table, and the
// handler that sends it as them. Nobody is the site's handler itself;
// a signed-in user is the handler with their session, from As.
type Role struct {
	Name    string
	Handler http.Handler
}

// Row is one request of a permission table and the status each role
// gets. Route is the route's name, URL what the route's URL function
// makes, Config.Prefix first, with a query if any; the method is the
// route's. Body is sent as it is, with the Content-Type
// application/json for an API route and
// application/x-www-form-urlencoded (url.Values.Encode()) for any
// other, unless ContentType says what it is, as a route that takes a
// file needs: a body from mime/multipart and the writer's
// FormDataContentType().
//
// Want has every role: Skip for one the row does not send.
type Row struct {
	Route, URL, Body, ContentType string
	Want                          map[string]int
}

// Skip in a Row's Want sends that role nothing: a row that changes
// what it asks for, such as a delete one role may do, has a fixture of
// its own for each role that succeeds.
const Skip = -1

// As is h for a browser with the cookie c, such as a session the
// project's auth.SignIn started: every request carries it.
func As(h http.Handler, c *http.Cookie) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(c)
		h.ServeHTTP(w, r)
	})
}

// Permissions sends each row's request as each role and fails t where
// the status is not the one wanted, then fails t for every route of
// apps that no row asks for: a route added without its row is a
// failing test. A route of an app not in apps is not seen: pass each
// app of Config.Apps whose routes the table is for, the admin's too,
// board.App and the App admin.App made (a copied app such as auth has
// a table of its own). Rows run in order, and in a row the roles in the order
// of roles, so a row that writes goes after those that read what it
// changes, and the roles it refuses go first. Each row's URL must be
// its route's: one that is another route, no route, or a redirect, such
// as a path without its final /, fails.
func Permissions(t testing.TB, apps []maago.App, roles []Role, rows ...Row) {
	t.Helper()
	if len(roles) == 0 {
		t.Errorf("maagotest.Permissions: no roles; give one for nobody and one for each kind of user")
		return
	}
	names := map[string]bool{}
	for _, r := range roles {
		if names[r.Name] {
			t.Errorf("maagotest.Permissions: role %s given twice", r.Name)
		}
		names[r.Name] = true
	}
	// The routes by name, and a mux that tells which one a request reaches.
	routes := map[string]rtcore.Route{}
	patterns := map[string]string{} // the route's name, by method and path
	var order []string
	mux := http.NewServeMux()
	var hit string
	for _, app := range apps {
		_, rs, _ := rtcore.Parts(app)
		for _, r := range rs {
			if _, ok := routes[r.Name]; ok {
				t.Errorf("maagotest.Permissions: two routes are named %s; a row could not tell them apart", r.Name)
				continue
			}
			pattern := r.Method + " " + r.Path
			if other, ok := patterns[pattern]; ok {
				t.Errorf("maagotest.Permissions: routes %s and %s are both %s", other, r.Name, pattern)
				continue
			}
			patterns[pattern] = r.Name
			routes[r.Name] = r
			order = append(order, r.Name)
			mux.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) { hit = r.Name })
		}
	}
	asked := map[string]bool{}
	for _, row := range rows {
		r, ok := routes[row.Route]
		if !ok {
			t.Errorf("maagotest.Permissions: no route named %s in apps", row.Route)
			continue
		}
		asked[row.Route] = true
		where := row.Route + " " + r.Method + " " + row.URL
		hit = ""
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(r.Method, unprefixed(row.URL), nil))
		if hit != row.Route {
			reaches := "no route (a 404 or a redirect)"
			if hit != "" {
				reaches = "route " + hit
			}
			t.Errorf("%s: the URL reaches %s, not %s", where, reaches, row.Route)
			continue
		}
		for _, name := range slices.Sorted(maps.Keys(row.Want)) {
			if !names[name] {
				t.Errorf("%s: no role named %s", where, name)
			}
		}
		sent := 0
		for _, role := range roles {
			want, ok := row.Want[role.Name]
			switch {
			case !ok:
				t.Errorf("%s: no status for %s; give it one, or Skip", where, role.Name)
				continue
			case want == Skip:
				continue
			}
			sent++
			req := httptest.NewRequest(r.Method, row.URL, strings.NewReader(row.Body))
			if ct := contentType(r, row); ct != "" {
				req.Header.Set("Content-Type", ct)
			}
			w := httptest.NewRecorder()
			role.Handler.ServeHTTP(w, req)
			if w.Code != want {
				t.Errorf("%s as %s: %d%s, want %d", where, role.Name, w.Code, location(w), want)
			}
		}
		if sent == 0 && len(row.Want) > 0 {
			t.Errorf("%s: every role is Skip; a row sends at least one", where)
		}
	}
	for _, name := range order {
		if !asked[name] {
			r := routes[name]
			t.Errorf("maagotest.Permissions: route %s (%s %s) has no row; add one with each role's status", name, r.Method, r.Path)
		}
	}
}

// unprefixed is u without Config.Prefix, which a URL function puts
// first: the path the routes have. A u not under it is left as it is.
func unprefixed(u string) string {
	p := maago.Prefix()
	rest, ok := strings.CutPrefix(u, p)
	switch {
	case !ok || p == "":
		return u
	case rest == "" || rest[0] == '?':
		return "/" + rest
	case rest[0] == '/':
		return rest
	}
	return u
}

// contentType is the row's, or the one its route reads.
func contentType(r rtcore.Route, row Row) string {
	switch {
	case row.ContentType != "":
		return row.ContentType
	case slices.Contains([]string{"GET", "HEAD", "DELETE"}, r.Method) && row.Body == "":
		return ""
	case r.API:
		return "application/json"
	}
	return "application/x-www-form-urlencoded"
}

// location is " to <Location>" for a redirect: a 303 to the login page
// and one after a write are the same number.
func location(w *httptest.ResponseRecorder) string {
	if l := w.Header().Get("Location"); l != "" {
		return " to " + l
	}
	return ""
}
