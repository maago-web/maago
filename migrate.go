package maago

import (
	"context"
	"errors"
	"io/fs"

	"github.com/maago-web/maago/internal/migrate"
	"github.com/maago-web/maago/internal/practicedb"
)

// Migrate applies the migrations of fsys that cfg's database has not
// applied, as maago migrate does, with the same record and the same
// checks: a file changed since it was applied, or an applied one with no
// file, is an error and nothing is applied. fsys holds the
// migrations directory, as //go:embed migrations gives it, so that the
// binary migrates where the CLI is not. main calls it before OpenDB: it
// makes a SQLite file that is not there. Each file applies in a
// transaction of its own that first takes a lock, so two processes that
// start at once apply each file once. It returns the files it applied.
func Migrate(ctx context.Context, cfg DBConfig, fsys fs.FS) ([]string, error) {
	if info, err := fs.Stat(fsys, migrate.Dir); err != nil || !info.IsDir() {
		return nil, errors.New("maago: Migrate: fsys has no " + migrate.Dir + " directory; pass the FS of //go:embed " + migrate.Dir + ", not a sub-FS of it")
	}
	files, ds := migrate.ReadFS(fsys)
	if ds == nil {
		ds = migrate.CheckFiles(files)
	}
	if ds != nil {
		return nil, diagError(ds)
	}
	d, url, _, err := database(cfg)
	if err != nil {
		return nil, err
	}
	db, ds := practicedb.OpenDatabase(ctx, d, url, ".")
	if ds != nil {
		return nil, diagError(ds)
	}
	defer db.Close()
	applied, ds := migrate.Up(ctx, db, d, files)
	if ds != nil {
		return applied, diagError(ds)
	}
	return applied, nil
}
