package main

import (
	"context"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/dialect/postgres"
	"github.com/maago-web/maago/internal/dialect/sqlite"
	"github.com/maago-web/maago/internal/golden"
	"github.com/maago-web/maago/internal/migrate"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/model/modeltest"
	"github.com/maago-web/maago/internal/practicedb"
	"github.com/maago-web/maago/internal/project"
	"github.com/maago-web/maago/internal/schema"
)

// draftBase is the models most draft cases start from; ~ stands for a
// backquote. blog_comment points at blog_post, which sorts after it.
const draftBase = `package blog

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
	Views    int32  ~maago:"default=0"~
}

type Comment struct {
	m.Model
	ID       int64
	PostID   int64  ~maago:"fk=Post"~
	ParentID *int64 ~maago:"fk=Comment"~
}

type Tag struct {
	m.Model
	ID    int64
	Label string
}
`

// edit returns draftBase with each old replaced by the new after it.
func edit(pairs ...string) string {
	s := draftBase
	for i := 0; i < len(pairs); i += 2 {
		if !strings.Contains(s, pairs[i]) {
			panic("edit: no " + pairs[i])
		}
		s = strings.Replace(s, pairs[i], pairs[i+1], 1)
	}
	return s
}

const titleLine = `	Title    string ~maago:"max=200"~` + "\n"

// draftCases have the models before and after; the hand SQL of a
// dialect then changes the database of before in ways models cannot.
var draftCases = []struct {
	name, before, after string
	sqlite, pg          string
}{
	{name: "initial", after: draftBase},
	{name: "add nullable field", before: draftBase, after: edit(titleLine, titleLine+"	Summary *string\n")},
	{name: "add field without default", before: draftBase, after: edit(titleLine, titleLine+"	Body string\n")},
	{name: "add version field", before: draftBase, after: edit(`import m "github.com/maago-web/maago"`, "import (\n\t\"time\"\n\n\tm \"github.com/maago-web/maago\"\n)",
		titleLine, titleLine+`	UpdatedAt time.Time ~maago:"auto=update"~`+"\n")},
	{name: "add field with max and default", before: draftBase, after: edit(titleLine, titleLine+`	Lang string ~maago:"max=5,default='ko'"~`+"\n")},
	{name: "add foreign key field", before: draftBase, after: edit(titleLine, titleLine+`	EditorID *int64 ~maago:"fk=Author"~`+"\n")},
	{name: "add field with choices", before: draftBase, after: edit(titleLine, titleLine+`	Status string ~maago:"choices=draft/published,default='draft'"~`+"\n")},
	{name: "add fields with unusual defaults", before: draftBase, after: edit(titleLine, titleLine+`	Ratio float64 ~maago:"default=0.0000001"~
	Delta int32 ~maago:"default=-3"~
`)},
	{name: "drop field", before: draftBase, after: edit(`	Views    int32  ~maago:"default=0"~`+"\n", "")},
	{name: "drop foreign key field", before: draftBase, after: edit(`	AuthorID int64  ~maago:"fk=Author"~`+"\n", "")},
	{name: "type", before: draftBase, after: edit(`Views    int32 `, `Views    float64`)},
	{
		name:   "type and default",
		before: edit(titleLine, titleLine+`	Code string ~maago:"default='0'"~`+"\n"),
		after:  edit(titleLine, titleLine+`	Code int64 ~maago:"default=0"~`+"\n"),
	},
	{name: "max", before: draftBase, after: edit(`max=200`, `max=100`)},
	{name: "nullable", before: draftBase, after: edit(`Title    string `, `Title    *string`)},
	{name: "default", before: draftBase, after: edit(`default=0`, `default=1`)},
	{name: "add unique", before: draftBase, after: edit("	Label string\n", `	Label string ~maago:"unique"~`+"\n")},
	{name: "drop unique", before: edit("	Label string\n", `	Label string ~maago:"unique"~`+"\n"), after: draftBase},
	{
		name:   "add choices",
		before: edit(titleLine, titleLine+`	Status string ~maago:"default='draft'"~`+"\n"),
		after:  edit(titleLine, titleLine+`	Status string ~maago:"choices=draft/published,default='draft'"~`+"\n"),
	},
	{name: "drop model", before: draftBase, after: edit(draftBase[strings.Index(draftBase, "type Comment"):strings.Index(draftBase, "type Tag")], "")},
	{
		name: "by hand", before: draftBase, after: draftBase,
		// Not STRICT, no AUTOINCREMENT, a UNIQUE constraint and a named
		// table CHECK.
		sqlite: `DROP TABLE blog_tag;
CREATE TABLE blog_tag (id INTEGER PRIMARY KEY, label TEXT NOT NULL UNIQUE, CONSTRAINT label_set CHECK (length(label) > 0));`,
		// A key on more columns, an identity that is ALWAYS, one
		// replaced by a serial default, and a cascading foreign key.
		pg: `ALTER TABLE blog_tag DROP CONSTRAINT blog_tag_pkey;
ALTER TABLE blog_tag ADD CONSTRAINT tag_key PRIMARY KEY (id, label);
ALTER TABLE blog_author ALTER COLUMN id SET GENERATED ALWAYS;
ALTER TABLE blog_comment ALTER COLUMN id DROP IDENTITY;
CREATE SEQUENCE comment_seq OWNED BY blog_comment.id;
ALTER TABLE blog_comment ALTER COLUMN id SET DEFAULT nextval('comment_seq');
ALTER TABLE blog_post DROP CONSTRAINT blog_post_author_id_fkey;
ALTER TABLE blog_post ADD CONSTRAINT blog_post_author_id_fkey FOREIGN KEY (author_id) REFERENCES blog_author (id) ON DELETE CASCADE;`,
	},
	{
		name: "unique constraint", before: draftBase, after: draftBase,
		// Only a UNIQUE constraint differs, which DROP INDEX cannot drop.
		sqlite: `DROP TABLE blog_tag;
CREATE TABLE blog_tag (id INTEGER PRIMARY KEY AUTOINCREMENT, label TEXT NOT NULL UNIQUE) STRICT;`,
		pg: `ALTER TABLE blog_tag ADD CONSTRAINT tag_label_key UNIQUE (label);`,
	},
}

