package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
