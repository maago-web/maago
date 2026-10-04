package main

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/build"
	"github.com/maago-web/maago/internal/golden"
	"github.com/maago-web/maago/internal/project"
)

// makeBase is the models the makemigrations steps start from.
const makeBase = `package blog

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
	Text     string ~maago:"max=500,index"~
}
`

// makeProject writes a project of src for dd without migrations.
func (dd draftDialect) makeProject(t *testing.T, src string) string {
	t.Helper()
	env := "sqlite:app.db"
	if dd.d == project.Postgres {
		env = pgURL(t)
	}
	t.Setenv("MAAGO_DATABASE_URL", env)
	return modelProject(t, src)
}

func setModels(t *testing.T, root, src string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "blog", "models.go"), strings.ReplaceAll(src, "~", "`"))
}

// makeMigrations runs makemigrations and returns the file it wrote, failing t on
// a diagnostic.
func makeMigrations(t *testing.T, root, name string, renames ...string) string {
	t.Helper()
	written, ds := build.MakeMigrations(root, name, renames)
	if len(ds) > 0 {
		t.Fatalf("makemigrations:\n%s", text(ds))
	}
	return written
}

// mark puts the allow-drop marker before the DROP COLUMN of a migration.
func mark(t *testing.T, root, migration string) {
	t.Helper()
	p := filepath.Join(root, migration)
	src := readFile(t, p)
	i := strings.Index(src, "ALTER TABLE \"blog_post\" DROP COLUMN")
	if i < 0 {
		t.Fatalf("no DROP COLUMN in:\n%s", src)
	}
	writeFile(t, p, src[:i]+"-- maago: allow-drop\n"+src[i:])
}

// The steps of the task: models, a field added, renamed, dropped.
func TestMakeMigrations(t *testing.T) {
	for _, dd := range []draftDialect{draftSQLite, draftPG} {
		t.Run(string(dd.d), func(t *testing.T) {
			root := dd.makeProject(t, makeBase)
			var out strings.Builder
			step := func(want string, written string) {
				t.Helper()
				if written != want {
					t.Fatalf("wrote %q, want %q", written, want)
				}
				out.WriteString("-- " + written + "\n\n" + readFile(t, filepath.Join(root, written)) + "\n")
			}

			step("migrations/0001_initial.sql", makeMigrations(t, root, ""))
			expect(t, root)

			// A field with an index: the index follows it and is not in the name.
			withSlug := strings.Replace(makeBase, "	Text ", "	Slug     string ~maago:\"max=50,index,default=''\"~\n	Text ", 1)
			setModels(t, root, withSlug)
			step("migrations/0002_add_blog_post_slug.sql", makeMigrations(t, root, ""))
			expect(t, root)

			// Text becomes Body, which also allows NULL: first seen as a
			// drop and an add, with the warning; check wants the marker.
			renamed := strings.Replace(withSlug, "	Text     string ", "	Body     *string", 1)
			setModels(t, root, renamed)
			step("migrations/0003_blog_post.sql", makeMigrations(t, root, ""))
			expect(t, root, "migrations/0003_blog_post.sql:")
			os.Remove(filepath.Join(root, "migrations", "0003_blog_post.sql"))
			step("migrations/0003_blog_post.sql", makeMigrations(t, root, "", "blog_post.text:body"))
			expect(t, root)

			// Dropping a field wants the marker; with it, check passes.
			dropped := strings.Replace(renamed, "	Slug     string ~maago:\"max=50,index,default=''\"~\n", "", 1)
			setModels(t, root, dropped)
			step("migrations/0004_drop_blog_post_slug.sql", makeMigrations(t, root, ""))
			expect(t, root, "migrations/0004_drop_blog_post_slug.sql:")
			mark(t, root, "migrations/0004_drop_blog_post_slug.sql")
			expect(t, root)

			// PostgreSQL keeps the index's old name through the rename;
			// the draft drops it by the name the database has.
			setModels(t, root, strings.Replace(dropped, "	Body     *string~maago:\"max=500,index\"~\n", "", 1))
			step("migrations/0005_drop_blog_post_body.sql", makeMigrations(t, root, ""))
			mark(t, root, "migrations/0005_drop_blog_post_body.sql")
			expect(t, root)

			if written := makeMigrations(t, root, ""); written != "" {
				t.Errorf("no changes wrote %q", written)
			}
			golden.Check(t, "testdata/makemigrations_"+string(dd.d)+".golden", []byte(out.String()))
		})
	}
}

