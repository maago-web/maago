package main

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/build"
	"github.com/maago-web/maago/internal/project"
)

// ordersProject copies the orders fixture into a new project with a
// second app, reports, that has queries and no models, and makes the
// first migration.
func ordersProject(t *testing.T, d project.Dialect) string {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../internal/testdata/orders")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/orders\n")
	writeFile(t, filepath.Join(root, "reports", "doc.go"), "package reports\n")
	writeFile(t, filepath.Join(root, "reports", "queries", "order_count.sql"),
		"-- name: OrderCount :one Count\n-- column: n int64\nSELECT count(*) AS n FROM orders\n")
	env := "sqlite:app.db"
	if d == project.Postgres {
		env = pgURL(t)
	}
	t.Setenv("MAAGO_DATABASE_URL", env)
	makeMigrations(t, root, "")
	return root
}

// diagnostics are the lines of out that start a diagnostic, without its
// fix and see lines.
func diagnostics(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if l != "" && !strings.HasPrefix(l, "  ") {
			lines = append(lines, l)
		}
	}
	return lines
}

// Gen writes each app's queries_gen.go, the same as the gentest golden,
// and check sees when a query file changes.
func TestQueriesGenCheck(t *testing.T) {
	root := ordersProject(t, project.SQLite)
	if _, ds := build.Gen(root); len(ds) > 0 {
		t.Fatal(text(ds))
	}
	for _, f := range []string{"models_gen.go", "queries_gen.go"} {
		if readFile(t, filepath.Join(root, f)) != readFile(t, "internal/gentest/sqlite/orders/"+f) {
			t.Errorf("%s is not the gentest golden", f)
		}
	}
	af, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "reports", "queries_gen.go"), nil, parser.PackageClauseOnly)
	if err != nil || af.Name.Name != "reports" {
		t.Fatalf("reports/queries_gen.go: %v, package %v", err, af)
	}
	if ds := build.Check(root); len(ds) > 0 {
		t.Fatalf("check right after gen:\n%s", text(ds))
	}

	search := filepath.Join(root, "queries", "search_orders.sql")
	writeFile(t, search, strings.Replace(readFile(t, search), "LIMIT :limit", "LIMIT :limit OFFSET 0", 1))
	if got := diagnostics(text(build.Check(root))); !slices.Equal(got, []string{"queries_gen.go: generated file is out of date"}) {
		t.Errorf("check after a query changed: %q", got)
	}

	os.Remove(filepath.Join(root, "reports", "doc.go"))
	if got := diagnostics(text(build.Check(root))); !slices.Equal(got, []string{
		"reports/queries: directory reports has queries but no Go file saying which package they belong to"}) {
		t.Errorf("check without a package clause: %q", got)
	}
	writeFile(t, filepath.Join(root, "reports", "doc.go"), "package reports\n")
	os.RemoveAll(filepath.Join(root, "reports", "queries"))
	if removed, ds := build.Gen(root); len(ds) > 0 || !slices.Equal(removed, []string{"reports/queries_gen.go"}) {
		t.Errorf("removed %v\n%s", removed, text(ds))
	}
}

// The queries are prepared only on a database that matches the models,
// and makemigrations reads no query file.
func TestQueriesAfterModels(t *testing.T) {
	root := ordersProject(t, project.SQLite)
	models := filepath.Join(root, "models.go")
	writeFile(t, models, strings.Replace(readFile(t, models), "\tAmount     int64\n", "\tAmount     int64\n\tNote       string\n", 1))
	writeFile(t, filepath.Join(root, "queries", "notes.sql"), "-- name: Notes :many Order\nSELECT * FROM orders\nWHERE note <> ''\n")
	out := text(build.Check(root))
	if !strings.Contains(out, differ) || strings.Contains(out, "queries/") {
		t.Errorf("check before the migration:\n%s", out)
	}

	writeFile(t, filepath.Join(root, "queries", "bad.sql"), "-- nmae: Bad :exec\nDELETE FROM orders\n")
	if makeMigrations(t, root, "") == "" {
		t.Fatal("no migration for Note")
	}
	if got := diagnostics(text(build.Check(root))); !slices.Equal(got, []string{
		"queries/bad.sql:1:1: query has no -- name: line", `queries/bad.sql:1:4: unknown header key "nmae"`}) {
		t.Errorf("check after the migration: %q", got)
	}
}

