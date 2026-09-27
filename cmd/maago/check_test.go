package main

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/build"
	"github.com/maago-web/maago/internal/dialect/sqlite"
	"github.com/maago-web/maago/internal/migrate"
	"github.com/maago-web/maago/internal/model/modeltest"
	"github.com/maago-web/maago/internal/project"
)

// checkBase is the models of the check tests. The line of each field is
// what the diagnostics point at: Author 5 (column 6), its ID 7, Name 8; Post 11, its
// ID 13, AuthorID 14, Title 15, Status 16, Views 17.
const checkBase = `package blog

import m "github.com/maago-web/maago"

type Author struct {
	m.Model
	ID   int64
	Name string ~maago:"max=50,unique"~
}

type Post struct {
	m.Model  ~maago:"index=(Title,Views)"~
	ID       int64
	AuthorID int64  ~maago:"fk=Author"~
	Title    string ~maago:"max=200"~
	Status   string ~maago:"choices=draft/published,default='draft'"~
	Views    int32  ~maago:"default=0"~
}
`

// checkProject writes a project of checkBase for dd, whose first
// migration is the models' DDL with each old replaced by the new after
// it, and more migrations by name.
func (dd draftDialect) checkProject(t *testing.T, edits []string, more map[string]string) string {
	t.Helper()
	env := "sqlite:app.db"
	if dd.d == project.Postgres {
		env = pgURL(t)
	}
	root := modelProject(t, checkBase)
	apps, fset := modeltest.Example(t, root)
	ddl, ds := dd.ddl(fset, apps)
	if len(ds) > 0 {
		t.Fatal(text(ds))
	}
	for i := 0; i < len(edits); i += 2 {
		if !strings.Contains(ddl, edits[i]) {
			t.Fatalf("the DDL has no %q:\n%s", edits[i], ddl)
		}
		ddl = strings.Replace(ddl, edits[i], edits[i+1], 1)
	}
	writeFile(t, filepath.Join(root, "migrations", "0001_initial.sql"), ddl)
	for name, src := range more {
		writeFile(t, filepath.Join(root, "migrations", name), src)
	}
	t.Setenv("MAAGO_DATABASE_URL", env)
	return root
}

// expect checks that check on root, after gen, reports exactly want: each
// is the start of a diagnostic, position included, in any order. Gen
// writes models_gen.go only if nothing is wrong, and then check passes
// but for the database.
func expect(t *testing.T, root string, want ...string) {
	t.Helper()
	build.Gen(root)
	out := text(build.Check(root))
	lines := diagnostics(out)
	missing := false
	for _, w := range want {
		if i := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, w) }); i >= 0 {
			lines = slices.Delete(lines, i, i+1)
		} else {
			missing = true
		}
	}
	if missing || len(lines) > 0 {
		t.Errorf("got:\n%s\nwant:\n%s", out, strings.Join(want, "\n"))
	}
}

const differ = "migrations do not match models: "

