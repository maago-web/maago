package main

import (
	"fmt"
	"maps"
	"os"
	osexec "os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/build"
	"github.com/maago-web/maago/internal/migrate"
	"github.com/maago-web/maago/internal/project"
)

// doneBase is the models M2's completion test starts from.
const doneBase = `package blog

import m "github.com/maago-web/maago"

type Author struct {
	m.Model
	ID   int64
	Name string
}

type Post struct {
	m.Model
	ID       int64
	AuthorID int64  ~maago:"fk=Author"~
	Title    string ~maago:"max=200"~
	Text     string
}
`

// M2's completion conditions (MILESTONES), in both dialects:
//  1. from an empty project, a model, a field added, renamed and dropped,
//     each made by makemigrations and passing check, apply to a new
//     database as the models say;
//  2. a draft changed by hand so that it differs from the models fails
//     check;
//  3. a DROP without its allow-drop marker fails check.
//
// Leaving a step out breaks a condition: without --rename, the rename is
// a drop and an add (SQLite: a rebuild) that check stops; without the
// marker, condition 3.
func TestM2Done(t *testing.T) {
	for _, dd := range []draftDialect{draftSQLite, draftPG} {
		t.Run(string(dd.d), func(t *testing.T) {
			root := dd.makeProject(t, doneBase)

			// The models.
			if w := makeMigrations(t, root, ""); w != "migrations/0001_initial.sql" {
				t.Fatalf("wrote %q", w)
			}
			expect(t, root)

			// A field added in the middle: ADD COLUMN puts it last, which
			// still matches the models, here and in the database at the end.
			added := strings.Replace(doneBase, "	Title ", "	Slug     string ~maago:\"max=50,index,default=''\"~\n	Title ", 1)
			setModels(t, root, added)
			if w := makeMigrations(t, root, ""); w != "migrations/0002_add_blog_post_slug.sql" {
				t.Fatalf("wrote %q", w)
			}
			expect(t, root)

			// A field renamed. Without --rename it is a drop and an add
			// (SQLite: a rebuild) whose draft says to use --rename, and
			// which check stops at the DROP: condition 1 would fail.
			renamed := strings.Replace(added, "	Title    string", "	Heading  string", 1)
			setModels(t, root, renamed)
			w := makeMigrations(t, root, "")
			if !strings.Contains(readFile(t, filepath.Join(root, w)), "--rename blog_post.title:heading") {
				t.Fatalf("no --rename hint in %s", w)
			}
			expect(t, root, w+":")
			os.Remove(filepath.Join(root, w))
			if w := makeMigrations(t, root, "", "blog_post.title:heading"); w != "migrations/0003_rename_blog_post_title.sql" {
				t.Fatalf("wrote %q", w)
			}
			expect(t, root)

			// A field dropped: check wants the marker at the DROP (condition
			// 3), then passes.
			dropped := strings.Replace(renamed, "	Text     string\n", "", 1)
			setModels(t, root, dropped)
			if w := makeMigrations(t, root, ""); w != "migrations/0004_drop_blog_post_text.sql" {
				t.Fatalf("wrote %q", w)
			}
			unmarked := readFile(t, filepath.Join(root, "migrations", "0004_drop_blog_post_text.sql"))
			expect(t, root, fmt.Sprintf("migrations/0004_drop_blog_post_text.sql:%d:", lineOf(t, unmarked, `ALTER TABLE "blog_post" DROP COLUMN "text"`)))
			mark(t, root, "migrations/0004_drop_blog_post_text.sql")
			expect(t, root)

			// The migrations applied to a new database are the models.
			r := newRealDB(t, dd.d)
			r.root = root
			r.migrate(t, "0001_initial.sql", "0002_add_blog_post_slug.sql", "0003_rename_blog_post_title.sql", "0004_drop_blog_post_text.sql")
			url := ""
			if dd.d == project.Postgres {
				url = pgURL(t)
			}
			want, _ := dd.open(t, url, dropped)
			if cs := migrate.Diff(dd.schema(t, r.open(t)), dd.schema(t, want)); len(cs) > 0 {
				t.Errorf("the database differs from the models: %s", cs[0].Message())
			}

			// Condition 2: a draft changed by hand fails check, at the field.
			add := filepath.Join(root, "migrations", "0002_add_blog_post_slug.sql")
			writeFile(t, add, strings.Replace(readFile(t, add), "DEFAULT ''", "DEFAULT 'x'", 1))
			expect(t, root, "blog/models.go:15:2: "+differ+`column "slug" of "blog_post" has DEFAULT 'x'`)
		})
	}
}

// lineOf is the line of src that starts with prefix.
func lineOf(t *testing.T, src, prefix string) int {
	t.Helper()
	for i, l := range strings.Split(src, "\n") {
		if strings.HasPrefix(l, prefix) {
			return i + 1
		}
	}
	t.Fatalf("no %q in:\n%s", prefix, src)
	return 0
}

