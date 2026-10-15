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
				project.SQLite:   {`CREATE TABLE "auth_user"`, `CREATE TABLE "auth_session"`, `CREATE TABLE "auth_token"`, `CREATE TABLE "auth_attempt"`, `CREATE TABLE "auth_api_token"`, `CREATE TABLE "auth_permission"`, `"is_superuser" INTEGER NOT NULL DEFAULT 0`, `"email" TEXT NOT NULL CHECK (length("email") <= 254)`, `"is_active" INTEGER NOT NULL DEFAULT 1`, `"is_staff" INTEGER NOT NULL DEFAULT 0`, `"verified_at" TEXT,`, `CREATE UNIQUE INDEX "auth_user_email_key"`},
				project.Postgres: {`CREATE TABLE "auth_user"`, `CREATE TABLE "auth_session"`, `CREATE TABLE "auth_token"`, `CREATE TABLE "auth_attempt"`, `CREATE TABLE "auth_api_token"`, `CREATE TABLE "auth_permission"`, `"is_superuser" BOOLEAN NOT NULL DEFAULT false`, `"email" VARCHAR(254) NOT NULL`, `"is_active" BOOLEAN NOT NULL DEFAULT true`, `"is_staff" BOOLEAN NOT NULL DEFAULT false`, `"verified_at" TIMESTAMPTZ,`, `CONSTRAINT "auth_user_email_key" UNIQUE ("email")`},
			}[dd.d]
			for _, w := range want {
				if !strings.Contains(initial, w) {
					t.Errorf("no %s in:\n%s", w, initial)
				}
			}
			expect(t, root)
			goTest(t, root)

			models := filepath.Join(root, "auth", "models.go")
			writeFile(t, models, strings.Replace(readFile(t, models), "\tIsActive ", "\tNickname string `maago:\"default=''\"`\n\tIsActive ", 1))
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
	"github.com/maago-web/maago/contrib/admin"
	_ "modernc.org/sqlite"

	"example.com/app/auth"
	"example.com/app/site"
)

func main() {
	createSuperuser := flag.Bool("createsuperuser", false, "create a superuser from MAAGO_SUPERUSER_EMAIL and MAAGO_SUPERUSER_PASSWORD")
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
	adminApp := admin.App(admin.Config{Models: [][]admin.Model{auth.Admin}, Can: auth.Can})
	h, err := maago.Handler(maago.Config{DB: db, Apps: []maago.App{auth.App, site.App, adminApp}, Auth: auth.Current, LoginURL: "/login"})
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
	maago.GET("/staff", "staff", Staff),
	maago.GET("/api/me", "api_me", Me, maago.API, maago.LoginRequired),
}
`,
	"views.go": `package site

import (
	"context"

	"github.com/maago-web/maago"

	"example.com/app/auth"
)

type StaffPage struct{ Email string }

