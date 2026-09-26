package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/dialect/sqlite"
	"github.com/maago-web/maago/internal/golden"
	"github.com/maago-web/maago/internal/practicedb"
	"github.com/maago-web/maago/internal/project"
	"github.com/maago-web/maago/internal/schema"
)

// apply runs each statement of src on db.
func apply(t *testing.T, db *practicedb.DB, src string) {
	t.Helper()
	stmts, ds := dialect.Split("", src)
	if len(ds) > 0 {
		t.Fatalf("Split:\n%s", text(ds))
	}
	for _, st := range stmts {
		mustExec(t, db, st.SQL)
	}
}

// formatSchema lays s out one line per item. Without names, names of
// indexes and constraints and whether a unique is a constraint are left
// out, as a comparison leaves them out (D-87).
func formatSchema(s *schema.Schema, names bool) string {
	var b strings.Builder
	named := func(n string) string {
		if !names || n == "" {
			return ""
		}
		return fmt.Sprintf("%q ", n)
	}
	for _, t := range s.Tables {
		fmt.Fprintf(&b, "table %q strict=%v pk=%q auto=%q\n", t.Name, t.Strict, t.PK, t.PKAuto)
		for _, c := range t.Columns {
			fmt.Fprintf(&b, "  column %q %s notnull=%v default=%q\n", c.Name, c.Type, c.NotNull, c.Default)
		}
		for _, ix := range t.Indexes {
			kind := "index"
			if ix.Unique {
				kind = "unique"
			}
			if names && ix.Constraint {
				kind += " constraint"
			}
			fmt.Fprintf(&b, "  %s %s%q\n", kind, named(ix.Name), ix.Columns)
		}
		for _, fk := range t.FKs {
			fmt.Fprintf(&b, "  fk %s%q -> %q %q delete=%s update=%s\n", named(fk.Name), fk.Columns, fk.RefTable, fk.RefColumns, fk.OnDelete, fk.OnUpdate)
		}
		for _, c := range t.Checks {
			fmt.Fprintf(&b, "  check %scolumn=%q %s\n", named(c.Name), c.Column, c.Expr)
		}
	}
	return b.String()
}

// readSQLite applies src, then each of whole, to a new practice database
// and reads it back. whole is not split: it holds statements such as
// triggers that dialect.Split cannot split.
func readSQLite(t *testing.T, src string, whole ...string) string {
	t.Helper()
	db := openPractice(t, project.SQLite, "")
	apply(t, db, src)
	for _, st := range whole {
		if st != "" {
			mustExec(t, db, st)
		}
	}
	s, ds := sqlite.Read(ctx(t), db)
	out := text(ds)
	if s != nil {
		out = formatSchema(s, true) + out
	}
	return out
}

func TestSQLiteReadExample(t *testing.T) {
	src, err := os.ReadFile("../../internal/dialect/sqlite/testdata/example.sql")
	if err != nil {
		t.Fatal(err)
	}
	golden.Check(t, "testdata/sqlite_example.golden", []byte(readSQLite(t, string(src))))
}

// The example written by hand, differently but meaning the same.
const exampleByHand = `
create table people (
  id integer primary key autoincrement,
  name text not null, parent_id integer,
  "editor_id" INTEGER NOT NULL UNIQUE,  -- a UNIQUE constraint, not an index
  reviewer_id integer, code text not null,
  "order" integer not null default (7),
  admin integer not null default 0 check ("admin" in (0,1)),
  foreign key (parent_id) references people,
  foreign key (editor_id) references people (id),
  foreign key (reviewer_id) references people
) strict;
create unique index people_code_unique on people (code);
create index p1 on people (reviewer_id);
create index p2 on people (code);
create index p3 on people (parent_id);

create table blog_post (
  id integer primary key autoincrement,
  author_id integer not null references people ( id ) on delete no action,
  title text not null check (length("title")<=200),
  slug text not null check(length("slug") <= 100),
  status text not null default 'draft' check ("status" in ('draft','published')),
  views integer not null default 0, likes integer not null default 0,
  score real default 1.5,
  public integer not null default 1 check ("public" IN (0, 1)),
  body blob not null, published_at text
) strict;
create unique index b1 on blog_post (slug);
create unique index b2 on blog_post (author_id, slug);
create index b3 on blog_post (status);
create index b4 on blog_post (author_id);
`

func TestSQLiteReadByHand(t *testing.T) {
	src, err := os.ReadFile("../../internal/dialect/sqlite/testdata/example.sql")
	if err != nil {
		t.Fatal(err)
	}
	read := func(src string) string {
		db := openPractice(t, project.SQLite, "")
		apply(t, db, src)
		s, ds := sqlite.Read(ctx(t), db)
		if len(ds) > 0 {
			t.Fatalf("Read:\n%s", text(ds))
		}
		return formatSchema(s, false)
	}
	if a, b := read(string(src)), read(exampleByHand); a != b {
		t.Errorf("read differently:\n%s\nby hand:\n%s", a, b)
	}
}

