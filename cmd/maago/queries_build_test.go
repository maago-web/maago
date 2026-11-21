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

// Before the migration, the queries are prepared on the migrations plus
// makemigrations' draft, the schema the next run sees: the "run
// makemigrations" line and the queries' errors come out in one run, and
// those say what they were checked against. makemigrations reads no
// query file, and after it the same errors come without the note.
func TestQueriesBeforeMigration(t *testing.T) {
	for _, d := range []project.Dialect{project.SQLite, project.Postgres} {
		t.Run(string(d), func(t *testing.T) {
			root := ordersProject(t, d)
			withEmail(t, root, "string")
			writeFile(t, filepath.Join(root, "queries", "emails.sql"), "-- name: Emails :many Customer\nSELECT * FROM customers\nWHERE email <> ''\n")
			email := `models.go:17:2: ` + differ + `column "email" of "customers" is missing`
			lines(t, root, email)

			writeFile(t, filepath.Join(root, "queries", "typo.sql"), typoSQL)
			writeFile(t, filepath.Join(root, "queries", "short.sql"), "-- name: Short :many Customer\nSELECT id FROM customers\n")
			short := `queries/short.sql:2:1: result columns do not match model Customer: missing "name", "email"`
			lines(t, root, email, short+against, typo(d)+against)

			if makeMigrations(t, root, "") == "" {
				t.Fatal("no migration for Email")
			}
			lines(t, root, short, typo(d))
			os.Remove(filepath.Join(root, "queries", "typo.sql"))
			os.Remove(filepath.Join(root, "queries", "short.sql"))
			writeFile(t, filepath.Join(root, "queries", "bad.sql"), "-- nmae: Bad :exec\nDELETE FROM orders\n")
			lines(t, root, "queries/bad.sql:1:1: query has no -- name: line", `queries/bad.sql:1:4: unknown header key "nmae"`)

			// A field dropped before the migration: the query still using
			// its column fails now, not after makemigrations.
			root = ordersProject(t, d)
			models := filepath.Join(root, "models.go")
			writeFile(t, models, regexp.MustCompile("\tPriority .*\n").ReplaceAllString(readFile(t, models), ""))
			priority := map[project.Dialect]string{
				project.SQLite:   `queries/order_by_amount.sql:1:10: SQL logic error: no such column: priority (1)`,
				project.Postgres: `queries/order_by_amount.sql:4:65: ERROR: column "priority" does not exist (SQLSTATE 42703)`,
			}[d]
			lines(t, root, `models.go:19:6: `+differ+`column "priority" of "orders" is not in the models`, priority+against)
		})
	}
}

// against ends a query's error found before the migration.
const against = " (checked against the models)"

// typoSQL is a query with a typo for column email, and typo its error.
const typoSQL = "-- name: Typo :exec\nDELETE FROM customers\nWHERE emial = ''\n"

func typo(d project.Dialect) string {
	if d == project.Postgres {
		return `queries/typo.sql:3:7: ERROR: column "emial" does not exist (SQLSTATE 42703)`
	}
	return `queries/typo.sql:1:10: SQL logic error: no such column: emial (1)`
}

// withEmail adds field Email of typ to model Customer, at models.go:17,
// which no query of the fixture returns.
func withEmail(t *testing.T, root, typ string) {
	t.Helper()
	models := filepath.Join(root, "models.go")
	writeFile(t, models, strings.Replace(readFile(t, models), "\tName string `maago:\"max=100\"`\n", "\tName string `maago:\"max=100\"`\n\tEmail "+typ+"\n", 1))
}

// lines checks that check on root reports exactly want, whole lines in
// order, without their fix and see lines.
func lines(t *testing.T, root string, want ...string) {
	t.Helper()
	if got := diagnostics(text(build.Check(root))); !slices.Equal(got, want) {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// What only a migration written by hand has, a name the models do not
// give, is in the database the queries are checked on before the
// migration too: a query using it has no error, and a typo beside it
// has, so the queries were checked, and on the migrations.
func TestQueriesHandMigration(t *testing.T) {
	for _, d := range []project.Dialect{project.SQLite, project.Postgres} {
		t.Run(string(d), func(t *testing.T) {
			root := ordersProject(t, d)
			hand := "-- name: ByCustomer :many Count\n-- column: n int64\nSELECT count(*) AS n FROM orders INDEXED BY orders_by_customer WHERE customer_id = 1\n"
			if d == project.SQLite {
				initial := filepath.Join(root, "migrations", "0001_initial.sql")
				src := readFile(t, initial)
				m := regexp.MustCompile(`CREATE INDEX "[^"]+" ON "orders" \("customer_id"\)`).FindString(src)
				if m == "" {
					t.Fatalf("no index on customer_id in:\n%s", src)
				}
				writeFile(t, initial, strings.Replace(src, m, `CREATE INDEX "orders_by_customer" ON "orders" ("customer_id")`, 1))
			} else {
				writeFile(t, filepath.Join(root, "migrations", "0002_kor.sql"), "CREATE TEXT SEARCH CONFIGURATION kor (COPY = simple);\n")
				hand = "-- name: ByWord :many Count\n-- column: n int64\nSELECT count(*) AS n FROM orders WHERE to_tsvector('kor', status) @@ to_tsquery('kor', 'new')\n"
			}
			writeFile(t, filepath.Join(root, "queries", "hand.sql"), hand)
			if _, ds := build.Gen(root); len(ds) > 0 {
				t.Fatalf("gen with the migrations matching:\n%s", text(ds))
			}

			withEmail(t, root, "*string")
			writeFile(t, filepath.Join(root, "queries", "typo.sql"), typoSQL)
			lines(t, root, `models.go:17:2: `+differ+`column "email" of "customers" is missing`, typo(d)+against)
		})
	}
}

// A draft that fails in the practice database, a NOT NULL column without
// a default on a table with rows, leaves the queries for the next run:
// check says only that the migrations differ, as makemigrations stops.
func TestQueriesDraftFails(t *testing.T) {
	for _, d := range []project.Dialect{project.SQLite, project.Postgres} {
		t.Run(string(d), func(t *testing.T) {
			root := ordersProject(t, d)
			writeFile(t, filepath.Join(root, "migrations", "0002_data.sql"), "INSERT INTO customers (name) VALUES ('a');\n")
			withEmail(t, root, "string")
			writeFile(t, filepath.Join(root, "queries", "typo.sql"), typoSQL)
			lines(t, root, `models.go:17:2: `+differ+`column "email" of "customers" is missing`)
			if _, ds := build.MakeMigrations(root, "", nil); !strings.Contains(text(ds), "maago did not write") {
				t.Errorf("makemigrations:\n%s", text(ds))
			}
		})
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
	expect(t, root, `models.go:16:22: unknown tag key "maxlen" in Customer.Name`,
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