type MeOut struct {
	Email string ` + "`json:\"email\"`" + `
}

func Me(ctx context.Context, in struct{}) (MeOut, error) {
	u, _ := maago.UserFrom[auth.User](ctx) // the route requires login
	return MeOut{u.Email}, nil
}

func Staff(ctx context.Context, u auth.User, in struct{}) (StaffPage, error) {
	if err := auth.RequireStaff(u); err != nil {
		return StaffPage{}, err
	}
	return StaffPage{u.Email}, nil
}
`,
	"admin_test.go": `package site

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/contrib/admin"
	authlib "github.com/maago-web/maago/contrib/auth"
	"github.com/maago-web/maago/maagotest"

	"example.com/app/auth"
)

// The admin on the copied auth (D-133, M6's condition 4): who may see and
// do what, the models it hides, and the hash it never shows nor writes.
func TestAdmin(t *testing.T) {
	authlib.Iterations = 1000
	db := maagotest.DB(t)
	adminApp := admin.App(admin.Config{Models: [][]admin.Model{auth.Admin}, Can: auth.Can})
	h, err := maago.Handler(maago.Config{DB: db, Apps: []maago.App{auth.App, App, adminApp}, Auth: auth.Current, LoginURL: "/login"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const password = "correct horse battery staple"
	hash, err := authlib.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	user := func(email string, staff, super bool) auth.User {
		u, err := auth.InsertUser(ctx, db, auth.NewUserInput{Email: email, PasswordHash: hash, VerifiedAt: &now, IsStaff: maago.Set(staff), IsSuperuser: maago.Set(super)})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	ann, staff, ed, _ := user("ann@example.com", false, false), user("staff@example.com", true, false), user("ed@example.com", true, false), user("boss@example.com", true, true)
	gone := user("gone@example.com", false, false) // never signs in: no session refers to it
	for _, p := range [][2]string{{"User", "view"}, {"Permission", "view"}, {"Permission", "add"}} { // add: a superuser's job all the same
		if _, err := auth.InsertPermission(ctx, db, auth.NewPermissionInput{UserID: ed.ID, App: "auth", ModelName: p[0], Action: p[1]}); err != nil {
			t.Fatal(err)
		}
	}
	do := func(session, method, path string, form url.Values) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		if form != nil {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		r.AddCookie(&http.Cookie{Name: authlib.CookieName, Value: session})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	login := func(email string) string {
		w := do("", "POST", "/login", url.Values{"email": {email}, "password": {password}})
		if w.Code != 303 || len(w.Result().Cookies()) != 1 {
			t.Fatalf("login %s: %d", email, w.Code)
		}
		return w.Result().Cookies()[0].Value
	}
	sessions := map[string]string{}
	for _, u := range []string{"ann", "staff", "ed", "boss"} {
		sessions[u] = login(u + "@example.com")
	}
	annRow := fmt.Sprintf("/admin/auth/User/%d/", ann.ID)
	change := url.Values{"email": {"ann2@example.com"}, "is_active": {"on"}, "verified_at": {"2026-01-01T00:00:00"}}
	grant := url.Values{"user_id": {fmt.Sprint(staff.ID)}, "app": {"auth"}, "model_name": {"User"}, "action": {"view"}}
	for _, c := range []struct {
		method, path string
		form         url.Values
		codes        [4]int // ann, staff, ed, boss
	}{
		{"GET", "/admin/", nil, [4]int{403, 403, 200, 200}},
		{"GET", "/admin/auth/User/", nil, [4]int{403, 403, 200, 200}},
		{"GET", "/admin/auth/Permission/", nil, [4]int{403, 403, 200, 200}},
		{"GET", annRow, nil, [4]int{403, 403, 200, 200}},
		{"GET", "/admin/auth/Permission/add/", nil, [4]int{403, 403, 403, 200}},
		{"GET", "/admin/auth/Session/", nil, [4]int{403, 403, 403, 404}},
		{"GET", "/admin/auth/User/add/", nil, [4]int{403, 403, 403, 404}}, // the hash would be empty (D-133)
		{"POST", annRow, change, [4]int{403, 403, 403, 303}},
		{"GET", fmt.Sprintf("/admin/auth/User/delete/?id=%d", gone.ID), nil, [4]int{403, 403, 403, 200}},
		{"POST", "/admin/auth/User/delete/", url.Values{"id": {fmt.Sprint(gone.ID)}}, [4]int{403, 403, 403, 303}},
		{"POST", "/admin/auth/Permission/add/", url.Values{"user_id": {fmt.Sprint(ed.ID)}, "app": {"auth"}, "model_name": {"User"}, "action": {"change"}}, [4]int{403, 403, 403, 303}},
	} {
		for i, u := range []string{"ann", "staff", "ed", "boss"} {
			if w := do(sessions[u], c.method, c.path, c.form); w.Code != c.codes[i] {
				t.Errorf("%s %s %s: %d, want %d", u, c.method, c.path, w.Code, c.codes[i])
			}
		}
	}

	body := do(sessions["boss"], "GET", "/admin/", nil).Body.String()
	for _, hidden := range []string{"Session", "Token", "Attempt", "APIToken"} {
		if strings.Contains(body, ">"+hidden+"<") {
			t.Errorf("the index shows %s:\n%s", hidden, body)
		}
	}
	if body := do(sessions["boss"], "GET", annRow, nil).Body.String(); strings.Contains(body, "password") || strings.Contains(body, hash) || strings.Contains(body, "Password") {
		t.Errorf("the User page shows the hash:\n%s", body)
	}
	if ann2, err := auth.GetUser(ctx, db, ann.ID); err != nil || ann2.Email != "ann2@example.com" || ann2.PasswordHash != hash {
		t.Errorf("after boss changed ann: %+v %v", ann2, err)
	}
	login("ann2@example.com") // the hash is as it was
	// The admin saves the email as typed, not as the views store it (the
	// help of User.Email says so, D-135).
	if body := do(sessions["boss"], "GET", annRow, nil).Body.String(); !strings.Contains(body, "lowercase") {
		t.Errorf("the User page has no help for the email:\n%s", body)
	}
	change.Set("email", " Ann@example.com")
	if w := do(sessions["boss"], "POST", annRow, change); w.Code != 303 {
		t.Fatalf("boss changing ann's email: %d\n%s", w.Code, w.Body)
	}
	if ann2, err := auth.GetUser(ctx, db, ann.ID); err != nil || ann2.Email != " Ann@example.com" {
		t.Errorf("the admin stored the email as %q, %v", ann2.Email, err)
	}
	if _, err := auth.GetUser(ctx, db, gone.ID); err == nil {
		t.Error("boss did not delete gone")
	}
	// A user who signed in has sessions: not even a superuser deletes it.
	w := do(sessions["boss"], "POST", "/admin/auth/User/delete/", url.Values{"id": {fmt.Sprint(ann.ID)}})
	if body := w.Body.String(); w.Code != 200 || !strings.Contains(body, "rows of auth.Session (UserID) refer to it") {
		t.Errorf("deleting a user with sessions: %d\n%s", w.Code, body)
	}

	if w := do(sessions["staff"], "GET", "/admin/auth/User/", nil); w.Code != 403 {
		t.Fatalf("staff before the grant: %d", w.Code)
	}
	if w := do(sessions["boss"], "POST", "/admin/auth/Permission/add/", grant); w.Code != 303 {
		t.Fatalf("grant: %d\n%s", w.Code, w.Body)
	}
	if w := do(sessions["staff"], "GET", "/admin/auth/User/", nil); w.Code != 200 {
		t.Errorf("staff after the grant: %d", w.Code)
	}
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

// A frontend on another origin calls the API with a token (D-129).
func TestAPIToken(t *testing.T) {
	authlib.Iterations = 1000
	db := maagotest.DB(t)
	const front = "https://app.example.com"
	h, err := maago.Handler(maago.Config{DB: db, Apps: []maago.App{auth.App, App}, Auth: auth.Current, LoginURL: "/login", Origins: []string{front}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	u, err := auth.CreateSuperuser(ctx, db, "ann@example.com", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	token, hash := authlib.NewToken()
	tk, err := auth.InsertAPIToken(ctx, db, auth.NewAPITokenInput{UserID: u.ID, Name: "front", Hash: hash})
	if err != nil {
		t.Fatal(err)
	}
	send := func(method string, header map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/me", nil)
		r.Header.Set("Origin", front)
		for k, v := range header {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := send("OPTIONS", map[string]string{"Access-Control-Request-Method": "GET", "Access-Control-Request-Headers": "authorization"})
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != front || !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Errorf("preflight: %d %v", w.Code, w.Header())
	}
	w = send("GET", map[string]string{"Authorization": "Bearer " + token})
	if w.Code != 200 || w.Body.String() != "{\"email\":\"ann@example.com\"}\n" || w.Header().Get("Access-Control-Allow-Origin") != front {
		t.Errorf("with the token: %d %q %v", w.Code, w.Body, w.Header())
	}
	if err := auth.DeleteAPIToken(ctx, db, tk.ID); err != nil {
		t.Fatal(err)
	}
	if w := send("GET", map[string]string{"Authorization": "Bearer " + token}); w.Code != 401 || w.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Errorf("a deleted token: %d %v", w.Code, w.Header())
	}
}
`,
}
