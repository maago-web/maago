package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/maago-web/maago/internal/practicedb"
	"github.com/maago-web/maago/internal/project"
)

// describeDB opens a practice database of d with two tables.
func describeDB(t *testing.T, d project.Dialect) *practicedb.DB {
	t.Helper()
	url := ""
	if d == project.Postgres {
		url = pgURL(t)
	}
	db := openPractice(t, d, url)
	if d == project.SQLite {
		mustExec(t, db,
			"CREATE TABLE boards (id INTEGER PRIMARY KEY, name TEXT NOT NULL)",
			"CREATE TABLE posts (id INTEGER PRIMARY KEY, board_id INTEGER NOT NULL, title TEXT NOT NULL, n INTEGER)")
	} else {
		mustExec(t, db,
			"CREATE TABLE boards (id bigint PRIMARY KEY, name text NOT NULL)",
			"CREATE TABLE posts (id bigint PRIMARY KEY, board_id bigint NOT NULL, title varchar(200) NOT NULL, n integer)")
	}
	return db
}

func describeText(desc practicedb.Description) string {
	var cols []string
	for _, c := range desc.Columns {
		cols = append(cols, fmt.Sprintf("%s=%s.%s:%d", c.Name, c.Table, c.Origin, c.Type))
	}
	return fmt.Sprintf("%s %v", strings.Join(cols, " "), desc.Params)
}

// What the drivers say, as TASKS-M3 "먼저 확인한 사실" found: the source of
// plain and aliased columns, none for expressions, and the source of the
// NULL side of an outer join as if it were not.
func TestDescribe(t *testing.T) {
	for _, c := range []struct {
		query          string
		sqlite, postgr string
	}{
		{"SELECT id, title FROM posts",
			"id=posts.id:0 title=posts.title:0 []", "id=posts.id:20 title=posts.title:1043 []"},
		{"SELECT title AS t FROM posts",
			"t=posts.title:0 []", "t=posts.title:1043 []"},
		{"SELECT p.title, b.name FROM posts p JOIN boards b ON b.id = p.board_id",
			"title=posts.title:0 name=boards.name:0 []", "title=posts.title:1043 name=boards.name:25 []"},
		{"SELECT count(*) FROM posts",
			"count(*)=.:0 []", "count=.:20 []"},
		{"SELECT b.name FROM posts p LEFT JOIN boards b ON b.id = p.board_id",
			"name=boards.name:0 []", "name=boards.name:25 []"},
		{"SELECT id FROM posts WHERE title = {1} AND n = {2}",
			"id=posts.id:0 []", "id=posts.id:20 [25 23]"},
		{"UPDATE posts SET n = 1", " []", " []"},
		// Only tables of the practice schema are sources.
		{"SELECT relname FROM pg_class LIMIT 1", "", "relname=.:19 []"},
	} {
		for _, d := range []project.Dialect{project.SQLite, project.Postgres} {
			t.Run(string(d), func(t *testing.T) {
				db := describeDB(t, d)
				q, want := strings.NewReplacer("{1}", "?1", "{2}", "?2").Replace(c.query), c.sqlite
				if d == project.Postgres {
					q, want = strings.NewReplacer("{1}", "$1", "{2}", "$2").Replace(c.query), c.postgr
				}
				if want == "" {
					t.Skip("PostgreSQL only")
				}
				desc, err, ds := db.Describe(ctx(t), q)
				if err != nil || ds != nil {
					t.Fatalf("%s: %v %s", q, err, text(ds))
				}
				if got := describeText(desc); got != want {
					t.Errorf("%s:\ngot  %s\nwant %s", q, got, want)
				}
			})
		}
	}
}

// A failed describe leaves the practice database usable, and describing
// leaves no prepared statement behind.
func TestDescribeFailures(t *testing.T) {
	for _, d := range []project.Dialect{project.SQLite, project.Postgres} {
		t.Run(string(d), func(t *testing.T) {
			db := describeDB(t, d)
			for _, q := range []string{"SELEC 1", "SELECT nope FROM posts"} {
				if _, err, ds := db.Describe(ctx(t), q); err == nil || ds != nil {
					t.Errorf("%s: got %v %s, want an error of the query", q, err, text(ds))
				}
				if _, err, ds := db.Describe(ctx(t), "SELECT id FROM posts"); err != nil || ds != nil {
					t.Fatalf("after %s: %v %s", q, err, text(ds))
				}
			}
			if d != project.Postgres {
				return
			}
			rows, err := db.QueryContext(ctx(t), "SELECT name FROM pg_prepared_statements WHERE name NOT LIKE 'stmtcache_%'")
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var name string
				rows.Scan(&name)
				t.Errorf("prepared statement %q is left", name)
			}
			rows.Close()
			// Every savepoint was released: releasing again fails. Last,
			// since the error aborts the transaction.
			var pe *pgconn.PgError
			if _, err := db.QueryContext(ctx(t), "RELEASE SAVEPOINT maago_describe"); !errors.As(err, &pe) || pe.Code != "3B001" {
				t.Errorf("RELEASE again: got %v, want SQLSTATE 3B001", err)
			}
		})
	}
}
