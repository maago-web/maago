// Package practicedb opens the practice database that checks migrations
// and queries before they reach a real database (D-35): an in-memory
// database for SQLite, and for PostgreSQL a schema in the development
// database, created inside a transaction that is always rolled back
// (D-96). Every statement runs on one connection, since an in-memory
// database and search_path belong to a connection. OpenDatabase opens the
// project's own database the same way, for migrate.
//
// The drivers are registered by cmd/maago (D-90); this package only knows
// their names.
package practicedb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/project"
)

// DB is one practice database.
type DB struct {
	db   *sql.DB
	conn *sql.Conn
	d    project.Dialect
}

var drivers = map[project.Dialect]string{project.SQLite: "sqlite", project.Postgres: "pgx"}

// Open opens a practice database. url is used for PostgreSQL only. On
// failure it closes what it opened and returns nil.
func Open(ctx context.Context, d project.Dialect, url string) (*DB, []diag.Diagnostic) {
	fix := "report this as a maago bug"
	dsn := ":memory:"
	if d == project.Postgres {
		fix = "start the PostgreSQL server, check MAAGO_DATABASE_URL, and make sure its user may create schemas (GRANT CREATE ON DATABASE)"
		dsn = url
	}
	return open(ctx, d, dsn, url, "the practice database", fix, true)
}

// OpenDatabase opens the project's own database, which migrate changes,
// on one connection like a practice database: SQLite at the file named
// after sqlite:, from the project root, with foreign keys on; PostgreSQL
// as its address says, outside any transaction.
func OpenDatabase(ctx context.Context, d project.Dialect, url, root string) (*DB, []diag.Diagnostic) {
	dsn, what, fix := url, "the database", "check MAAGO_DATABASE_URL and that the database is running"
	if d == project.SQLite {
		dsn = SQLitePath(url, root)
		abs, _ := filepath.Abs(dsn)
		what = fmt.Sprintf("SQLite file %q", abs)
		fix = "check that its directory exists and can be written (WAL writes " + filepath.Base(dsn) + "-wal and -shm beside it); a relative sqlite: path is from the working directory"
	}
	return open(ctx, d, dsn, url, what, fix, false)
}

// SQLitePath is the file of a sqlite:FILE address, from the project root
// unless absolute.
func SQLitePath(url, root string) string {
	path := strings.TrimPrefix(url, "sqlite:")
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	return path
}

func open(ctx context.Context, d project.Dialect, dsn, url, what, fix string, practice bool) (*DB, []diag.Diagnostic) {
	name, ok := drivers[d]
	if !ok {
		panic("practicedb: unknown dialect " + string(d))
	}
	if !slices.Contains(sql.Drivers(), name) {
		return nil, fail(`no "`+name+`" database driver in this maago build`, "build maago from its cmd/maago module, which registers the drivers")
	}
	cannotOpen := func(err error) []diag.Diagnostic {
		return fail("cannot open "+what+": "+diag.OneLine(project.HidePassword(err.Error(), url)), fix)
	}
	db, err := sql.Open(name, dsn)
	if err != nil {
		return nil, cannotOpen(err)
	}
	p := &DB{db: db, d: d}
	if p.conn, err = db.Conn(ctx); err == nil {
		err = p.setUp(ctx, practice)
	}
	if err != nil {
		p.Close()
		return nil, cannotOpen(err)
	}
	if d == project.Postgres {
		var version int
		if err := p.conn.QueryRowContext(ctx, "SELECT current_setting('server_version_num')::int").Scan(&version); err != nil {
			p.Close()
			return nil, cannotOpen(err)
		}
		if version < minPostgres {
			p.Close()
			return nil, fail(fmt.Sprintf("PostgreSQL %d.%d is older than %d, which maago needs", version/10000, version%10000, minPostgres/10000),
				fmt.Sprintf("use PostgreSQL %d or later (D-98)", minPostgres/10000))
		}
	}
	return p, nil
}

// minPostgres is the oldest PostgreSQL whose catalog the schema reader
// knows (server_version_num), D-98.
const minPostgres = 150000

// setUp turns on foreign keys for SQLite, or creates the practice schema
// for PostgreSQL. It reads each setting back, in case it was ignored. A
// real PostgreSQL database needs nothing set up.
func (p *DB) setUp(ctx context.Context, practice bool) error {
	var q, want string
	switch {
	case p.d == project.Postgres && !practice:
		return nil
	case p.d == project.SQLite:
		pragmas := []string{"PRAGMA foreign_keys = ON"}
		if !practice {
			// A migrate waits for another, which may take long, and for the
			// app's writes (D-153). WAL from the start, as the app's
			// connections set it: switching needs the file alone, which
			// replicas that start at once do not leave it.
			pragmas = append(pragmas, "PRAGMA busy_timeout = 600000", "PRAGMA journal_mode = WAL")
		}
		for _, pragma := range pragmas {
			if _, err := p.conn.ExecContext(ctx, pragma); err != nil {
				return err
			}
		}
		q, want = "PRAGMA foreign_keys", "1"
	default:
		schema := "maago_practice_" + strings.ToLower(rand.Text())
		for _, s := range []string{
			"BEGIN",
			"CREATE SCHEMA " + dialect.Quote(schema),
			"SET LOCAL search_path TO " + dialect.Quote(schema),
			// Backslashes in '…' are plain, as dialect.Split reads them.
			"SET LOCAL standard_conforming_strings = on",
		} {
			if _, err := p.conn.ExecContext(ctx, s); err != nil {
				return err
			}
		}
		q, want = "SELECT current_schema()", schema
	}
	var got string
	if err := p.conn.QueryRowContext(ctx, q).Scan(&got); err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("%s gave %q, want %q", q, got, want)
	}
	return nil
}

