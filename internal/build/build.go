// Package build assembles the stages of maago gen and maago check. It is
// the one place that knows every stage, so the CLI only dispatches (D-68)
// and pipeline stays unaware of models.
package build

import (
	"context"
	"go/token"
	"maps"
	"os"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/dialect/postgres"
	"github.com/maago-web/maago/internal/dialect/sqlite"
	"github.com/maago-web/maago/internal/migrate"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/pipeline"
	"github.com/maago-web/maago/internal/practicedb"
	"github.com/maago-web/maago/internal/project"
	"github.com/maago-web/maago/internal/schema"
)

// Gen checks the project holding dir and, if nothing is wrong, writes its
// generated files. It returns the generated files it removed.
func Gen(dir string) (removed []string, ds []diag.Diagnostic) {
	root, ds := project.Root(dir)
	if len(ds) > 0 {
		return nil, ds
	}
	return pipeline.Gen(root, stages(root))
}

// Check checks the project holding dir and that its generated files are up
// to date. It writes nothing.
func Check(dir string) []diag.Diagnostic {
	root, ds := project.Root(dir)
	if len(ds) > 0 {
		return ds
	}
	r := &pipeline.Run{Root: root, Files: map[string][]byte{}}
	if ds := pipeline.Generate(r, stages(root)); len(ds) > 0 {
		return ds
	}
	return pipeline.Check(root, r.Files)
}

// stages returns fresh stages for one run. What a stage hands to the next
// lives in these variables, so each run starts clean. A stage runs only if
// the ones before it found nothing wrong, so none of them sees
// half-finished work (D-61). The stages follow ARCHITECTURE §3: reading,
// parsing, checking without a database, the database check, generating.
// Go syntax comes first, since the models are read from it.
func stages(root string) [][]pipeline.Step {
	fset := token.NewFileSet()
	var files []string
	var pkgs []*model.Package
	var apps []*model.App
	var d project.Dialect
	var url, ddl string
	var migrations []migrate.File
	step := func(f func() []diag.Diagnostic) pipeline.Step {
		return func(*pipeline.Run) []diag.Diagnostic { return f() }
	}
	return [][]pipeline.Step{
		{step(func() (ds []diag.Diagnostic) { files, ds = project.GoFiles(root); return })},
		{step(func() (ds []diag.Diagnostic) { pkgs, ds = model.Read(fset, root, files); return })},
		// .env and the migration files beside the tags: none of them
		// needs another, so none hides another.
		{
			step(func() (ds []diag.Diagnostic) { apps, ds = model.Check(fset, pkgs); return }),
			step(func() (ds []diag.Diagnostic) { d, url, ds = project.Database(root, os.LookupEnv); return }),
			step(func() (ds []diag.Diagnostic) { migrations, ds = migrate.ReadFiles(root); return }),
		},
		{
			step(func() []diag.Diagnostic { return dialect.CheckNames(fset, apps, 0) }),
			step(func() []diag.Diagnostic { return migrate.CheckFiles(migrations) }),
		},
		// PostgreSQL's name length limit, now that the dialect is known.
		{step(func() (ds []diag.Diagnostic) { ddl, ds = dialects[d].ddl(fset, apps); return })},
		{step(func() []diag.Diagnostic { return checkDatabase(d, url, migrations, ddl, positions(fset, apps)) })},
		{func(r *pipeline.Run) []diag.Diagnostic { maps.Copy(r.Files, model.Files(apps)); return nil }},
	}
}

// dialects are what the build needs of each dialect.
var dialects = map[project.Dialect]struct {
	ddl  func(*token.FileSet, []*model.App) (string, []diag.Diagnostic)
	read func(context.Context, dialect.Querier) (*schema.Schema, []diag.Diagnostic)
}{
	project.SQLite:   {sqlite.DDL, sqlite.Read},
	project.Postgres: {postgres.DDL, postgres.Read},
}

// checkDatabase applies the migrations to one practice database and the
// models' DDL to another, and reports how the two differ (D-34, D-64).
func checkDatabase(d project.Dialect, url string, files []migrate.File, ddl string, pos func(table, column string) token.Position) []diag.Diagnostic {
	ctx := context.Background()
	a, ds := applied(ctx, d, url, files)
	if len(ds) > 0 {
		return ds
	}
	defer a.Close()
	want, ds := modelSchema(ctx, d, url, ddl)
	if len(ds) > 0 {
		return ds
	}
	got, ds := dialects[d].read(ctx, a)
	if len(ds) > 0 {
		return ds
	}
	return migrate.Diagnose(migrate.Diff(got, want), pos)
}

// applied opens a practice database and applies the migrations to it.
// The caller closes it.
func applied(ctx context.Context, d project.Dialect, url string, files []migrate.File) (*practicedb.DB, []diag.Diagnostic) {
	db, ds := practicedb.Open(ctx, d, url)
	if len(ds) > 0 {
		return nil, ds
	}
	if ds := migrate.Apply(ctx, db, d, files); len(ds) > 0 {
		db.Close()
		return nil, ds
	}
	return db, nil
}

// modelSchema reads the schema the models' DDL makes in a practice
// database of its own.
func modelSchema(ctx context.Context, d project.Dialect, url, ddl string) (*schema.Schema, []diag.Diagnostic) {
	db, ds := practicedb.Open(ctx, d, url)
	if len(ds) > 0 {
		return nil, ds
	}
	defer db.Close()
	stmts, _ := dialect.Split("", ddl)
	for _, st := range stmts {
		rows, err := db.QueryContext(ctx, st.SQL) // the models' DDL failing is maago's fault
		if err != nil {
			return nil, dialect.Bug(err)
		}
		rows.Close()
	}
	return dialects[d].read(ctx, db)
}

// positions finds where the models have a table and its column: the
// field, or the model if the field is gone, or nothing if the model is.
func positions(fset *token.FileSet, apps []*model.App) func(table, column string) token.Position {
	return func(table, column string) token.Position {
		for _, app := range apps {
			for _, m := range app.Models {
				if m.Table != table {
					continue
				}
				for _, f := range m.Fields {
					if f.Column == column {
						return fset.Position(f.Pos)
					}
				}
				return fset.Position(m.Pos)
			}
		}
		return token.Position{}
	}
}
