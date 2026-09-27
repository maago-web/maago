package main

import (
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/project"
)

// exampleCase checks that M1's example schema accepts or refuses one
// thing, for the reason it should: its statements run one by one on a
// fresh practice database, and the last fails with want, or succeeds and
// then cond, a query, reads 'ok'. Each case has its own database, since
// in PostgreSQL a failed statement spoils the transaction.
type exampleCase struct {
	name  string
	stmts []string
	want  string // part of the error of the last statement; "" if none
	cond  string
}

func (c exampleCase) run(t *testing.T, d project.Dialect, url, example string) {
	t.Run(c.name, func(t *testing.T) {
		db := openPractice(t, d, url)
		apply(t, db, example)
		if n := len(c.stmts); n > 0 {
			mustExec(t, db, c.stmts[:n-1]...)
			last := c.stmts[n-1]
			if out := exec(t, db, last); c.want == "" && out != "" || !strings.Contains(out, c.want) {
				t.Fatalf("%s: got %q, want %q", last, out, c.want)
			}
		}
		if c.cond != "" {
			if got := rowsText(t, db, c.cond); got != "ok\n" {
				t.Errorf("%s: got %q", c.cond, got)
			}
		}
	})
}

// The cases of M1's SQLite smoke script, moved here to run in CI.
func TestExampleSQLite(t *testing.T) {
	example := readFile(t, "../../internal/dialect/sqlite/testdata/example.sql")
	author := "INSERT INTO people (name, editor_id, code, created_at, updated_at) VALUES ('a', 1, 'c1', '2026-09-27 00:00:00.000000', '2026-09-27 00:00:00.000000')"
	post := "INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, 't', 's', x'00')"
	postWith := func(col, val string) string {
		return "INSERT INTO blog_post (author_id, title, slug, body, " + col + ") VALUES (1, 't', 's', x'00', " + val + ")"
	}
	for _, c := range []exampleCase{
		{name: "schema applies"},
		{name: "NULL id is generated", stmts: []string{author}, cond: "SELECT CASE WHEN id = 1 THEN 'ok' ELSE 'no' END FROM people"},
		{name: "defaults fill in", stmts: []string{author, post},
			cond: "SELECT CASE WHEN status = 'draft' AND public = 1 AND views = 0 AND score = 1.5 AND (SELECT admin FROM people) = 0 THEN 'ok' ELSE 'no' END FROM blog_post"},
		{name: "FK to a missing author", stmts: []string{post}, want: "FOREIGN KEY constraint failed"},
		{name: "choice outside the list", stmts: []string{author, postWith("status", "'x'")}, want: "CHECK constraint failed: status"},
		{name: "title over 200 characters", stmts: []string{author, "INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, printf('%.201c', 'a'), 's', x'00')"},
			want: `CHECK constraint failed: length("title") <= 200`},
		{name: "200 Korean characters fit", stmts: []string{author, "INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, replace(printf('%.200c', 'x'), 'x', '가'), 's', x'00')"}},
		{name: "text in an integer (STRICT)", stmts: []string{author, postWith("views", "'abc'")}, want: "cannot store TEXT value in INTEGER column blog_post.views"},
		{name: "bool other than 0 or 1", stmts: []string{author, postWith("public", "2")}, want: "CHECK constraint failed: public"},
		{name: "NULL passes CHECK", stmts: []string{author, postWith("score", "NULL")}},
		{name: "duplicate slug", stmts: []string{author, post, "INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, 't2', 's', x'00')"}, want: "UNIQUE constraint failed"},
		{name: "delete a referenced author", stmts: []string{author, post, "DELETE FROM people"}, want: "FOREIGN KEY constraint failed"},
		{name: "NULL into a NOT NULL column", stmts: []string{author, "INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, NULL, 's', x'00')"},
			want: "NOT NULL constraint failed: blog_post.title"},
		{name: "composite unique and indexes exist",
			cond: "SELECT CASE WHEN count(*) = 9 THEN 'ok' ELSE 'no' END FROM sqlite_master WHERE type = 'index' AND name IN ('blog_post_author_id_slug_key', 'blog_post_slug_key', 'blog_post_status_idx', 'blog_post_author_id_idx', 'people_editor_id_key', 'people_code_key', 'people_reviewer_id_idx', 'people_code_idx', 'people_parent_id_idx')"},
		{name: "ids are not reused", stmts: []string{author, "INSERT INTO people (name, editor_id, code, created_at, updated_at) VALUES ('b', 2, 'c2', '2026-09-27 00:00:00.000000', '2026-09-27 00:00:00.000000')", "DELETE FROM people WHERE id = 2",
			"INSERT INTO people (name, editor_id, code, created_at, updated_at) VALUES ('c', 3, 'c3', '2026-09-27 00:00:00.000000', '2026-09-27 00:00:00.000000')"}, cond: "SELECT CASE WHEN max(id) = 3 THEN 'ok' ELSE 'no' END FROM people"},
	} {
		c.run(t, project.SQLite, "", example)
	}
}

