package main

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/mapper"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/model/modeltest"
	"github.com/maago-web/maago/internal/practicedb"
	"github.com/maago-web/maago/internal/project"
)

// exampleConn is one connection to a database of d holding the shared
// example's tables: SQLite in memory, or for PostgreSQL a schema in a
// transaction that is rolled back.
func exampleConn(t *testing.T, d project.Dialect) *sql.Conn {
	t.Helper()
	driver, dsn := "sqlite", ":memory:"
	if d == project.Postgres {
		driver, dsn = "pgx", pgURL(t)
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	conn, err := db.Conn(ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	setup := []string{"PRAGMA foreign_keys = ON"}
	if d == project.Postgres {
		schema := "maago_crud_" + strings.ToLower(rand.Text())
		setup = []string{"BEGIN", "CREATE SCHEMA " + schema, "SET LOCAL search_path TO " + schema}
	}
	stmts, _ := dialect.Split("", readFile(t, "../../internal/dialect/"+string(d)+"/testdata/example.sql"))
	for _, s := range stmts {
		setup = append(setup, s.SQL)
	}
	for _, s := range setup {
		if _, err := conn.ExecContext(ctx(t), s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return conn
}

// PostgreSQL's types for each field type, as the models' DDL makes them.
// It repeats the table of internal/mapper, which does not export it.
var pgFieldTypes = map[model.Type][]uint32{
	model.Int64: {20}, model.Int32: {23}, model.String: {25, 1043}, model.Float64: {701},
	model.Bool: {16}, model.Time: {1184}, model.Bytes: {17},
}

// Every CRUD statement of the shared example prepares, and its
// placeholders take the fields of Args, in order.
func TestCRUDPrepares(t *testing.T) {
	apps, _ := modeltest.Example(t, "../../internal/testdata/example")
	for _, d := range []project.Dialect{project.SQLite, project.Postgres} {
		t.Run(string(d), func(t *testing.T) {
			conn := exampleConn(t, d)
			for _, c := range mapper.CRUDs(apps[0], d) {
				field := func(name string) *model.Field {
					i := slices.IndexFunc(c.Model.Fields, func(f *model.Field) bool { return f.Name == name })
					return c.Model.Fields[i]
				}
				for _, st := range []*mapper.Statement{&c.Get, &c.Insert, c.Update, &c.Delete} {
					desc, err := practicedb.DescribeQuery(ctx(t), conn, st.SQL)
					if err != nil {
						t.Fatalf("%s: %v", st.SQL, err)
					}
					if st == &c.Get {
						for i, col := range desc.Columns {
							if col.Name != c.Model.Fields[i].Column {
								t.Errorf("%s: column %d is %s, want %s", c.Model.Name, i, col.Name, c.Model.Fields[i].Column)
							}
						}
					}
					if d != project.Postgres {
						continue
					}
					if len(desc.Params) != len(st.Args) {
						t.Errorf("%s: %d placeholders, %d args", st.SQL, len(desc.Params), len(st.Args))
						continue
					}
					for i, oid := range desc.Params {
						if f := field(st.Args[i]); !slices.Contains(pgFieldTypes[f.Type], oid) {
							t.Errorf("%s: $%d is %s (%s), but PostgreSQL reads OID %d", st.SQL, i+1, f.Name, f.Type, oid)
						}
					}
				}
			}
		})
	}
}

// The CRUD runs: Insert returns the new id, and Update and Delete say how
// many rows they touched.
func TestCRUDRuns(t *testing.T) {
	apps, _ := modeltest.Example(t, "../../internal/testdata/example")
	for _, d := range []project.Dialect{project.SQLite, project.Postgres} {
		t.Run(string(d), func(t *testing.T) {
			conn := exampleConn(t, d)
			cs := mapper.CRUDs(apps[0], d)
			post, author := cs[0], cs[1]
			now := any(time.Now())
			if d == project.SQLite {
				now = maago.SQLiteTime(time.Now()) // D-105
			}
			args := func(st *mapper.Statement, values map[string]any) []any {
				out := make([]any, len(st.Args))
				for i, a := range st.Args {
					v, ok := values[a]
					if !ok {
						t.Fatalf("no value for %s", a)
					}
					out[i] = v
				}
				return out
			}
			person := func(name, code string, parent any, editor int64) map[string]any { // editor_id is unique
				return map[string]any{"Name": name, "ParentID": parent, "EditorID": editor, "ReviewerID": nil,
					"Code": code, "Order": int32(len(name) + 6), "Admin": false, "CreatedAt": now, "UpdatedAt": now}
			}
			// get reads a row back and shows the columns whose values tell
			// the fields apart, so that a column bound to the wrong field shows.
			get := func(id int64) string {
				row := make([]any, len(author.Model.Fields))
				ptrs := make([]any, len(row))
				for i := range row {
					ptrs[i] = &row[i]
				}
				if err := conn.QueryRowContext(ctx(t), author.Get.SQL, id).Scan(ptrs...); err != nil {
					t.Fatalf("get %d: %v", id, err)
				}
				// id, name, parent_id, editor_id, reviewer_id, code, order
				return fmt.Sprint(row[0], " ", row[1], " ", row[2], " ", row[3], " ", row[4], " ", row[5], " ", row[6])
			}
			for i, p := range []map[string]any{person("a", "c1", nil, 1), person("bb", "c2", int64(1), 2)} {
				var id int64
				if err := conn.QueryRowContext(ctx(t), author.Insert.SQL, args(&author.Insert, p)...).Scan(&id); err != nil || id != int64(i+1) {
					t.Fatalf("insert %d: id %d, %v", i+1, id, err)
				}
			}
			for id, want := range map[int64]string{1: "1 a <nil> 1 <nil> c1 7", 2: "2 bb 1 2 <nil> c2 8"} {
				if got := get(id); got != want {
					t.Errorf("get %d: %s, want %s", id, got, want)
				}
			}
			exec := func(st *mapper.Statement, values map[string]any) int64 {
				res, err := conn.ExecContext(ctx(t), st.SQL, args(st, values)...)
				if err != nil {
					t.Fatalf("%s: %v", st.SQL, err)
				}
				n, _ := res.RowsAffected()
				return n
			}
			changed := person("zzz", "c3", nil, 1)
			changed["ReviewerID"], changed["ID"] = int64(1), int64(1)
			if n := exec(author.Update, changed); n != 1 {
				t.Errorf("update: %d rows, want 1", n)
			}
			if got, want := get(1), "1 zzz <nil> 1 1 c3 9"; got != want {
				t.Errorf("after update: %s, want %s", got, want)
			}
			if n := exec(&author.Delete, map[string]any{"ID": int64(99)}); n != 0 {
				t.Errorf("delete of a missing id: %d rows, want 0", n)
			}
			if n := exec(&author.Delete, map[string]any{"ID": int64(2)}); n != 1 {
				t.Errorf("delete: %d rows, want 1", n)
			}
			// The zero value wins over the column's default: a zero Status
			// is "", which the choices CHECK refuses (D-83 against D-21).
			zero := map[string]any{"AuthorID": int64(1), "Title": "", "Slug": "", "Status": "", "Views": int32(0),
				"Likes": int32(0), "Score": nil, "Public": false, "Body": []byte{}, "PublishedAt": nil}
			var id int64
			err := conn.QueryRowContext(ctx(t), post.Insert.SQL, args(&post.Insert, zero)...).Scan(&id)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), "check") {
				t.Errorf("zero Post: got %v, want a CHECK failure on status", err)
			}
		})
	}
}

// A model of nothing but ID inserts with DEFAULT VALUES and gets its id,
// in both dialects.
func TestCRUDOnlyIDRuns(t *testing.T) {
	m := &model.Model{Name: "Tick", Table: "ticks", Fields: []*model.Field{{Name: "ID", Column: "id", Type: model.Int64}}}
	for _, d := range []project.Dialect{project.SQLite, project.Postgres} {
		t.Run(string(d), func(t *testing.T) {
			conn := exampleConn(t, d)
			ddl := "CREATE TABLE ticks (id INTEGER PRIMARY KEY AUTOINCREMENT) STRICT"
			if d == project.Postgres {
				ddl = "CREATE TABLE ticks (id bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY)"
			}
			if _, err := conn.ExecContext(ctx(t), ddl); err != nil {
				t.Fatal(err)
			}
			c := mapper.CRUDs(&model.App{Models: []*model.Model{m}}, d)[0]
			var id int64
			if err := conn.QueryRowContext(ctx(t), c.Insert.SQL).Scan(&id); err != nil || id != 1 {
				t.Errorf("%s: id %d, %v", c.Insert.SQL, id, err)
			}
		})
	}
}
