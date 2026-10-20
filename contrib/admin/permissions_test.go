package admin

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/maagotest"
)

// The admin's routes are made at run time, out of maago gen's sight: a
// permission table covers them as the App admin.App made.
func TestPermissionsTable(t *testing.T) {
	post := plain("blog", "Post")
	post.Form = func(context.Context, maago.DB, int64, url.Values) (Form, error) { return Form{}, nil }
	cfg := Config{Models: [][]Model{{post}}, Can: func(ctx context.Context, app, model, action string) (bool, error) {
		u, _ := maago.UserFrom[string](ctx)
		return u == "ann" && action == View, nil // ann may view the posts, ed nothing
	}}
	h := handler(t, cfg, "", nil)
	as := func(user string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Set("X-User", user)
			h.ServeHTTP(w, r)
		})
	}
	want := func(nobody, ed, ann int) map[string]int {
		return map[string]int{"nobody": nobody, "ed": ed, "ann": ann}
	}
	maagotest.Permissions(t, []maago.App{App(cfg)}, []maagotest.Role{{Name: "nobody", Handler: h}, {Name: "ed", Handler: as("ed")}, {Name: "ann", Handler: as("ann")}},
		maagotest.Row{Route: "admin_index", URL: "/admin/", Want: want(303, 403, 200)},
		maagotest.Row{Route: "admin_list", URL: "/admin/blog/Post/", Want: want(303, 403, 200)},
		maagotest.Row{Route: "admin_add", URL: "/admin/blog/Post/add/", Want: want(303, 403, 403)},
		maagotest.Row{Route: "admin_add_submit", URL: "/admin/blog/Post/add/", Want: want(401, 403, 403)},
		maagotest.Row{Route: "admin_row", URL: "/admin/blog/Post/1/", Want: want(303, 403, 200)},
		maagotest.Row{Route: "admin_row_submit", URL: "/admin/blog/Post/1/", Want: want(401, 403, 403)},
		maagotest.Row{Route: "admin_delete", URL: "/admin/blog/Post/delete/?id=1", Want: want(303, 403, 403)},
		maagotest.Row{Route: "admin_delete_submit", URL: "/admin/blog/Post/delete/", Body: "id=1", Want: want(401, 403, 403)},
	)
}