// M2 completion condition 2: a draft changed by hand in one place fails
// check, saying what differs and where the models have it.
func TestCheckEditedDraft(t *testing.T) {
	statusLine := `    "status" TEXT NOT NULL DEFAULT 'draft' CHECK ("status" IN ('draft', 'published')),` + "\n"
	status := `blog/models.go:16:2: ` + differ
	views := `blog/models.go:17:2: ` + differ
	authorID := `blog/models.go:14:2: ` + differ
	title := `blog/models.go:15:2: ` + differ
	author := `blog/models.go:5:6: ` + differ // a key is the model's
	type edit struct {
		name  string
		edits []string
		want  []string
	}
	shared := func(pk, pkNoAuto, fkLine string) []edit {
		return []edit{
			{"no column", []string{statusLine, ""}, []string{status + `column "status" of "blog_post" is missing`}},
			{"extra choice", []string{`('draft', 'published')`, `('draft', 'published', 'x')`}, []string{status + `check (`, status + `check (`}},
			{"no foreign key", []string{fkLine, ""}, []string{authorID + `foreign key ("author_id") -> "blog_author" ("id") ON DELETE NO ACTION of "blog_post" is missing`}},
			{"on delete cascade", []string{`ON DELETE NO ACTION`, `ON DELETE CASCADE`}, []string{
				authorID + `foreign key ("author_id") -> "blog_author" ("id") ON DELETE CASCADE of "blog_post" is not in the models`,
				authorID + `foreign key ("author_id") -> "blog_author" ("id") ON DELETE NO ACTION of "blog_post" is missing`,
			}},
			{"index order", []string{`("title", "views")`, `("views", "title")`}, []string{
				views + `index ("views", "title") of "blog_post" is not in the models`,
				title + `index ("title", "views") of "blog_post" is missing`,
			}},
			{"no autoincrement", []string{pk, pkNoAuto}, []string{
				author + `primary key ("id") of "blog_author" is not in the models`,
				author + `primary key ("id") `,
			}},
		}
	}
	sqliteEdits := shared(`"id" INTEGER PRIMARY KEY AUTOINCREMENT,`+"\n"+`    "name"`, `"id" INTEGER PRIMARY KEY,`+"\n"+`    "name"`,
		`,`+"\n"+`    CONSTRAINT "blog_post_author_id_fkey" FOREIGN KEY ("author_id") REFERENCES "blog_author" ("id") ON DELETE NO ACTION`)
	sqliteEdits = append(sqliteEdits,
		edit{"not null", []string{`"author_id" INTEGER NOT NULL,`, `"author_id" INTEGER,`}, []string{authorID + `column "author_id" of "blog_post" is NULL, the models have NOT NULL`}},
		edit{"type", []string{`"views" INTEGER NOT NULL DEFAULT 0`, `"views" REAL NOT NULL DEFAULT 0`}, []string{views + `column "views" of "blog_post" is REAL, the models have INTEGER`}},
		edit{"default", []string{`"views" INTEGER NOT NULL DEFAULT 0`, `"views" INTEGER NOT NULL DEFAULT 1`}, []string{views + `column "views" of "blog_post" has DEFAULT 1, the models have DEFAULT 0`}},
		edit{"no max", []string{` CHECK (length("title") <= 200)`, ``}, []string{title + `check (LENGTH ("title") <= 200) of "blog_post" is missing`}},
		edit{"no unique", []string{`CREATE UNIQUE INDEX "blog_author_name_key" ON "blog_author" ("name");`, ``}, []string{`blog/models.go:8:2: ` + differ + `unique index ("name") of "blog_author" is missing`}},
	)
	pgEdits := shared(`"id" BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,`+"\n"+`    "name"`, `"id" BIGINT PRIMARY KEY,`+"\n"+`    "name"`,
		`ALTER TABLE "blog_post" ADD CONSTRAINT "blog_post_author_id_fkey" FOREIGN KEY ("author_id") REFERENCES "blog_author" ("id") ON DELETE NO ACTION;`)
	pgEdits = append(pgEdits,
		edit{"not null", []string{`"author_id" BIGINT NOT NULL,`, `"author_id" BIGINT,`}, []string{authorID + `column "author_id" of "blog_post" is NULL, the models have NOT NULL`}},
		edit{"type", []string{`"views" INTEGER NOT NULL DEFAULT 0`, `"views" BIGINT NOT NULL DEFAULT 0`}, []string{views + `column "views" of "blog_post" is BIGINT, the models have INTEGER`}},
		edit{"default", []string{`"views" INTEGER NOT NULL DEFAULT 0`, `"views" INTEGER NOT NULL DEFAULT 1`}, []string{views + `column "views" of "blog_post" has DEFAULT 1, the models have DEFAULT 0`}},
		edit{"no max", []string{`"title" VARCHAR(200)`, `"title" TEXT`}, []string{title + `column "title" of "blog_post" is TEXT, the models have CHARACTER VARYING(200)`}},
		edit{"no unique", []string{`,` + "\n" + `    CONSTRAINT "blog_author_name_key" UNIQUE ("name")`, ``}, []string{`blog/models.go:8:2: ` + differ + `unique index ("name") of "blog_author" is missing`}},
	)
	for _, tc := range []struct {
		dd    draftDialect
		edits []edit
	}{{draftSQLite, sqliteEdits}, {draftPG, pgEdits}} {
		t.Run(string(tc.dd.d), func(t *testing.T) {
			t.Run("unchanged", func(t *testing.T) { expect(t, tc.dd.checkProject(t, nil, nil)) })
			for _, e := range tc.edits {
				t.Run(e.name, func(t *testing.T) { expect(t, tc.dd.checkProject(t, e.edits, nil), e.want...) })
			}
		})
	}
}