// A field that a query uses is dropped: makemigrations drafts it all the
// same, and check reports the query, and only the query: the generated
// files are not compared, and no other query fails.
func TestQueriesDroppedField(t *testing.T) {
	root := ordersProject(t, project.SQLite)
	models := filepath.Join(root, "models.go")
	writeFile(t, models, regexp.MustCompile("\tPriority .*\n").ReplaceAllString(readFile(t, models), ""))
	written := makeMigrations(t, root, "")
	if written == "" {
		t.Fatal("no migration")
	}
	p := filepath.Join(root, written)
	src := readFile(t, p)
	i := strings.Index(src, "ALTER TABLE \"orders\" DROP COLUMN")
	if i < 0 {
		t.Fatalf("no DROP COLUMN in:\n%s", src)
	}
	writeFile(t, p, src[:i]+"-- maago: allow-drop\n"+src[i:])
	expect(t, root, "queries/order_by_amount.sql:1:10: SQL logic error: no such column: priority")
}

// The queries of every app are reported in one run.
func TestQueriesErrorsTogether(t *testing.T) {
	root := ordersProject(t, project.SQLite)
	writeFile(t, filepath.Join(root, "queries", "nope.sql"), "-- name: Nope :exec\nDELETE FROM orders\nWHERE nope = 1\n")
	writeFile(t, filepath.Join(root, "reports", "queries", "nope.sql"), "-- name: Nope :exec\nDELETE FROM orders\nWHERE nope = 1\n")
	// SQLite does not say where in the SQL: the error is at the name.
	expect(t, root, "queries/nope.sql:1:10: SQL logic error: no such column: nope", "reports/queries/nope.sql:1:10: SQL logic error: no such column: nope")
}

// What needs no database is reported without one: a name clash stops the
// run before the database is opened. The tags and the query files are
// read in one stage, so an error in each comes out together.
func TestQueriesWithoutDatabase(t *testing.T) {
	root := ordersProject(t, project.SQLite)
	t.Setenv("MAAGO_DATABASE_URL", "postgres://127.0.0.1:1/nowhere")
	writeFile(t, filepath.Join(root, "queries", "get_order.sql"), "-- name: GetOrder :exec\nDELETE FROM orders\nWHERE id = 1\n")
	writeFile(t, filepath.Join(root, "reports", "doc.go"), "package reports\n\nfunc OrderCount() {}\n")
	expect(t, root,
		"queries/get_order.sql:1:10: query GetOrder would declare GetOrder, which the CRUD of model Order declares too",
		"reports/queries/order_count.sql:1:10: query OrderCount would declare OrderCount, which reports/doc.go:3 declares already")

	models := filepath.Join(root, "models.go")
	writeFile(t, models, strings.Replace(readFile(t, models), `maago:"max=100"`, `maago:"maxlen=100"`, 1))
	writeFile(t, filepath.Join(root, "queries", "get_order.sql"), "-- nmae: X :exec\nDELETE FROM orders\n")
	expect(t, root, `models.go:15:22: unknown tag key "maxlen" in Customer.Name`,
		"queries/get_order.sql:1:1: query has no -- name: line", `queries/get_order.sql:1:4: unknown header key "nmae"`)
}

// In PostgreSQL, gen matches the gentest golden, and a query that fails
// to prepare does not end the transaction: the queries after it and the
// CRUD still prepare, so its error is the only one.
func TestQueriesPG(t *testing.T) {
	root := ordersProject(t, project.Postgres)
	if _, ds := build.Gen(root); len(ds) > 0 {
		t.Fatal(text(ds))
	}
	if readFile(t, filepath.Join(root, "queries_gen.go")) != readFile(t, "internal/gentest/postgres/orders/queries_gen.go") {
		t.Error("queries_gen.go is not the gentest golden")
	}
	if ds := build.Check(root); len(ds) > 0 {
		t.Fatalf("check right after gen:\n%s", text(ds))
	}
	writeFile(t, filepath.Join(root, "queries", "a_nope.sql"), "-- name: ANope :exec\nDELETE FROM orders\nWHERE nope = 1\n")
	expect(t, root, "queries/a_nope.sql:3:")
}
