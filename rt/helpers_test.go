package rt

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/internal/rtcore"
)

// noDriver is a connector that is never connected: Handler only carries
// the *sql.DB to the views.
type noDriver struct{}

func (noDriver) Connect(context.Context) (driver.Conn, error) { return nil, errors.New("no driver") }
func (noDriver) Driver() driver.Driver                        { return nil }

var testDB = sql.OpenDB(noDriver{})

// serve answers one request on a handler of cfg, and returns the
// response and what OnError heard.
func serve(t *testing.T, cfg maago.Config, r *http.Request) (*httptest.ResponseRecorder, []error) {
	t.Helper()
	var reported []error
	cfg.DB = testDB
	cfg.OnError = func(_ context.Context, _ *http.Request, err error) { reported = append(reported, err) }
	h, err := maago.Handler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w, reported
}

func route(method, path string, serve func(w http.ResponseWriter, r *http.Request) error) AppRoute {
	return AppRoute{Method: method, Path: path, Name: strings.Trim(path, "/{}"), Serve: serve}
}

func ok(w http.ResponseWriter, r *http.Request) error { fmt.Fprint(w, "ok"); return nil }

// code is the status err answers with, as Handler decides it.
func code(err error) int {
	if err == nil {
		return 0
	}
	return rtcore.Code(err)
}