// M2 completion condition 3: a DROP needs its marker.
func TestCheckDrops(t *testing.T) {
	junk := []string{"\nCREATE INDEX", "\nCREATE TABLE junk (x INTEGER);\n\nCREATE TABLE junk2 (x INTEGER);\n\nCREATE INDEX"}
	for _, dd := range []draftDialect{draftSQLite, draftPG} {
		t.Run(string(dd.d), func(t *testing.T) {
			run := func(name, src string, want ...string) {
				t.Run(name, func(t *testing.T) {
					expect(t, dd.checkProject(t, junk, map[string]string{"0002_drop.sql": src}), want...)
				})
			}
			run("marked", "-- maago: allow-drop\nDROP TABLE junk;\n-- maago: allow-drop\nDROP TABLE junk2;\n")
			run("unmarked", "DROP TABLE junk;\n-- maago: allow-drop\nDROP TABLE junk2;\n", "migrations/0002_drop.sql:1:1: DROP TABLE deletes data")
			run("marker elsewhere", "-- maago: allow-drop\nCREATE INDEX junk_x ON junk (x);\nDROP TABLE junk;\n-- maago: allow-drop\nDROP TABLE junk2;\n",
				"migrations/0002_drop.sql:1:1: -- maago: allow-drop is not before", "migrations/0002_drop.sql:3:1: DROP TABLE deletes data")
			run("column", "-- maago: allow-drop\nDROP TABLE junk;\n-- maago: allow-drop\nDROP TABLE junk2;\nALTER TABLE blog_post DROP COLUMN views;\n",
				"migrations/0002_drop.sql:5:1: ALTER TABLE … DROP COLUMN deletes data")
		})
	}
}

// A SQLite rebuild's DROP TABLE needs no marker, unless its copy leaves a
// column behind (D-93).
func TestCheckRebuild(t *testing.T) {
	rebuild := func(insert string) map[string]string {
		return map[string]string{"0002_rebuild.sql": `CREATE TABLE "maago_new_blog_author" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "name" TEXT NOT NULL CHECK (length("name") <= 50)
) STRICT;
` + insert + `
DROP TABLE "blog_author";
ALTER TABLE "maago_new_blog_author" RENAME TO "blog_author";
CREATE UNIQUE INDEX "blog_author_name_key" ON "blog_author" ("name");
`}
	}
	t.Run("copies all", func(t *testing.T) {
		expect(t, draftSQLite.checkProject(t, nil, rebuild(`INSERT INTO "maago_new_blog_author" ("id", "name") SELECT "id", "name" FROM "blog_author";`)))
	})
	t.Run("leaves one", func(t *testing.T) {
		expect(t, draftSQLite.checkProject(t, nil, rebuild(`INSERT INTO "maago_new_blog_author" ("id") SELECT "id" FROM "blog_author";`)),
			`migrations/0002_rebuild.sql:6:1: DROP TABLE "blog_author" loses the values of "name"`)
	})
	t.Run("selects a constant as one", func(t *testing.T) {
		expect(t, draftSQLite.checkProject(t, nil, rebuild(`INSERT INTO "maago_new_blog_author" ("id", "name") SELECT "id", '' AS "name" FROM "blog_author";`)),
			`migrations/0002_rebuild.sql:6:1: DROP TABLE "blog_author" loses the values of "name"`)
	})
	t.Run("renames one", func(t *testing.T) {
		// The old table calls it nickname; the copy selects it by that name.
		old := []string{`"name" TEXT NOT NULL CHECK (length("name") <= 50)`, `"nickname" TEXT NOT NULL`, `ON "blog_author" ("name")`, `ON "blog_author" ("nickname")`}
		expect(t, draftSQLite.checkProject(t, old, rebuild(`INSERT INTO "maago_new_blog_author" ("id", "name") SELECT "id", "nickname" FROM "blog_author";`)))
	})
}

