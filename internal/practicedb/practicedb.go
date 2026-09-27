// Package practicedb opens the practice database that checks migrations
// and queries before they reach a real database (D-35): an in-memory
// database for SQLite, and for PostgreSQL a schema in the development
// database, created inside a transaction that is always rolled back
// (D-96). Every statement runs on one connection, since an in-memory
// database and search_path belong to a connection.
//
// The drivers are registered by cmd/maago (D-90); this package only knows
// their names.
package practicedb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"go/token"
	"net/url"
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
	name, ok := drivers[d]
	if !ok {
		panic("practicedb: unknown dialect " + string(d))
	}
	if !slices.Contains(sql.Drivers(), name) {
		return nil, fail(`no "`+name+`" database driver in this maago build`, "build maago from its cmd/maago module, which registers the drivers")
	}
	fix := "report this as a maago bug"
	if d == project.Postgres {
		fix = "start the PostgreSQL server, check MAAGO_DATABASE_URL, and make sure its user may create schemas (GRANT CREATE ON DATABASE)"
	} else {
		url = ":memory:"
	}
	cannotOpen := func(err error) []diag.Diagnostic {
		return fail("cannot open the practice database: "+diag.OneLine(hidePassword(err.Error(), url)), fix)
	}
	db, err := sql.Open(name, url)
	if err != nil {
		return nil, cannotOpen(err)
	}
	p := &DB{db: db, d: d}
	if p.conn, err = db.Conn(ctx); err == nil {
		err = p.setUp(ctx)
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

// hidePassword removes the password of addr from msg. pgx already leaves
// it out of its errors; this does not rely on that.
func hidePassword(msg, addr string) string {
	u, err := url.Parse(addr)
	if err != nil {
		return msg // pgx's own parse error redacts it
	}
	pw, _ := u.User.Password()
	for _, p := range []string{pw, u.Query().Get("password")} {
		if p != "" {
			msg = strings.ReplaceAll(msg, p, "xxxxx")
		}
	}
	return msg
}

// setUp turns on foreign keys for SQLite, or creates the practice schema
// for PostgreSQL. It reads each setting back, in case it was ignored.
func (p *DB) setUp(ctx context.Context) error {
	var q, want string
	if p.d == project.SQLite {
		if _, err := p.conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
			return err
		}
		q, want = "PRAGMA foreign_keys", "1"
	} else {
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