type draftDialect struct {
	d     project.Dialect
	ddl   func(*token.FileSet, []*model.App) (string, []diag.Diagnostic)
	read  func(context.Context, dialect.Querier) (*schema.Schema, []diag.Diagnostic)
	draft func([]*model.App, []migrate.Change) string
}

var (
	draftSQLite = draftDialect{project.SQLite, sqlite.DDL, sqlite.Read, sqlite.Draft}
	draftPG     = draftDialect{project.Postgres, postgres.DDL, postgres.Read, postgres.Draft}
)

// models loads src, with ~ for backquotes, as a project; "" is none.
func models(t *testing.T, src string) ([]*model.App, *token.FileSet) {
	t.Helper()
	if src == "" {
		return nil, token.NewFileSet()
	}
	return modeltest.Example(t, modelProject(t, src))
}

// modelProject writes a project whose blog app holds src, with ~ for
// backquotes, and returns its root.
func modelProject(t *testing.T, src string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "blog"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "blog", "models.go"), []byte(strings.ReplaceAll(src, "~", "`")), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func (dd draftDialect) schema(t *testing.T, db *practicedb.DB) *schema.Schema {
	t.Helper()
	s, ds := dd.read(ctx(t), db)
	if len(ds) > 0 {
		t.Fatalf("read:\n%s", text(ds))
	}
	return s
}

// open returns a practice database holding the DDL of src, and its models.
func (dd draftDialect) open(t *testing.T, url, src string) (*practicedb.DB, []*model.App) {
	t.Helper()
	db := openPractice(t, dd.d, url)
	apps, fset := models(t, src)
	if apps != nil {
		ddl, ds := dd.ddl(fset, apps)
		if len(ds) > 0 {
			t.Fatalf("DDL:\n%s", text(ds))
		}
		apply(t, db, ddl)
	}
	return db, apps
}

// applyDraft applies draft to db as a migration would be: in SQLite with
// foreign keys off around it and checked before they are back on.
func (dd draftDialect) applyDraft(t *testing.T, db *practicedb.DB, draft string) {
	t.Helper()
	if dd.d == project.SQLite {
		mustExec(t, db, "PRAGMA foreign_keys = OFF")
	}
	apply(t, db, draft)
	if dd.d == project.SQLite {
		rows, err := db.QueryContext(ctx(t), "PRAGMA foreign_key_check")
		if err != nil {
			t.Fatal(err)
		}
		if rows.Next() {
			t.Error("foreign_key_check found rows")
		}
		rows.Close()
		mustExec(t, db, "PRAGMA foreign_keys = ON")
	}
}