// A statement the database refuses points at its line, and PostgreSQL's
// at its column; migration numbers and .env are read from the root.
func TestCheckFiles(t *testing.T) {
	t.Run("syntax sqlite", func(t *testing.T) {
		expect(t, draftSQLite.checkProject(t, nil, map[string]string{"0002_bad.sql": "\n\nCREATE TABLE x (a int,, b int);\n"}),
			`migrations/0002_bad.sql:3:1: `)
	})
	t.Run("syntax postgres", func(t *testing.T) {
		expect(t, draftPG.checkProject(t, nil, map[string]string{"0002_bad.sql": "\n\nCREATE TABLE x (a int,, b int);\n"}),
			`migrations/0002_bad.sql:3:23: `)
	})
	t.Run("syntax postgres, later line", func(t *testing.T) {
		expect(t, draftPG.checkProject(t, nil, map[string]string{"0002_bad.sql": "CREATE TABLE x (\n  a int,\n  b int,,\n  c int\n);\n"}),
			`migrations/0002_bad.sql:3:9: `)
	})
	t.Run("syntax postgres, after Korean", func(t *testing.T) {
		// PostgreSQL counts characters; columns count bytes: 한글 is two
		// characters and six bytes.
		expect(t, draftPG.checkProject(t, nil, map[string]string{"0002_bad.sql": "CREATE TABLE x (a TEXT DEFAULT '한글',, b int);\n"}),
			`migrations/0002_bad.sql:1:41: `)
	})
	t.Run("foreign key pointing nowhere", func(t *testing.T) {
		expect(t, draftSQLite.checkProject(t, nil, map[string]string{"0002_data.sql": "INSERT INTO blog_post (author_id, title) VALUES (99, 't');\n"}),
			`migrations/0002_data.sql: after this migration, a row of "blog_post" has a foreign key that points nowhere`)
	})
	t.Run("numbers", func(t *testing.T) {
		expect(t, draftSQLite.checkProject(t, nil, map[string]string{"0003_a.sql": "SELECT 1;", "0004_b.sql": "SELECT 1;", "0004_c.sql": "SELECT 1;"}),
			"migrations/0003_a.sql: migration 0002 is missing",
			"migrations/0004_b.sql: migration number 0004 is also used by migrations/0004_c.sql",
			"migrations/0004_c.sql: migration number 0004 is also used by migrations/0004_b.sql")
	})
	t.Run("env from the root", func(t *testing.T) {
		root := draftSQLite.checkProject(t, nil, nil)
		noEnv(t)
		writeFile(t, filepath.Join(root, ".env"), "MAAGO_DATABASE_URL=sqlite:app.db\n")
		expect(t, filepath.Join(root, "blog"))
		writeFile(t, filepath.Join(root, ".env"), "MAAGO_DATABASE_URL=mysql://x\n")
		before := snapshot(t, root)
		if _, ds := build.Gen(filepath.Join(root, "blog")); !strings.Contains(text(ds), ".env:1:") {
			t.Errorf("gen with a bad .env:\n%s", text(ds))
		}
		if !maps.Equal(before, snapshot(t, root)) {
			t.Error("gen wrote files despite a bad .env")
		}
	})
	t.Run("models changed", func(t *testing.T) {
		root := draftSQLite.checkProject(t, nil, nil)
		models := filepath.Join(root, "blog", "models.go")
		writeFile(t, models, strings.Replace(readFile(t, models), "	Views ", "	Body     string\n	Views ", 1))
		before := snapshot(t, root)
		_, ds := build.Gen(root)
		if out := text(ds); len(ds) != 1 || !strings.Contains(out, `column "body" of "blog_post" is missing`) || !strings.Contains(out, "fix: run maago makemigrations") {
			t.Errorf("gen:\n%s", out)
		}
		if !maps.Equal(before, snapshot(t, root)) {
			t.Error("gen wrote files despite the difference")
		}
	})
}

// The draft of task 7 passes check as it is: its rebuild is the shape the
// check exempts, sqlite_sequence statements and all.
func TestCheckDraft(t *testing.T) {
	root := draftSQLite.checkProject(t, nil, nil)
	models := filepath.Join(root, "blog", "models.go")
	writeFile(t, models, strings.Replace(readFile(t, models), "Views    int32 ", "Views    float64", 1))
	a, _ := draftSQLite.open(t, "", checkBase)
	b, apps := draftSQLite.open(t, "", strings.Replace(checkBase, "Views    int32 ", "Views    float64", 1))
	draft := sqlite.Draft(apps, migrate.Diff(draftSQLite.schema(t, a), draftSQLite.schema(t, b)))
	if !strings.Contains(draft, "sqlite_sequence") {
		t.Fatalf("not a rebuild:\n%s", draft)
	}
	writeFile(t, filepath.Join(root, "migrations", "0002_views.sql"), draft)
	expect(t, root)
}
