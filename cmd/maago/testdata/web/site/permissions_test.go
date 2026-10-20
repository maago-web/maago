package site

import (
	"net/http"
	"testing"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/maagotest"
)

// Who gets each route: nobody, or ann, as the X-User header says (Auth).
// Secret and Mine take an Account, so they need one; Greet takes a
// pointer, so anybody gets it.
func TestPermissions(t *testing.T) {
	h, db, _ := handler(t)
	p, err := InsertPost(t.Context(), db, NewPostInput{Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	ann := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("X-User", "ann")
		h.ServeHTTP(w, r)
	})
	both := func(nobody, ann int) map[string]int { return map[string]int{"nobody": nobody, "ann": ann} }
	maagotest.Permissions(t, []maago.App{App}, []maagotest.Role{{Name: "nobody", Handler: h}, {Name: "ann", Handler: ann}},
		maagotest.Row{Route: "post_detail", URL: PostDetailURL(p.ID), Want: both(200, 200)},
		maagotest.Row{Route: "page", URL: PageURL(1), Want: both(200, 200)},
		maagotest.Row{Route: "post_list", URL: PostListURL() + "?page=1", Want: both(200, 200)},
		maagotest.Row{Route: "post_rows", URL: PostRowsURL() + "?page=1", Want: both(200, 200)},
		maagotest.Row{Route: "post_create", URL: PostCreateURL(), Want: both(303, 303)},
		maagotest.Row{Route: "post_new", URL: PostNewURL(), Want: both(200, 200)},
		maagotest.Row{Route: "post_new_save", URL: PostNewSaveURL(), Body: "title=", Want: both(200, 200)},
		maagotest.Row{Route: "api_post", URL: APIPostURL(p.ID), Want: both(200, 200)},
		maagotest.Row{Route: "api_post_create", URL: APIPostCreateURL(), Body: `{"title": ""}`, Want: both(422, 422)},
		maagotest.Row{Route: "broken", URL: BrokenURL(), Want: both(500, 500)},
		maagotest.Row{Route: "post_export", URL: PostExportURL("csv"), Want: both(200, 200)},
		maagotest.Row{Route: "secret", URL: SecretURL(), Want: both(303, 200)},
		maagotest.Row{Route: "api_secret", URL: APISecretURL(), Want: both(401, 200)},
		maagotest.Row{Route: "mine", URL: MineURL(), Want: both(303, 200)},
		maagotest.Row{Route: "mine_post", URL: MinePostURL(), Want: both(401, 200)},
		maagotest.Row{Route: "api_greet", URL: APIGreetURL(), Want: both(200, 200)},
		maagotest.Row{Route: "api_mine", URL: APIMineURL(), Want: both(401, 200)},
		maagotest.Row{Route: "save_title", URL: SaveTitleURL("a"), Want: both(303, 303)},
		maagotest.Row{Route: "upload", URL: UploadURL(), Want: both(200, 200)},
		maagotest.Row{Route: "upload_save", URL: UploadSaveURL(), Want: both(400, 400)},
		maagotest.Row{Route: "media", URL: MediaURL("nokey"), Want: both(404, 404)},
		// A tag's name is taken once: each role saves its own.
		maagotest.Row{Route: "tag_save", URL: TagSaveURL(), Body: "name=a&kind=k&code=1", Want: both(303, maagotest.Skip)},
		maagotest.Row{Route: "tag_save", URL: TagSaveURL(), Body: "name=b&kind=k&code=2", Want: both(maagotest.Skip, 303)},
	)
}