// ErrorPosition returns where in its statement err happened, counting
// characters from 1, or 0. The root module knows no driver (D-90), so the
// command that registers them fills it in.
var ErrorPosition = func(error) int { return 0 }

// Exec runs one statement, written at pos. If it fails, the diagnostic is
// at pos, or where in the statement the database says. In PostgreSQL a
// failed statement aborts the transaction, so callers stop at the first
// failure.
func (p *DB) Exec(ctx context.Context, pos token.Position, stmt string) []diag.Diagnostic {
	_, err := p.conn.ExecContext(ctx, stmt)
	if err == nil {
		return nil
	}
	if n := ErrorPosition(err); n > 0 {
		for _, r := range stmt {
			if n--; n == 0 {
				break
			}
			if r == '\n' {
				pos.Line, pos.Column = pos.Line+1, 1
			} else {
				pos.Column += utf8.RuneLen(r)
			}
		}
	}
	return []diag.Diagnostic{diag.New(pos, diag.OneLine(err.Error()), "fix the SQL statement")}
}

// Column is a result column of a query, as the database describes it.
type Column struct {
	Name string
	// Table and Origin are the table and column it comes from, in the
	// practice schema; both "" for an expression. Table is the table's
	// name, not the alias the query gives it.
	Table, Origin string
	Type          uint32 // PostgreSQL type OID; 0 for SQLite
}

// Description is what the database says about a query without running
// it. Params are the PostgreSQL type OIDs of $1…$n; SQLite does not know
// them, so they are nil.
type Description struct {
	Columns []Column
	Params  []uint32
}

// DescribeQuery prepares query on conn and describes it. The root module
// knows no driver (D-90), so the command that registers them fills it in.
var DescribeQuery = func(ctx context.Context, conn *sql.Conn, query string) (Description, error) {
	return Description{}, errors.New("no database driver to describe queries in this maago build")
}

// Describe prepares query and describes it. A failed query does not end
// the practice database: in PostgreSQL it runs under a savepoint, rolled
// back if it fails, so the transaction goes on (D-96). The error is the
// query's fault; the diagnostics are the connection's, after which the
// database cannot be used.
func (p *DB) Describe(ctx context.Context, query string) (Description, error, []diag.Diagnostic) {
	pg := p.d == project.Postgres
	run := func(stmt string) []diag.Diagnostic {
		if _, err := p.conn.ExecContext(ctx, stmt); err != nil {
			return fail("practice database: "+diag.OneLine(err.Error()), "check that the PostgreSQL server is still running")
		}
		return nil
	}
	if pg {
		if ds := run("SAVEPOINT maago_describe"); ds != nil {
			return Description{}, nil, ds
		}
	}
	desc, err := DescribeQuery(ctx, p.conn, query)
	if pg {
		if err != nil {
			if ds := run("ROLLBACK TO SAVEPOINT maago_describe"); ds != nil {
				return Description{}, nil, ds
			}
		}
		if ds := run("RELEASE SAVEPOINT maago_describe"); ds != nil {
			return Description{}, nil, ds
		}
	}
	return desc, err, nil
}

// Dialect is the database's dialect.
func (p *DB) Dialect() project.Dialect { return p.d }

// TypeName is the PostgreSQL name of a type OID, for messages.
func (p *DB) TypeName(ctx context.Context, oid uint32) string {
	var name string
	if err := p.conn.QueryRowContext(ctx, "SELECT format_type($1, NULL)", oid).Scan(&name); err != nil {
		return fmt.Sprintf("type OID %d", oid)
	}
	return name
}

// QueryContext runs a query on the connection, for reading the schema.
func (p *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return p.conn.QueryContext(ctx, query, args...)
}

// Close closes the connection. For PostgreSQL that rolls back the
// transaction and with it the practice schema (D-96): an uncommitted
// transaction dies with its connection, so no ROLLBACK is sent that could
// hang on a stuck server.
func (p *DB) Close() {
	if p.conn != nil {
		p.conn.Close()
	}
	p.db.Close()
}

func fail(msg, fix string) []diag.Diagnostic {
	return []diag.Diagnostic{diag.New(token.Position{}, msg, fix)}
}
