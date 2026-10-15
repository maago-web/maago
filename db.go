package maago

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"go/token"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/project"
)

// DB is what generated queries run on: *sql.DB, *sql.Tx and *sql.Conn
// all are one.
type DB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

var _, _, _ DB = (*sql.DB)(nil), (*sql.Tx)(nil), (*sql.Conn)(nil)

// DBConfig tells OpenDB which database to open and how big its pool is.
// A zero field takes the default of D-101. A negative one is passed on,
// and database/sql reads it as 0: no limit on open connections, no idle
// connections kept, no lifetime or idle time.
type DBConfig struct {
	// URL is sqlite:FILE or postgres://…, as for the CLI. Empty means
	// MAAGO_DATABASE_URL: the environment variable, else .env in the
	// working directory (D-101). A relative SQLite file is from the
	// working directory, and must exist: maago migrate makes it.
	URL string

	MaxOpenConns, MaxIdleConns       int
	ConnMaxLifetime, ConnMaxIdleTime time.Duration
}

// OpenDB opens the application's database and checks that it answers.
// main must import the driver: _ "modernc.org/sqlite" or
// _ "github.com/jackc/pgx/v5/stdlib" (D-90). SQLite connections enforce
// foreign keys, wait up to 5 seconds for a lock, and use WAL.
func OpenDB(ctx context.Context, cfg DBConfig) (*sql.DB, error) {
	// The same rules and messages as the CLI (D-97). DBConfig.URL wins,
	// and .env is not read then.
	var d project.Dialect
	var ds []diag.Diagnostic
	url := cfg.URL
	if url != "" {
		d, ds = project.URLDialect(url, token.Position{}, "DBConfig.URL")
	} else {
		d, url, ds = project.Database(".", os.LookupEnv)
	}
	if ds != nil {
		var b strings.Builder
		diag.Write(&b, ds)
		return nil, errors.New("maago: " + strings.TrimSpace(b.String()))
	}
	driver, dsn := "pgx", url
	open, idle, life, idleTime := 20, 5, 30*time.Minute, 5*time.Minute
	file := strings.TrimPrefix(url, "sqlite:")
	if d == project.SQLite {
		// mode=rw: a missing file is an error, not a new empty database.
		// The pragmas run on every connection the pool makes, since SQLite
		// keeps them per connection. _txlock=immediate: a transaction takes
		// the write lock when it begins, so two that read then write wait
		// for each other instead of one failing with SQLITE_BUSY (D-150).
		driver = "sqlite"
		dsn = "file:" + strings.NewReplacer("%", "%25", "#", "%23").Replace(file) +
			"?mode=rw&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate"
		open, idle, life, idleTime = 4, 4, 0, 0
	}
	if !slices.Contains(sql.Drivers(), driver) {
		return nil, errors.New(`maago: no "` + driver + `" database driver; import _ "` + map[string]string{
			"sqlite": "modernc.org/sqlite", "pgx": "github.com/jackc/pgx/v5/stdlib"}[driver] + `" in package main`)
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, errors.New("maago: cannot open the database: " + diag.OneLine(project.HidePassword(err.Error(), url)))
	}
	db.SetMaxOpenConns(cmp.Or(cfg.MaxOpenConns, open))
	db.SetMaxIdleConns(cmp.Or(cfg.MaxIdleConns, idle))
	db.SetConnMaxLifetime(cmp.Or(cfg.ConnMaxLifetime, life))
	db.SetConnMaxIdleTime(cmp.Or(cfg.ConnMaxIdleTime, idleTime))
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		if d == project.SQLite {
			if _, serr := os.Stat(file); serr != nil {
				wd, _ := os.Getwd()
				return nil, fmt.Errorf("maago: cannot open SQLite file %q: it is not in the working directory %s; run maago migrate, or start the app from the project root",
					file, wd)
			}
			return nil, fmt.Errorf("maago: cannot open SQLite file %q: %s; check that the file and its directory can be written (WAL writes %s-wal and %s-shm)",
				file, diag.OneLine(err.Error()), file, file)
		}
		// The address is left out: it may hold a password.
		return nil, errors.New("maago: cannot open the database: " + diag.OneLine(project.HidePassword(err.Error(), url)))
	}
	return db, nil
}
