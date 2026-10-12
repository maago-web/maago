package main

import (
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/project"
	"github.com/maago-web/maago/internal/scaffold"
)

// The auth app maago add writes is an ordinary app (D-123): its tables
// go into makemigrations, check passes and its own tests pass, in both
// dialects, also once a field is added to User; and another app's view
// lets in only staff with one line (D-130). Nothing else compiles it:
// the go tool skips the scaffold's testdata.
func TestAddAuth(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, dd := range []draftDialect{draftSQLite, draftPG} {
		t.Run(string(dd.d), func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			// The fixture's module: both drivers, with their sums.
			writeFile(t, filepath.Join(root, "go.mod"), strings.Replace(readFile(t, "testdata/web/go.mod"), "module example.com/web", "module example.com/app", 1))
			writeFile(t, filepath.Join(root, "go.sum"), readFile(t, "testdata/web/go.sum"))
			writeFile(t, filepath.Join(root, "go.work"), "go 1.25.0\n\nuse .\n\nreplace github.com/maago-web/maago => "+repo+"\n")
			writeFile(t, filepath.Join(root, "main.go"), authMain)
			t.Setenv("GOWORK", filepath.Join(root, "go.work"))
			url := "sqlite:app.db"
			if dd.d == project.Postgres {
				url = pgURL(t)
			}
			t.Setenv("MAAGO_DATABASE_URL", url)
			if _, ds := scaffold.Add(root, "auth"); ds != nil {
				t.Fatal(text(ds))
			}
			for name, src := range staffApp {
				writeFile(t, filepath.Join(root, "site", name), src)
			}
			driver := readFile(t, filepath.Join(root, "auth", "driver_test.go"))
			writeFile(t, filepath.Join(root, "site", "driver_test.go"), strings.Replace(driver, "package auth", "package site", 1))

			initial := readFile(t, filepath.Join(root, makeMigrations(t, root, "")))
			want := map[project.Dialect][]string{
				project.SQLite:   {`"email" TEXT NOT NULL CHECK (length("email") <= 254)`, `"is_active" INTEGER NOT NULL DEFAULT 1`, `"is_staff" INTEGER NOT NULL DEFAULT 0`, `"verified_at" TEXT,`, `CREATE UNIQUE INDEX "auth_user_email_key"`},
				project.Postgres: {`"email" VARCHAR(254) NOT NULL`, `"is_active" BOOLEAN NOT NULL DEFAULT true`, `"is_staff" BOOLEAN NOT NULL DEFAULT false`, `"verified_at" TIMESTAMPTZ,`, `CONSTRAINT "auth_user_email_key" UNIQUE ("email")`},
			}[dd.d]
			for _, w := range want {
				if !strings.Contains(initial, w) {
					t.Errorf("no %s in:\n%s", w, initial)
				}
			}
			expect(t, root)
			goTest(t, root)

			models := filepath.Join(root, "auth", "models.go")
			writeFile(t, models, strings.Replace(readFile(t, models), "\tPasswordHash string\n", "\tPasswordHash string\n\tNickname     string `maago:\"default=''\"`\n", 1))
			if added := readFile(t, filepath.Join(root, makeMigrations(t, root, ""))); !strings.Contains(added, `"nickname"`) {
				t.Errorf("no nickname:\n%s", added)
			}
			expect(t, root)
			goTest(t, root)
		})
	}
}

// authMain serves the auth app, as main does once maago add auth wrote it.
// It is only compiled, so its SQLite driver serves the PostgreSQL run too.
const authMain = `package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/maago-web/maago"
	_ "modernc.org/sqlite"

	"example.com/app/auth"
	"example.com/app/site"
)

func main() {
	createSuperuser := flag.Bool("createsuperuser", false, "create a staff account from MAAGO_SUPERUSER_EMAIL and MAAGO_SUPERUSER_PASSWORD")
	flag.Parse()
	ctx := context.Background()
	db, err := maago.OpenDB(ctx, maago.DBConfig{})
	if err != nil {
		log.Fatal(err)
	}
	if *createSuperuser {
		u, err := auth.CreateSuperuser(ctx, db, os.Getenv("MAAGO_SUPERUSER_EMAIL"), os.Getenv("MAAGO_SUPERUSER_PASSWORD"))
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("created %s", u.Email)
		return
	}
	h, err := maago.Handler(maago.Config{DB: db, Apps: []maago.App{auth.App, site.App}, Auth: auth.Current, LoginURL: "/login"})
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.ListenAndServe(":8000", h))
}
`

// goTest runs the project's own tests, offline, on the database of
// MAAGO_DATABASE_URL.
func goTest(t *testing.T, root string) {
	t.Helper()
	test := osexec.Command("go", "test", "-count=1", "./...")
	test.Dir = root
	test.Env = append(os.Environ(), "GOFLAGS=-mod=readonly", "GOPROXY=off")
	if out, err := test.CombinedOutput(); err != nil {
		t.Errorf("go test: %v\n%s", err, out)
	}
}

// staffApp is an app of the project with a page for staff only.
var staffApp = map[string]string{
	"urls.go": `package site

import "github.com/maago-web/maago"

var Routes = maago.Routes{
	maago.GET("/staff", "staff", Staff, maago.LoginRequired),
}
`,
	"views.go": `package site

import (
	"context"

	"example.com/app/auth"
)

type StaffPage struct{ Email string }

func Staff(ctx context.Context, in struct{}) (StaffPage, error) {
	u, err := auth.RequireStaff(ctx)
	if err != nil {
		return StaffPage{}, err
	}
	return StaffPage{u.Email}, nil
}
`,
	"templates/base.html":       `<!doctype html><title>Site</title>{{block "content" .}}{{end}}`,
	"templates/staff_page.html": `{{define "content"}}<p>Staff: {{.Email}}</p>{{end}}`,
	"site_test.go": `package site

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/maago-web/maago"
	authlib "github.com/maago-web/maago/contrib/auth"
	"github.com/maago-web/maago/maagotest"

	"example.com/app/auth"
)

func TestStaff(t *testing.T) {
	authlib.Iterations = 1000
	db := maagotest.DB(t)
	h, err := maago.Handler(maago.Config{DB: db, Apps: []maago.App{auth.App, App}, Auth: auth.Current, LoginURL: "/login"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const password = "correct horse battery staple"
	if _, err := auth.CreateSuperuser(ctx, db, "boss@example.com", password); err != nil {
		t.Fatal(err)
	}
	hash, err := authlib.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if _, err := auth.InsertUser(ctx, db, auth.NewUserInput{Email: "ann@example.com", PasswordHash: hash, VerifiedAt: &now}); err != nil {
		t.Fatal(err)
	}
	get := func(session string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/staff", nil)
		if session != "" {
			r.AddCookie(&http.Cookie{Name: authlib.CookieName, Value: session})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	login := func(email string) string {
		r := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{"email": {email}, "password": {password}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 303 || len(w.Result().Cookies()) != 1 {
			t.Fatalf("login %s: %d", email, w.Code)
		}
		return w.Result().Cookies()[0].Value
	}
	if w := get(""); w.Code != 303 {
		t.Errorf("nobody: %d", w.Code)
	}
	if w := get(login("ann@example.com")); w.Code != 403 || strings.Contains(w.Body.String(), "ann@") {
		t.Errorf("not staff: %d %s", w.Code, w.Body)
	}
	if w := get(login("boss@example.com")); w.Code != 200 || !strings.Contains(w.Body.String(), "Staff: boss@example.com") {
		t.Errorf("staff: %d %s", w.Code, w.Body)
	}
}
`,
}