// run writes the draft of each case to golden, and checks that applying
// it leaves nothing between the migrations and the models.
func (dd draftDialect) run(t *testing.T, url, goldenFile string) {
	var out strings.Builder
	for _, tc := range draftCases {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := dd.open(t, url, tc.before)
			hand := tc.sqlite
			if dd.d == project.Postgres {
				hand = tc.pg
			}
			apply(t, a, hand)
			b, apps := dd.open(t, url, tc.after)
			want := dd.schema(t, b)
			draft := dd.draft(apps, migrate.Diff(dd.schema(t, a), want))
			out.WriteString("-- " + tc.name + "\n\n" + draft + "\n")
			dd.applyDraft(t, a, draft)
			if cs := migrate.Diff(dd.schema(t, a), want); len(cs) > 0 {
				var msgs []string
				for _, c := range cs {
					msgs = append(msgs, c.Message())
				}
				t.Errorf("after the draft:\n%s\n\ndraft:\n%s", strings.Join(msgs, "\n"), draft)
			}
		})
	}
	golden.Check(t, goldenFile, []byte(out.String()))
}

func TestDraftSQLite(t *testing.T) {
	draftSQLite.run(t, "", "testdata/draft_sqlite.golden")
}

func TestDraftPG(t *testing.T) {
	draftPG.run(t, pgURL(t), "testdata/draft_pg.golden")
}

// A rebuild keeps the rows, the foreign keys pointing at them, a table's
// foreign key to itself, and the AUTOINCREMENT counter.
func TestDraftSQLiteRebuildKeepsRows(t *testing.T) {
	a, _ := draftSQLite.open(t, "", draftBase)
	mustExec(t, a,
		`INSERT INTO blog_author (id, name) VALUES (1, 'a'), (2, 'b'), (3, 'c')`,
		`DELETE FROM blog_author WHERE id = 3`,
		`INSERT INTO blog_post (id, author_id, title) VALUES (1, 2, 't')`,
		`INSERT INTO blog_comment (id, post_id, parent_id) VALUES (1, 1, NULL), (2, 1, 1)`,
	)
	// Both tables need a rebuild: a changed CHECK and a changed NOT NULL.
	after := edit("	Name string\n", `	Name string ~maago:"max=50"~`+"\n", "PostID   int64 ", "PostID   *int64")
	b, apps := draftSQLite.open(t, "", after)
	draft := sqlite.Draft(apps, migrate.Diff(draftSQLite.schema(t, a), draftSQLite.schema(t, b)))
	for _, table := range []string{"blog_author", "blog_comment"} {
		if !strings.Contains(draft, `RENAME TO "`+table+`"`) {
			t.Fatalf("%s is not rebuilt:\n%s", table, draft)
		}
	}
	draftSQLite.applyDraft(t, a, draft)
	mustExec(t, a, `INSERT INTO blog_author (name) VALUES ('d')`)
	got := rowsText(t, a, `SELECT 'author ' || id || ' ' || name FROM blog_author
		UNION ALL SELECT 'comment ' || id || ' ' || coalesce(parent_id, 0) || ' ' || post_id FROM blog_comment ORDER BY 1`)
	want := "author 1 a\nauthor 2 b\nauthor 4 d\ncomment 1 0 1\ncomment 2 1 1\n"
	if got != want {
		t.Errorf("rows after the rebuild:\n%s\nwant\n%s", got, want)
	}
}

// rowsText lays out the rows of query, which has one text column.
func rowsText(t *testing.T, db *practicedb.DB, query string) string {
	t.Helper()
	rows, err := db.QueryContext(ctx(t), query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		b.WriteString(line + "\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// Making max smaller fails on a longer value rather than cutting it.
func TestDraftPGRefusesLongValues(t *testing.T) {
	url := pgURL(t)
	a, _ := draftPG.open(t, url, draftBase)
	mustExec(t, a, `INSERT INTO blog_author (id, name) VALUES (1, 'a')`,
		`INSERT INTO blog_post (author_id, title) VALUES (1, repeat('x', 150))`)
	after := edit(`max=200`, `max=100`)
	b, apps := draftPG.open(t, url, after)
	draft := postgres.Draft(apps, migrate.Diff(draftPG.schema(t, a), draftPG.schema(t, b)))
	stmts, _ := dialect.Split("", draft)
	var errs []diag.Diagnostic
	for _, st := range stmts {
		errs = append(errs, a.Exec(ctx(t), st.Pos, st.SQL)...)
	}
	if got := text(errs); !strings.Contains(got, "value too long") {
		t.Errorf("the draft did not refuse a long value: %q\ndraft:\n%s", got, draft)
	}
}