func TestSQLiteRead(t *testing.T) {
	// Statements that dialect.Split cannot split, run after src.
	whole := map[string]string{"trigger": `CREATE TRIGGER t_tr AFTER INSERT ON t BEGIN SELECT 1; END`}
	cases := []struct{ name, src string }{
		{"checks", `CREATE TABLE t (
			a TEXT CHECK (a <> '') CHECK (length(a) < 10),
			b TEXT CONSTRAINT b_choice CHECK (b IN ('x,(', 'y')) CONSTRAINT b_u UNIQUE CHECK (b <> 'z'),
			c INTEGER,
			CHECK (c > 0),
			CONSTRAINT t_ab CHECK (a <> b)
		) STRICT`},
		{"keys", `CREATE TABLE p (x INTEGER, y INTEGER, PRIMARY KEY (y, x)) STRICT;
			CREATE TABLE c (
				id INTEGER PRIMARY KEY,
				x INTEGER, y INTEGER,
				CONSTRAINT c_xy_fkey FOREIGN KEY (x, y) REFERENCES p (x, y) ON UPDATE CASCADE
			)`},
		{"foreign keys on the same column", `CREATE TABLE a (id INTEGER PRIMARY KEY);
			CREATE TABLE b (id INTEGER PRIMARY KEY);
			CREATE TABLE c (
				x INTEGER CONSTRAINT c_x_a REFERENCES a,
				CONSTRAINT c_x_b FOREIGN KEY (x) REFERENCES "b" (id)
			)`},
		{"names that are keywords", `CREATE TABLE t (conflict INTEGER, match INTEGER, UNIQUE (conflict, match)) STRICT`},
		{"added column", `CREATE TABLE t (a TEXT) STRICT;
			ALTER TABLE t ADD COLUMN c TEXT CHECK (c <> '')`},
		{"renamed column", `CREATE TABLE t (a TEXT CHECK (length(a) < 10)) STRICT;
			ALTER TABLE t RENAME COLUMN a TO b`},
		{"dropped column", `CREATE TABLE t (a TEXT, b TEXT CHECK (b <> '')) STRICT;
			ALTER TABLE t DROP COLUMN a`},

		{"partial index", `CREATE TABLE t (a INTEGER); CREATE INDEX t_a ON t (a) WHERE a > 0;
			CREATE TABLE u (a INTEGER); -- read though another table has a diagnostic
			CREATE TABLE maago_migrations (name TEXT)`},
		{"AUTOINCREMENT in a table constraint", `CREATE TABLE t (id INTEGER, PRIMARY KEY (id AUTOINCREMENT))`},
		{"descending primary key", `CREATE TABLE t (id INTEGER PRIMARY KEY DESC)`},
		{"primary key with COLLATE", `CREATE TABLE t (a TEXT, PRIMARY KEY (a COLLATE NOCASE))`},
		{"name in single quotes", `CREATE TABLE t ('order' INTEGER)`},
		{"expression index", `CREATE TABLE t (a INTEGER); CREATE INDEX t_a ON t (a + 1)`},
		{"descending index", `CREATE TABLE t (a INTEGER); CREATE INDEX t_a ON t (a DESC)`},
		{"index with COLLATE", `CREATE TABLE t (a TEXT); CREATE INDEX t_a ON t (a COLLATE NOCASE)`},
		{"column with COLLATE", `CREATE TABLE t (a TEXT COLLATE NOCASE, b TEXT CHECK (b = 'x' COLLATE NOCASE)); CREATE INDEX t_a ON t (a)`},
		{"generated column", `CREATE TABLE t (a INTEGER, b INTEGER GENERATED ALWAYS AS (a + 1))`},
		{"WITHOUT ROWID", `CREATE TABLE t (a INTEGER PRIMARY KEY) WITHOUT ROWID`},
		{"trigger", `CREATE TABLE t (a INTEGER)`},
		{"view", `CREATE TABLE t (a INTEGER); CREATE VIEW v AS SELECT a FROM t`},
		{"virtual table", `CREATE VIRTUAL TABLE v USING fts5(a)`},
		{"bracket name", `CREATE TABLE t ([order] INTEGER, b INTEGER)`},
		{"ON CONFLICT", `CREATE TABLE t (a INTEGER UNIQUE ON CONFLICT REPLACE)`},
		{"DEFERRABLE", `CREATE TABLE p (id INTEGER PRIMARY KEY); CREATE TABLE t (p_id INTEGER REFERENCES p DEFERRABLE INITIALLY DEFERRED)`},
		{"MATCH", `CREATE TABLE p (id INTEGER PRIMARY KEY); CREATE TABLE t (p_id INTEGER REFERENCES p (id) MATCH FULL)`},
		{"unknown table", `CREATE TABLE t (p_id INTEGER REFERENCES nosuch (id), b INTEGER)`},
	}
	var b strings.Builder
	for _, c := range cases {
		fmt.Fprintf(&b, "== %s\n%s", c.name, readSQLite(t, c.src, whole[c.name]))
	}
	golden.Check(t, "testdata/sqlite_cases.golden", []byte(b.String()))
}
