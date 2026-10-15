package mapper

import (
	"regexp"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/model"
)

// An UPDATE of a table with a version sets it from a time.Time parameter
// (D-150).
func TestVersions(t *testing.T) {
	apps := []*model.App{{Dir: "blog", Models: []*model.Model{{Name: "Post", Table: "posts", Fields: []*model.Field{
		{Name: "ID", Column: "id", Type: model.Int64},
		{Name: "Status", Column: "status", Type: model.String},
		{Name: "UpdatedAt", Column: "updated_at", Type: model.Time, Auto: "update"},
	}}}}}
	// head declares the parameters body uses.
	head := func(body string) string {
		h := "-- name: Q :exec\n"
		for _, p := range []string{"id int64", "status string", "now time.Time", "n? int64"} {
			name, _, _ := strings.Cut(strings.Replace(p, "?", "", 1), " ")
			if regexp.MustCompile(`[:{]` + name + `\b`).MatchString(body) {
				h += "-- param: " + p + "\n"
			}
		}
		return h
	}
	for _, c := range []struct{ body, want string }{
		{"UPDATE posts SET status = :status, updated_at = :now WHERE id = :id", ""},
		{"UPDATE \"posts\" SET updated_at = :now, status = :status WHERE id = :id RETURNING id", ""},
		{"UPDATE boards SET name = :status WHERE id = :id", ""},                            // no version
		{"UPDATE posts SET updated_at = :now\n{n} , status = :status\nWHERE id = :id", ""}, // the condition is another column's
		{"UPDATE posts SET status = :status\n{n} , updated_at = :now\nWHERE id = :id", "sets updated_at, the version of posts, only under a condition"},
		{"UPDATE posts SET (status, updated_at) = (:status, :now) WHERE id = :id", "to something other than a time.Time parameter"},
		{"UPDATE posts SET status = :status WHERE id = :id", "query Q updates posts without its version updated_at"},
		{"UPDATE posts SET status = :status, updated_at = CURRENT_TIMESTAMP WHERE id = :id", "query Q sets updated_at, the version of posts, to something other than a time.Time parameter"},
		{"UPDATE posts SET status = :status, updated_at = :n WHERE id = :id", "to something other than a time.Time parameter"},
		{"WITH x AS (SELECT 1) UPDATE posts SET status = :status WHERE id = :id", "without its version"},
		{"UPDATE posts SET {\nupdated_at = :now,\n{n} status = :status,\n}\nWHERE id = :id", ""},
		{"UPDATE posts SET {\nstatus = :status,\nupdated_at = :now,\n}\nWHERE id = :id", ""},
		{"UPDATE posts SET {\n{n} status = :status,\n}\nWHERE id = :id", "without its version"},
		{"WITH u AS (UPDATE posts SET updated_at = :now) SELECT 1", ""},
		{"INSERT INTO posts (id, status, updated_at) VALUES (:id, :status, :now) ON CONFLICT (id) DO UPDATE SET status = excluded.status", "updates posts without its version"},
		{"INSERT INTO posts (id, status, updated_at) VALUES (:id, :status, :now) ON CONFLICT (id) DO UPDATE SET status = excluded.status, updated_at = :now", ""},
	} {
		q, ds := parse("blog/queries/q.sql", head(c.body)+c.body+"\n")
		if ds == nil {
			ds = q.body()
		}
		if ds != nil {
			t.Fatalf("%s: %s", c.body, text(ds))
		}
		got := text(Versions(apps, []*Query{q}))
		if c.want == "" && got != "" || c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("%s:\ngot  %s\nwant %s", c.body, got, c.want)
		}
	}
}