// What makemigrations refuses writes nothing.
func TestMakeMigrationsRefuses(t *testing.T) {
	refuse := func(t *testing.T, root, name string, renames []string, want ...string) {
		t.Helper()
		before := snapshot(t, root)
		written, ds := build.MakeMigrations(root, name, renames)
		out := text(ds)
		for _, w := range want {
			if !strings.Contains(out, w) {
				t.Errorf("no %q in:\n%s", w, out)
			}
		}
		if written != "" || len(ds) == 0 || !maps.Equal(before, snapshot(t, root)) {
			t.Errorf("wrote %q:\n%s", written, out)
		}
	}
	started := func(t *testing.T) string {
		root := draftSQLite.makeProject(t, makeBase)
		makeMigrations(t, root, "")
		setModels(t, root, strings.Replace(makeBase, "	Text     string ", "	Body     string ", 1))
		return root
	}
	t.Run("rename not in the changes", func(t *testing.T) {
		refuse(t, started(t), "", []string{"blog_post.title:body"}, `column "title" of "blog_post" is not dropped`)
		refuse(t, started(t), "", []string{"blog_post.text:title"}, `column "title" of "blog_post" is not added`)
	})
	t.Run("rename twice", func(t *testing.T) {
		refuse(t, started(t), "", []string{"blog_post.text:body", "blog_post.text:other"}, `names column "text" of "blog_post" again`)
		refuse(t, started(t), "", []string{"blog_post.text:body", "blog_post.title:body"}, `names column "body" of "blog_post" again`)
	})
	// Flags are checked with the migrations, before any practice database.
	t.Run("bad flags", func(t *testing.T) {
		root := started(t)
		writeFile(t, filepath.Join(root, "migrations", "0002_drop.sql"), "CREATE TABLE x (a int);\nDROP TABLE x;\n")
		refuse(t, root, "Bad-Name", []string{"blog_post.text"}, `--name "Bad-Name"`, `--rename "blog_post.text" is not table.old:new`, "DROP TABLE deletes data")
	})
	t.Run("migration does not apply", func(t *testing.T) {
		root := started(t)
		writeFile(t, filepath.Join(root, "migrations", "0002_bad.sql"), "CREATE TABLE x (a int,, b int);\n")
		refuse(t, root, "", nil, "migrations/0002_bad.sql:1:")
	})
	t.Run("unmarked drop left", func(t *testing.T) {
		root := started(t)
		writeFile(t, filepath.Join(root, "migrations", "0002_drop.sql"), "CREATE TABLE x (a int);\nDROP TABLE x;\n")
		refuse(t, root, "", nil, "migrations/0002_drop.sql:2:1: DROP TABLE deletes data")
	})
	t.Run("two renames", func(t *testing.T) {
		root := started(t)
		setModels(t, root, strings.Replace(strings.Replace(makeBase, "	Text     string ", "	Body     string ", 1), "	Title    string ", "	Heading  string ", 1))
		written := makeMigrations(t, root, "", "blog_post.text:body", "blog_post.title:heading")
		if src := readFile(t, filepath.Join(root, written)); written != "migrations/0002_blog_post.sql" || strings.Count(src, "RENAME COLUMN") != 2 {
			t.Errorf("wrote %q:\n%s", written, src)
		}
		expect(t, root)
	})
	// A SQLite rebuild that loses a column is written; check then wants
	// its marker, as for any drop.
	t.Run("rebuild losing a column", func(t *testing.T) {
		root := draftSQLite.makeProject(t, makeBase)
		makeMigrations(t, root, "")
		setModels(t, root, strings.Replace(makeBase, "	AuthorID int64  ~maago:\"fk=Author\"~\n", "", 1))
		written := makeMigrations(t, root, "")
		if written != "migrations/0002_drop_blog_post_author_id.sql" {
			t.Fatalf("wrote %q", written)
		}
		expect(t, root, written+":")
	})
	t.Run("name wins", func(t *testing.T) {
		root := started(t)
		if written := makeMigrations(t, root, "my_change", "blog_post.text:body"); written != "migrations/0002_my_change.sql" {
			t.Errorf("wrote %q", written)
		}
	})
	// Two models that point at each other, dropped together, make a
	// draft PostgreSQL refuses (task 6's TODO): nothing is written.
	t.Run("draft fails", func(t *testing.T) {
		both := strings.Replace(makeBase, "	Name string\n", "	Name string\n	BestID *int64 ~maago:\"fk=Post\"~\n", 1)
		root := draftPG.makeProject(t, both)
		makeMigrations(t, root, "")
		setModels(t, root, "package blog\n")
		refuse(t, root, "", nil, "maago did not write migrations/0002_", "fix: write this migration by hand")
	})
}

