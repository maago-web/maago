// Command web serves the fixture app of cmd/maago's TestWeb.
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/maago-web/maago"
	_ "modernc.org/sqlite"

	"example.com/web/site"
)

func main() {
	db, err := maago.OpenDB(context.Background(), maago.DBConfig{})
	if err != nil {
		log.Fatal(err)
	}
	h, err := maago.Handler(maago.Config{DB: db, Apps: []maago.App{site.App}, Auth: site.Auth, LoginURL: "/login"})
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.ListenAndServe(":8000", h))
}