// The cases of M1's PostgreSQL smoke script, moved here to run in CI on
// a practice schema instead of a throwaway cluster.
func TestExamplePG(t *testing.T) {
	url := pgURL(t)
	example := readFile(t, "../../internal/dialect/postgres/testdata/example.sql")
	author := "INSERT INTO people (name, editor_id, code, created_at, updated_at) VALUES ('a', 1, 'c1', now(), now())"
	post := `INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, 't', 's', '\x00')`
	postWith := func(col, val string) string {
		return `INSERT INTO blog_post (author_id, title, slug, body, ` + col + `) VALUES (1, 't', 's', '\x00', ` + val + `)`
	}
	for _, c := range []exampleCase{
		{name: "schema applies"},
		{name: "id is generated", stmts: []string{author}, cond: "SELECT CASE WHEN id = 1 THEN 'ok' ELSE 'no' END FROM people"},
		{name: "defaults fill in", stmts: []string{author, post},
			cond: `SELECT CASE WHEN status = 'draft' AND public AND views = 0 AND score = 1.5 AND (SELECT NOT admin AND "order" = 7 FROM people) THEN 'ok' ELSE 'no' END FROM blog_post`},
		{name: "FK to a missing author", stmts: []string{post}, want: `violates foreign key constraint "blog_post_author_id_fkey"`},
		{name: "choice outside the list", stmts: []string{author, postWith("status", "'x'")}, want: `violates check constraint "blog_post_status_check"`},
		{name: "title over 200 characters", stmts: []string{author, `INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, repeat('a', 201), 's', '\x00')`},
			want: "value too long for type character varying(200)"},
		{name: "200 Korean characters fit", stmts: []string{author, `INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, repeat('가', 200), 's', '\x00')`}},
		{name: "text in an integer", stmts: []string{author, postWith("views", "'abc'")}, want: "invalid input syntax for type integer"},
		{name: "integer in a bool", stmts: []string{author, postWith("public", "2")}, want: "is of type boolean but expression is of type integer"},
		{name: "NULL where allowed", stmts: []string{author, postWith("score", "NULL")}},
		{name: "NULL into a NOT NULL column", stmts: []string{author, `INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, NULL, 's', '\x00')`},
			want: `null value in column "title"`},
		{name: "duplicate slug", stmts: []string{author, post, `INSERT INTO blog_post (author_id, title, slug, body) VALUES (1, 't2', 's', '\x00')`},
			want: `violates unique constraint "blog_post_slug_key"`},
		{name: "delete a referenced author", stmts: []string{author, post, "DELETE FROM people"}, want: `violates foreign key constraint "blog_post_author_id_fkey"`},
		// Only in the practice schema, not wherever else the names are.
		{name: "composite unique and indexes exist",
			cond: "SELECT CASE WHEN (SELECT count(*) FROM pg_constraint WHERE conname = 'blog_post_author_id_slug_key' AND connamespace = current_schema()::regnamespace) = 1 " +
				"AND (SELECT count(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname IN ('blog_post_status_idx', 'blog_post_author_id_idx', 'people_parent_id_idx', 'people_reviewer_id_idx', 'people_code_idx')) = 5 THEN 'ok' ELSE 'no' END"},
		{name: "an explicit id is accepted (BY DEFAULT)", stmts: []string{"INSERT INTO people (id, name, editor_id, code, created_at, updated_at) VALUES (10, 'a', 10, 'c1', now(), now())"}},
		{name: "ids are not reused", stmts: []string{author, "INSERT INTO people (name, editor_id, code, created_at, updated_at) VALUES ('b', 2, 'c2', now(), now())", "DELETE FROM people WHERE id = 2",
			"INSERT INTO people (name, editor_id, code, created_at, updated_at) VALUES ('c', 3, 'c3', now(), now())"}, cond: "SELECT CASE WHEN max(id) = 3 THEN 'ok' ELSE 'no' END FROM people"},
	} {
		c.run(t, project.Postgres, url, example)
	}
}
