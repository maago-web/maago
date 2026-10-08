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

// The auth app maago add writes is an ordinary app (D-123): its table
// goes into makemigrations and check passes, in both dialects, also once
// a field is added to User; and it compiles, which nothing else checks,
// since the go tool skips the scaffold's testdata.
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
			writeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.25.0\n\nrequire github.com/maago-web/maago v0.0.0\n")
			writeFile(t, filepath.Join(root, "go.work"), "go 1.25.0\n\nuse .\n\nreplace github.com/maago-web/maago => "+repo+"\n")
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

			models := filepath.Join(root, "auth", "models.go")
			writeFile(t, models, strings.Replace(readFile(t, models), "\tCreatedAt ", "\tNickname     string     `maago:\"default=''\"`\n\tCreatedAt ", 1))
			if added := readFile(t, filepath.Join(root, makeMigrations(t, root, ""))); !strings.Contains(added, `"nickname"`) {
				t.Errorf("no nickname:\n%s", added)
			}
			expect(t, root)

			vet := osexec.Command("go", "vet", "./...")
			vet.Dir = root
			vet.Env = append(os.Environ(), "GOFLAGS=-mod=readonly", "GOPROXY=off")
			if out, err := vet.CombinedOutput(); err != nil {
				t.Errorf("go vet: %v\n%s", err, out)
			}
		})
	}
}
