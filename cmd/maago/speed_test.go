package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/maago-web/maago/internal/build"
)

// maago gen on a project of usual size (D-144: 5 apps, 30 models, 100
// queries, and a page an app, so that its Go is type-checked) takes at
// most 1 second on SQLite and 3 on PostgreSQL (D-65):
// the median of five runs after a first, which warms Go's build cache.
// Each run starts with no generated file, so it checks and writes all of
// them; the practice database (SQLite in memory, a schema of the PG
// server) is opened and filled in the time, makemigrations before it. The
// time is the gate here; CI's shared runners only print it (D-144).
func TestGenSpeed(t *testing.T) {
	for _, c := range []struct {
		name, url string
		limit     time.Duration
	}{
		{"sqlite", "sqlite:app.db", time.Second},
		{"postgres", os.Getenv("MAAGO_TEST_PG_URL"), 3 * time.Second},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.url == "" {
				pgURL(t) // skips, or fails in CI
			}
			root := speedProject(t)
			t.Setenv("MAAGO_DATABASE_URL", c.url)
			if _, ds := build.MakeMigrations(root, "", nil); ds != nil {
				t.Fatalf("makemigrations:\n%s", text(ds))
			}
			var runs []time.Duration
			for i := range 6 {
				gens, _ := filepath.Glob(filepath.Join(root, "*", "*_gen.go"))
				for _, f := range gens {
					os.Remove(f)
				}
				start := time.Now()
				if _, ds := build.Gen(root); ds != nil {
					t.Fatalf("gen:\n%s", text(ds))
				}
				if i == 0 {
					t.Logf("maago gen, the first: %v", time.Since(start).Round(time.Millisecond))
				} else {
					runs = append(runs, time.Since(start))
				}
			}
			slices.Sort(runs)
			median := runs[len(runs)/2]
			t.Logf("maago gen: median %v of %v (limit %v)", median.Round(time.Millisecond), runs, c.limit)
			if median > c.limit && os.Getenv("CI") == "" {
				t.Errorf("maago gen took %v, over %v (D-65)", median, c.limit)
			}
		})
	}
}

// speedProject writes a project of D-144's size: five apps of six models,
// each model pointing at the one before, and twenty queries an app, of
// the kinds a service has (a list, one by a column, a search with a
// condition line, a count).
func speedProject(t *testing.T) string {
	t.Helper()
	noEnv(t)
	root := t.TempDir()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/speed\n\ngo 1.25.0\n\nrequire github.com/maago-web/maago v0.0.0-00010101000000-000000000000\n\nreplace github.com/maago-web/maago => "+repo+"\n")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOPROXY", "off")
	queries := 0
	for a := 1; a <= 5; a++ {
		app := fmt.Sprintf("app%d", a)
		var models strings.Builder
		fmt.Fprintf(&models, "package %s\n\nimport (\n\t\"time\"\n\n\t\"github.com/maago-web/maago\"\n)\n", app)
		for m := 1; m <= 6; m++ {
			name := fmt.Sprintf("Thing%d", m)
			fmt.Fprintf(&models, "\ntype %s struct {\n\tmaago.Model\n\n\tID        int64\n\tName      string `maago:\"max=100,label='이름'\"`\n\tNote      string\n\tCount     int64     `maago:\"default=0\"`\n\tCreatedAt time.Time `maago:\"auto=create\"`\n", name)
			if m > 1 {
				fmt.Fprintf(&models, "\tParentID  *int64    `maago:\"fk=Thing%d\"`\n", m-1)
			}
			models.WriteString("}\n")
			table := fmt.Sprintf("%s_thing%d", app, m)
			cols := "id, name, note, count, created_at"
			if m > 1 {
				cols += ", parent_id"
			}
			files := map[string]string{
				fmt.Sprintf("list_thing%d.sql", m):    fmt.Sprintf("-- name: ListThing%d :many %s\n-- param: limit int64\nSELECT %s FROM %s ORDER BY id LIMIT :limit\n", m, name, cols, table),
				fmt.Sprintf("thing%d_by_name.sql", m): fmt.Sprintf("-- name: Thing%dByName :one %s\n-- param: name string\nSELECT %s FROM %s WHERE name = :name\n", m, name, cols, table),
				fmt.Sprintf("search_thing%d.sql", m):  fmt.Sprintf("-- name: SearchThing%d :many Thing%dRow\n-- param: q? string\n-- param: limit int64\nSELECT id, name FROM %s\nWHERE count >= 0\n  {q} AND name LIKE '%%' || :q || '%%'\nORDER BY id LIMIT :limit\n", m, m, table),
			}
			if m <= 2 {
				files[fmt.Sprintf("count_thing%d.sql", m)] = fmt.Sprintf("-- name: CountThing%d :one Thing%dCount\n-- column: n int64\nSELECT count(*) AS n FROM %s\n", m, m, table)
			}
			for f, sql := range files {
				writeFile(t, filepath.Join(root, app, "queries", f), sql)
				queries++
			}
		}
		writeFile(t, filepath.Join(root, app, "models.go"), models.String())
		// A page, so that gen type-checks the app's Go as it does a
		// service's (D-117).
		writeFile(t, filepath.Join(root, app, "urls.go"), fmt.Sprintf("package %[1]s\n\nimport \"github.com/maago-web/maago\"\n\nvar Routes = maago.Routes{\n\tmaago.GET(\"/%[1]s/{$}\", \"%[1]s_list\", List),\n}\n", app))
		writeFile(t, filepath.Join(root, app, "views.go"), fmt.Sprintf("package %s\n\nimport (\n\t\"context\"\n\n\t\"github.com/maago-web/maago\"\n)\n\ntype ListPage struct {\n\tThings []Thing1\n}\n\nfunc List(ctx context.Context, db maago.DB, in struct{}) (ListPage, error) {\n\tts, err := ListThing1(ctx, db, ListThing1Params{Limit: 20})\n\treturn ListPage{ts}, err\n}\n", app))
		writeFile(t, filepath.Join(root, app, "templates", "base.html"), `<!doctype html><title>Speed</title>{{block "content" .}}{{end}}`)
		writeFile(t, filepath.Join(root, app, "templates", "list_page.html"), `{{define "content"}}<ul>{{range .Things}}<li>{{.Name}}</li>{{end}}</ul>{{end}}`)
	}
	writeFile(t, filepath.Join(root, "main.go"), `package main

import (
	"github.com/maago-web/maago"

	"example.com/speed/app1"
	"example.com/speed/app2"
	"example.com/speed/app3"
	"example.com/speed/app4"
	"example.com/speed/app5"
)

var apps = []maago.App{app1.App, app2.App, app3.App, app4.App, app5.App}

func main() {}
`)
	if queries != 100 {
		t.Fatalf("%d queries, want 100", queries)
	}
	return root
}
