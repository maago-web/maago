package build

import (
	"context"
	"errors"
	"io/fs"
	"os"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/migrate"
	"github.com/maago-web/maago/internal/pipeline"
	"github.com/maago-web/maago/internal/practicedb"
	"github.com/maago-web/maago/internal/project"
)

// Migrate applies the migrations the project's database has not applied,
// in number order, and returns their names. It reads no models: whether
// the migrations match them is check's business.
func Migrate(dir string) (applied []string, ds []diag.Diagnostic) {
	ds = onDatabase(dir, func(ctx context.Context, s *state) []diag.Diagnostic {
		db, ds := practicedb.OpenDatabase(ctx, s.d, s.url, s.root)
		if len(ds) > 0 {
			return ds
		}
		defer db.Close()
		applied, ds = migrate.Up(ctx, db, s.d, s.migrations)
		return ds
	})
	return applied, ds
}

// ShowMigrations lists the migrations and which the project's database
// has applied. It only reads the database, and does not create a SQLite
// file that is not there.
func ShowMigrations(dir string) (list string, ds []diag.Diagnostic) {
	ds = onDatabase(dir, func(ctx context.Context, s *state) []diag.Diagnostic {
		records := map[string]string{}
		open := s.d == project.Postgres
		if !open {
			_, err := os.Stat(practicedb.SQLitePath(s.url, s.root))
			open = !errors.Is(err, fs.ErrNotExist)
		}
		if open {
			db, ds := practicedb.OpenDatabase(ctx, s.d, s.url, s.root)
			if len(ds) > 0 {
				return ds
			}
			defer db.Close()
			if records, ds = migrate.Records(ctx, db, s.d); len(ds) > 0 {
				return ds
			}
		}
		if _, ds := migrate.Match(s.migrations, records); len(ds) > 0 {
			return ds
		}
		list = migrate.Show(s.migrations, records)
		return nil
	})
	return list, ds
}

// onDatabase reads the database address and the migrations, checks the
// migrations without a database, then runs f.
func onDatabase(dir string, f func(context.Context, *state) []diag.Diagnostic) []diag.Diagnostic {
	root, ds := project.Root(dir)
	if len(ds) > 0 {
		return ds
	}
	s := &state{root: root}
	return pipeline.Generate(&pipeline.Run{Root: root}, [][]pipeline.Step{
		{
			step(func() (ds []diag.Diagnostic) { s.d, s.url, ds = project.Database(s.root, os.LookupEnv); return }),
			step(func() (ds []diag.Diagnostic) { s.migrations, ds = migrate.ReadFiles(s.root); return }),
		},
		{step(func() []diag.Diagnostic { return migrate.CheckFiles(s.migrations) })},
		{step(func() []diag.Diagnostic { return f(context.Background(), s) })},
	})
}
