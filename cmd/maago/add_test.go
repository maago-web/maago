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
// dialects, also once a field is added to User. Nothing else compiles
// it: the go tool skips the scaffold's testdata.
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
	"log"
	"net/http"

	"github.com/maago-web/maago"
	_ "modernc.org/sqlite"

	"example.com/app/auth"
)

func main() {
	db, err := maago.OpenDB(context.Background(), maago.DBConfig{})
	if err != nil {
		log.Fatal(err)
	}
	h, err := maago.Handler(maago.Config{DB: db, Apps: []maago.App{auth.App}, Auth: auth.Current, LoginURL: "/login"})
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
