package maago_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/rt"
)

// Handler serves the apps of a project. Here one app is written by hand
// as maago gen would write it; a project's main gives each app's
// generated App instead, with the database of maago.OpenDB.
func ExampleHandler() {
	hello := rt.NewApp("example.com/app/hello", nil, rt.AppRoute{
		Method: "GET", Path: "/{$}", Name: "home",
		Serve: func(w http.ResponseWriter, r *http.Request) error {
			fmt.Fprint(w, "hello")
			return nil
		},
	})
	db := sql.OpenDB(nil) // no driver: nothing here asks the database
	h, err := maago.Handler(maago.Config{DB: db, Apps: []maago.App{hello}})
	if err != nil {
		fmt.Println(err)
		return
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	fmt.Println(w.Code, w.Body)
	// Output: 200 hello
}
