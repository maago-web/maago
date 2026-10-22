package mapper

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/golden"
	"github.com/maago-web/maago/internal/project"
)

func TestTables(t *testing.T) {
	for _, c := range []struct{ body, want string }{
		// The shapes of the queries of the board, auth and the fixtures.
		{"SELECT p.id, (SELECT count(*) FROM board_comment c WHERE c.post_id = p.id) AS comments FROM board_post p", "board_comment c, board_post p"},
		{"SELECT EXISTS (SELECT 1 FROM orders WHERE amount > 1)", "orders"},
		{"SELECT u.* FROM auth_user u JOIN auth_token t ON t.user_id = u.id", "auth_user u, auth_token t"},
		{"SELECT 1 FROM posts AS P JOIN \"Boards\" \"B\" ON true", "posts p, boards B"},
		// What INSERT, UPDATE and DELETE write is not read; what they read is.
		{"DELETE FROM auth_token WHERE hash = 1 RETURNING *", ""},
		{"UPDATE orders SET status = 'x' WHERE id IN (SELECT id FROM customers)", "customers"},
		{"INSERT INTO orders (id) SELECT id FROM customers c", "customers c"},
		{"DELETE FROM orders o USING customers c WHERE c.id = o.customer_id", "customers c"},
		{"UPDATE orders SET status = c.name FROM customers c, notes n WHERE true", "customers c, notes n"},
		// A CTE hides the table of its name where it is seen.
		{"WITH posts AS (SELECT * FROM posts) SELECT * FROM posts", "posts"},
		{"WITH a AS (SELECT * FROM posts), posts AS (SELECT 1) SELECT * FROM a JOIN posts ON true", "posts"},
		{"WITH RECURSIVE t(n) AS (SELECT 1 UNION ALL SELECT n FROM t) SELECT * FROM t JOIN boards b ON true", "boards b"},
		{"SELECT * FROM (WITH posts AS (SELECT 1) SELECT * FROM posts) s, posts", "posts"},
		{"WITH posts AS (SELECT * FROM posts) SELECT * FROM (WITH posts AS (SELECT * FROM posts) SELECT * FROM posts) q", "posts"},
		{"WITH boards AS (SELECT 1) SELECT * FROM s.boards", "boards"},
		{"WITH \"Posts\" AS (SELECT 1), \"boards\" AS (SELECT 1) SELECT * FROM posts, Boards", "posts"},
		// FROM lists, and where they end.
		{"SELECT 1 FROM a, b x, c AS y", "a, b x, c y"},
		{"SELECT 1 FROM a JOIN b ON a.x IN (1, 2), c", "a, b, c"},
		{"SELECT (SELECT 1 FROM a) AS x, y FROM t", "a, t"},
		{"SELECT (SELECT 1 FROM a), EXTRACT(year FROM b) FROM t", "a, t"},
		{"SELECT 1 FROM t WHERE a IS DISTINCT FROM b AND c IS NOT DISTINCT FROM d ORDER BY x, y", "t"},
		{"SELECT SUBSTRING(x FROM 2 FOR 3), TRIM(BOTH ' ' FROM y) FROM t GROUP BY a, b", "t"},
		{"SELECT 1 FROM a UNION SELECT 1 FROM b, c", "a, b, c"},
		{"SELECT 1 FROM (SELECT 1 FROM a) s JOIN LATERAL (SELECT 1 FROM b) l ON true", "a, b"},
		{"SELECT 1 FROM ONLY a, json_each(x) j, generate_series(1, 3) AS g", "a"},
		{"-- param: q? string\nSELECT 1 FROM a\n{q} JOIN b ON true", "a, b"},
		{"-- param: x? string\nSELECT 1 FROM a WHERE true\n{x} AND EXISTS (SELECT 1 FROM b)", "a, b"},
		{"SELECT 'FROM x' FROM a /* FROM y */", "a"},
		{"-- param: from? time.Time\nSELECT 1 FROM a WHERE true\n{from} AND b > :from", "a"},
		{"SELECT 1 FROM a JOIN b USING (id) LEFT JOIN c ON true", "a, b, c"},
		{"SELECT 1 FROM a CROSS JOIN b INNER JOIN c ON true NATURAL JOIN d RIGHT JOIN e ON true FULL JOIN f ON true", "a, b, c, d, e, f"},
		{"DELETE FROM o USING c RETURNING id, name", "c"},
		{"INSERT INTO t SELECT * FROM a ON CONFLICT (id) DO UPDATE SET x = 1, y = 2", "a"},
		{"SELECT 1 FROM a HAVING x, y WINDOW w AS (), z LIMIT 1, 2 OFFSET 3, 4 FOR UPDATE OF p, q", "a"},
		{"SELECT 1 FROM (VALUES (1), (2)) v, a", "a"},
		// What it cannot tell.
		{"SELECT 1 FROM (a JOIN b ON true)", "15: cannot tell which tables the query reads: a join in parentheses"},
		{"TABLE a", "1: cannot tell which tables the query reads: TABLE a"},
		{"SELECT 1 FROM a WHERE x IN (TABLE b)", "29: cannot tell which tables the query reads: TABLE b"},
		{"SELECT 1 FROM a UNION TABLE b", "23: cannot tell which tables the query reads: TABLE b"},
		{"SELECT 1 FROM 'a'", "15: cannot tell which tables the query reads: FROM not followed by a table name"},
		{"SELECT 1 FROM a *", "17: cannot tell which tables the query reads: * after the table a"},
		{"SELECT 1 FROM a TABLESAMPLE SYSTEM (1)", "29: cannot tell which tables the query reads: SYSTEM after the table a"},
		{"SELECT 1 FROM a INDEXED BY i", "25: cannot tell which tables the query reads: BY after the table a"},
	} {
		q, out := readBody(t, "-- name: Q :many R\n"+c.body+"\n")
		if out != "" {
			t.Fatalf("%s: %s", c.body, out)
		}
		refs, ds := q.tables()
		got := strings.TrimPrefix(text(ds), file+":2:")
		if ds == nil {
			got = refsText(refs)
		}
		if first, _, _ := strings.Cut(got, "\n"); first != c.want {
			t.Errorf("%s:\ngot  %s\nwant %s", c.body, got, c.want)
		}
	}
}