// M3's completion condition 3 (MILESTONES): an empty WHERE of a DELETE, a
// parameter its line's condition does not guarantee, and a model result
// type missing a column are each an error of maago gen, which then
// writes nothing (D-60). The first two stop before the database; only
// the third is found there, so it is the one both dialects really run.
// The other conditions' tests are named under "M3 끝" in docs/TASKS-M3.md.
func TestM3Done(t *testing.T) {
	cases := []struct{ name, file, src, want string }{
		{"empty WHERE of a DELETE", "purge.sql",
			"-- name: Purge :exec\n-- param: status? string\nDELETE FROM orders\nWHERE {\n  {status} AND status = :status\n}\n",
			"queries/purge.sql:4:1: WHERE { } of DELETE is empty when status absent, so it would touch every row"},
		{"parameter its line does not guarantee", "by_status.sql",
			"-- name: ByStatus :many Order\n-- param: status? string\nSELECT * FROM orders\nWHERE status = :status\n",
			"queries/by_status.sql:4:16: optional parameter status is used on a line that does not require {status}"},
		{"model result missing a column", "amounts.sql",
			"-- name: Amounts :many Order\nSELECT id, amount FROM orders\n",
			`queries/amounts.sql:2:1: result columns do not match model Order: missing "customer_id", "status", "priority", "created_at", "updated_at"`},
	}
	for _, d := range []project.Dialect{project.SQLite, project.Postgres} {
		for _, c := range cases {
			t.Run(string(d)+"/"+c.name, func(t *testing.T) {
				root := ordersProject(t, d)
				if _, ds := build.Gen(root); ds != nil {
					t.Fatal(text(ds))
				}
				// A change a successful gen would write, beside the error:
				// any file the failed gen writes shows in the snapshot.
				search := filepath.Join(root, "queries", "search_orders.sql")
				writeFile(t, search, strings.Replace(readFile(t, search), "LIMIT :limit", "LIMIT :limit OFFSET 0", 1))
				writeFile(t, filepath.Join(root, "queries", c.file), c.src)
				before := snapshot(t, root)
				_, ds := build.Gen(root)
				if got := diagnostics(text(ds)); len(got) != 1 || got[0] != c.want {
					t.Errorf("gen: %q\nwant %q", got, c.want)
				}
				if !maps.Equal(snapshot(t, root), before) {
					t.Error("the failed gen changed files")
				}
			})
		}
	}
}

// M4's completion conditions (MILESTONES) on the fixture app, in both
// dialects: a field that does not exist and a route name that does not
// are errors of maago gen, both in one run (D-61), and then it writes
// nothing (D-60); on PostgreSQL also the requests of conditions 3 to 5,
// the fixture's own tests (404s, the cross-origin check, login, the
// form's three results), which TestWeb runs on SQLite. The other
// conditions' tests are named under "M4 끝" in docs/TASKS-M4.md.
func TestM4Done(t *testing.T) {
	for _, d := range []project.Dialect{project.SQLite, project.Postgres} {
		t.Run(string(d), func(t *testing.T) {
			root := webProject(t)
			if d == project.Postgres {
				t.Setenv("MAAGO_DATABASE_URL", pgURL(t))
				if err := os.RemoveAll(filepath.Join(root, "migrations")); err != nil {
					t.Fatal(err)
				}
				if w := makeMigrations(t, root, ""); w != "migrations/0001_initial.sql" {
					t.Fatalf("wrote %q", w)
				}
				if m := readFile(t, filepath.Join(root, "migrations", "0001_initial.sql")); !strings.Contains(m, "GENERATED BY DEFAULT AS IDENTITY") {
					t.Fatalf("not PostgreSQL DDL:\n%s", m)
				}
			}
			if _, ds := build.Gen(root); ds != nil {
				t.Fatalf("maago gen:\n%s", text(ds))
			}
			if ds := build.Check(root); ds != nil {
				t.Fatalf("maago check:\n%s", text(ds))
			}
			if d == project.Postgres {
				test := osexec.Command("go", "test", "-count=1", "./...")
				test.Dir = root
				test.Env = append(os.Environ(), "GOFLAGS=-mod=readonly")
				if out, err := test.CombinedOutput(); err != nil {
					t.Fatalf("go test: %v\n%s", err, out)
				}
			}

			// A change a successful gen would write, beside the typos: any
			// file the failed gen writes shows in the snapshot.
			edit := func(name, old, new string) {
				p := filepath.Join(root, "site", filepath.FromSlash(name))
				src := readFile(t, p)
				if !strings.Contains(src, old) {
					t.Fatalf("%s has no %q", name, old)
				}
				writeFile(t, p, strings.Replace(src, old, new, 1))
			}
			edit("queries/title_taken.sql", "WHERE title = :title", "WHERE title = :title AND id > 0")
			edit("templates/post_page.html", "{{.Post.Title}}", "{{.Post.Titel}}")
			edit("templates/number_page.html", `{{url "post_detail" .N}}`, `{{url "post_detial" .N}}`)
			before := snapshot(t, root)
			_, ds := build.Gen(root)
			want := []string{
				`site/templates/number_page.html:2:10: unknown route name "post_detial"`,
				`site/templates/post_page.html:1:33: PostPage.Post has no field or method "Titel"`,
			}
			if got := diagnostics(text(ds)); !slices.Equal(got, want) {
				t.Errorf("gen: %q\nwant %q", got, want)
			}
			if !maps.Equal(snapshot(t, root), before) {
				t.Error("the failed gen changed files")
			}
		})
	}
}