// Two apps that refer to each other (D-124), the auth app's table named
// with table=: makemigrations, check and migrate in both dialects, rows
// kept, then a change that makes SQLite rebuild blog_post, whose FK
// points into the other app, applied over those rows.
func TestCrossAppMigrations(t *testing.T) {
	const auth = `package auth

import m "github.com/maago-web/maago"

type User struct {
	m.Model ~maago:"table=users"~

	ID         int64
	Email      string
	FavoriteID *int64 ~maago:"fk=blog.Post"~
}
`
	const blog = `package blog

import m "github.com/maago-web/maago"

type Post struct {
	m.Model

	ID       int64
	AuthorID int64 ~maago:"fk=auth.User"~
	Title    string
}
`
	for _, dd := range []draftDialect{draftSQLite, draftPG} {
		t.Run(string(dd.d), func(t *testing.T) {
			root := dd.makeProject(t, blog)
			if err := os.Mkdir(filepath.Join(root, "auth"), 0o755); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(root, "auth", "models.go"), strings.ReplaceAll(auth, "~", "`"))
			initial := readFile(t, filepath.Join(root, makeMigrations(t, root, "")))
			if !strings.Contains(initial, `REFERENCES "users"`) || !strings.Contains(initial, `REFERENCES "blog_post"`) {
				t.Fatalf("no FK across the apps:\n%s", initial)
			}
			expect(t, root)

			r := newRealDB(t, dd.d)
			r.root = root
			r.migrate(t, "0001_initial.sql")
			db := r.open(t)
			mustExec(t, db, `INSERT INTO users (email) VALUES ('a@example.com')`)
			mustExec(t, db, `INSERT INTO blog_post (author_id, title) VALUES (1, 'hi')`)

			// A CHECK SQLite cannot add in place: it rebuilds blog_post.
			setModels(t, root, strings.Replace(blog, "Title    string", "Title    string ~maago:\"max=100\"~", 1))
			w := makeMigrations(t, root, "")
			if change := readFile(t, filepath.Join(root, w)); dd.d == project.SQLite && !strings.Contains(change, `INSERT INTO "maago_new_blog_post"`) {
				t.Fatalf("not a rebuild:\n%s", change)
			}
			expect(t, root)
			r.migrate(t, filepath.Base(w))
			if got := rowsText(t, r.open(t), "SELECT u.email || ' ' || p.title FROM blog_post p JOIN users u ON u.id = p.author_id"); got != "a@example.com hi\n" {
				t.Errorf("rows after the change: %q", got)
			}
		})
	}
}