func refsText(refs []tableRef) string {
	var s []string
	for _, r := range refs {
		s = append(s, strings.TrimSpace(r.Table+" "+r.Alias))
	}
	return strings.Join(s, ", ")
}

// Every query of the board's last step, auth and the fixtures, with the
// tables it reads: those only a subquery reads are there too.
func TestTablesOfQueries(t *testing.T) {
	board := t.TempDir()
	if out, err := exec.Command("git", "-C", board, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	patches, _ := filepath.Glob("../../examples/board/steps/*.patch")
	for _, p := range patches {
		abs, _ := filepath.Abs(p)
		if out, err := exec.Command("git", "-C", board, "apply", "--include=*/queries/*.sql", abs).CombinedOutput(); err != nil {
			t.Fatalf("git apply %s: %v\n%s", p, err, out)
		}
	}
	var b strings.Builder
	for _, root := range []string{board, "../scaffold/testdata/auth", "../testdata/orders", "../testdata/plain", "../../cmd/maago/testdata/web/site"} {
		label := strings.TrimLeft(root, "./")
		if root == board {
			label = "examples/board"
		}
		if _, err := os.Stat(root); err != nil {
			t.Fatal(err)
		}
		files, ds := project.QueryFiles(root)
		if ds != nil {
			t.Fatal(text(ds))
		}
		apps, ds := Read(root, files)
		if ds != nil {
			t.Fatalf("%s: %s", root, text(ds))
		}
		for _, a := range apps {
			for _, q := range a.Queries {
				refs, ds := q.tables()
				if ds != nil {
					t.Errorf("%s", text(ds))
				}
				for _, r := range refs {
					fmt.Fprintf(&b, "%s/%s:%d:%d %s\n", label, q.File, r.Pos.Line, r.Pos.Column, strings.TrimSpace(r.Table+" "+r.Alias))
				}
			}
		}
	}
	golden.Check(t, "testdata/tables.golden", []byte(b.String()))
}
