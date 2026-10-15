// Package build assembles the stages of maago gen and maago check. It is
// the one place that knows every stage, so the CLI only dispatches (D-68)
// and pipeline stays unaware of models.
package build

import (
	"context"
	"go/ast"
	"go/token"
	"maps"
	"os"
	"path"
	"slices"

	"github.com/maago-web/maago/internal/admingen"
	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/dialect/postgres"
	"github.com/maago-web/maago/internal/dialect/sqlite"
	"github.com/maago-web/maago/internal/gotypes"
	"github.com/maago-web/maago/internal/mapper"
	"github.com/maago-web/maago/internal/migrate"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/pipeline"
	"github.com/maago-web/maago/internal/practicedb"
	"github.com/maago-web/maago/internal/project"
	"github.com/maago-web/maago/internal/schema"
	"github.com/maago-web/maago/internal/web"
)

// Version is this maago's version, which the project's runtime must match
// (D-151). A test sets it.
var Version = project.OwnVersion()

// Gen checks the project holding dir and, if nothing is wrong, writes its
// generated files. It returns the generated files it removed.
func Gen(dir string) (removed []string, ds []diag.Diagnostic) {
	root, ds := project.Root(dir)
	if len(ds) > 0 {
		return nil, ds
	}
	return pipeline.Gen(root, checking(root))
}

// Check checks the project holding dir and that its generated files are up
// to date. It writes nothing.
func Check(dir string) []diag.Diagnostic {
	root, ds := project.Root(dir)
	if len(ds) > 0 {
		return ds
	}
	r := &pipeline.Run{Root: root, Files: map[string][]byte{}}
	if ds := pipeline.Generate(r, checking(root)); len(ds) > 0 {
		return ds
	}
	return pipeline.Check(root, r.Files)
}

// checking is the stages of gen and check: every check, the database's
// included, then the generated files.
func checking(root string) [][]pipeline.Step {
	s := &state{root: root}
	parse := func() []diag.Diagnostic {
		var ds []diag.Diagnostic
		if files, qds := project.QueryFiles(s.root); len(qds) > 0 {
			ds = qds
		} else {
			s.queries, ds = mapper.Read(s.root, files)
		}
		mod, mds := project.ModulePath(s.root)
		if len(mds) > 0 {
			return append(ds, mds...)
		}
		s.mod, s.parsed = mod, web.Parse(s.fset, s.root, s.files)
		apps, wds := web.Read(s.fset, s.parsed, mod)
		s.web = apps
		return append(ds, wds...)
	}
	early := func() (ds []diag.Diagnostic) { // what makemigrations does not need
		ds = web.ReadStatic(s.root, s.parsed, s.web) // before Check, which keeps routes off the static files
		ds = append(append(ds, web.Check(s.fset, s.web)...), web.Views(s.fset, s.web)...)
		ds = append(ds, web.Forms(s.fset, s.web, s.app)...)
		var ads []diag.Diagnostic
		s.admin, ads = admingen.Use(s.fset, s.parsed, s.mod, s.apps, s.web)
		ds = append(ds, ads...)
		s.checked, s.pkgs = map[string][]mapper.Checked{}, map[string]string{}
		for _, qa := range s.queries {
			for _, q := range qa.Queries {
				axes, vs, vds := q.Variants()
				ds = append(ds, vds...)
				s.checked[qa.Dir] = append(s.checked[qa.Dir], mapper.Checked{Query: q, Axes: axes, Variants: vs})
			}
			if app := s.app(qa.Dir); app != nil {
				s.pkgs[qa.Dir] = app.Name
			} else {
				var pds []diag.Diagnostic
				s.pkgs[qa.Dir], pds = mapper.PackageName(s.root, qa.Dir)
				ds = append(ds, pds...)
			}
		}
		// The names every generator declares in each app, checked together.
		dirs := map[string]bool{}
		queries := map[string][]*mapper.Query{}
		routes := map[string]*mapper.More{}
		for _, app := range s.apps {
			dirs[app.Dir] = true
		}
		for _, qa := range s.queries {
			dirs[qa.Dir], queries[qa.Dir] = true, qa.Queries
		}
		for _, app := range s.web {
			dirs[app.Dir], routes[app.Dir] = true, web.More(s.fset, app)
		}
		if s.admin { // admin_gen.go declares Admin in each app with models
			for _, app := range s.apps {
				more := routes[app.Dir]
				if more == nil {
					more = &mapper.More{}
					routes[app.Dir] = more
				}
				more.Names = append(more.Names, mapper.Name{Name: "Admin", By: "the admin", Pos: s.fset.Position(app.Models[0].Pos),
					Fix: "rename what your code declares as Admin; maago generates it for the admin (D-131)"})
				more.Imports = append(more.Imports, admingen.Path)
			}
		}
		for _, dir := range slices.Sorted(maps.Keys(dirs)) {
			ds = append(ds, mapper.Names(s.fset, s.root, dir, s.app(dir), queries[dir], routes[dir])...)
		}
		return ds
	}
	return append(stages(s, parse, early, func() []diag.Diagnostic { return checkDatabase(s) }),
		[]pipeline.Step{func(r *pipeline.Run) []diag.Diagnostic {
			for _, app := range s.apps {
				r.Files[path.Join(app.Dir, "models_gen.go")] = mapper.ModelsFile(s.d, app)
				if s.admin {
					r.Files[path.Join(app.Dir, "admin_gen.go")] = admingen.File(s.d, app, s.apps)
				}
			}
			for dir, qs := range s.checked {
				r.Files[path.Join(dir, "queries_gen.go")] = mapper.QueriesFile(s.d, s.pkgs[dir], qs)
			}
			// The views call the URL functions and the forms' Valid, and main
			// names each App, so they exist before the type check; App gets
			// its value after.
			for _, app := range s.web {
				r.Files[path.Join(app.Dir, "urls_gen.go")] = web.URLsFile(app, true)
				if b := web.FormsFile(app); b != nil {
					r.Files[path.Join(app.Dir, "forms_gen.go")] = b
				}
			}
			return nil
		}},
		// Only a project with routes is type-checked, so only it needs the
		// go command (D-117).
		[]pipeline.Step{func(r *pipeline.Run) []diag.Diagnostic {
			if len(s.web) == 0 {
				return nil
			}
			res, ds := gotypes.Check(s.root, r.Files)
			if len(ds) > 0 {
				return ds
			}
			return web.Types(s.root, s.fset, res, s.web)
		}},
		[]pipeline.Step{func(r *pipeline.Run) []diag.Diagnostic {
			for _, app := range s.web {
				r.Files[path.Join(app.Dir, "urls_gen.go")] = web.URLsFile(app, false)
				r.Files[path.Join(app.Dir, "views_gen.go")] = web.ViewsFile(app, s.web)
			}
			return nil
		}})
}

// state is what the stages of one run hand on, fresh for each run.
type state struct {
	root       string
	fset       *token.FileSet
	apps       []*model.App
	d          project.Dialect
	url, ddl   string
	migrations []migrate.File
	queries    []*mapper.App               // nil for makemigrations
	checked    map[string][]mapper.Checked // each app's queries, by directory
	pkgs       map[string]string           // the package of each app with queries
	files      []string                    // the project's Go files
	mod        string                      // the module path
	parsed     map[string][]*ast.File      // the Go files that are neither tests nor generated, by directory
	admin      bool                        // a file imports contrib/admin (D-131)
	web        []*web.App                  // the apps with routes; nil for makemigrations
}

// app is the app with models at dir, or nil.
func (s *state) app(dir string) *model.App {
	for _, a := range s.apps {
		if a.Dir == dir {
			return a
		}
	}
	return nil
}

// stages returns the stages up to and including the database's. A stage
// runs only if the ones before it found nothing wrong, so none of them
// sees half-finished work (D-61). The stages follow ARCHITECTURE §3:
// reading, parsing, checking without a database, the database check.
// Go syntax comes first, since the models are read from it. parse joins
// the parsing, early the checks without a database; final is the
// database stage.
func stages(s *state, parse, early, final func() []diag.Diagnostic) [][]pipeline.Step {
	s.fset = token.NewFileSet()
	var pkgs []*model.Package
	checks := []pipeline.Step{
		step(func() []diag.Diagnostic { return dialect.CheckNames(s.fset, s.apps, 0) }),
		step(func() []diag.Diagnostic { return migrate.CheckFiles(s.migrations) }),
	}
	if early != nil {
		checks = append(checks, step(early))
	}
	parsing := []pipeline.Step{
		step(func() (ds []diag.Diagnostic) { s.apps, ds = model.Check(s.fset, pkgs); return }),
		step(func() (ds []diag.Diagnostic) { s.d, s.url, ds = project.Database(s.root, os.LookupEnv); return }),
		step(func() (ds []diag.Diagnostic) { s.migrations, ds = migrate.ReadFiles(s.root); return }),
	}
	if parse != nil {
		parsing = append(parsing, step(parse))
	}
	return [][]pipeline.Step{
		{
			step(func() (ds []diag.Diagnostic) { s.files, ds = project.GoFiles(s.root); return }),
			// A runtime of another release breaks what follows in ways that
			// do not point here.
			step(func() []diag.Diagnostic { return project.Version(s.root, Version) }),
		},
		{step(func() (ds []diag.Diagnostic) { pkgs, ds = model.Read(s.fset, s.root, s.files); return })},
		// .env, the migration files and the query files beside the tags:
		// none of them needs another, so none hides another.
		parsing,
		checks,
		// PostgreSQL's name length limit, now that the dialect is known.
		{step(func() (ds []diag.Diagnostic) { s.ddl, ds = dialects[s.d].ddl(s.fset, s.apps); return })},
		{step(final)},
	}
}

// step is a step that needs nothing of the run.
func step(f func() []diag.Diagnostic) pipeline.Step {
	return func(*pipeline.Run) []diag.Diagnostic { return f() }
}

// dialects are what the build needs of each dialect.
var dialects = map[project.Dialect]struct {
	ddl   func(*token.FileSet, []*model.App) (string, []diag.Diagnostic)
	read  func(context.Context, dialect.Querier) (*schema.Schema, []diag.Diagnostic)
	draft func([]*model.App, []migrate.Change) string
}{
	project.SQLite:   {sqlite.DDL, sqlite.Read, sqlite.Draft},
	project.Postgres: {postgres.DDL, postgres.Read, postgres.Draft},
}

// checkDatabase applies the migrations to one practice database and the
// models' DDL to another, and reports how the two differ (D-34, D-64).
func checkDatabase(s *state) []diag.Diagnostic {
	ctx := context.Background()
	a, diff, ds := compared(ctx, s)
	if len(ds) > 0 {
		return ds
	}
	defer a.Close()
	cs, ds := diff()
	if len(ds) > 0 {
		return ds
	}
	if ds := migrate.Diagnose(cs, positions(s.fset, s.apps)); len(ds) > 0 {
		return ds // the queries would fail on what differs
	}
	return checkQueries(ctx, s, a)
}

// checkQueries prepares every query on a, the practice database that
// matches the models, and works out the result types (rule 5, D-23).
// Each query's errors are its own, so all of them are reported, unless
// the connection fails. Then the SQL of the CRUD and of the
// admin (D-134) is prepared: it fails only through a bug of maago, which
// TestCRUDPrepares and the gentest tests pin down, so this is a seat belt.
func checkQueries(ctx context.Context, s *state, a *practicedb.DB) []diag.Diagnostic {
	var ds []diag.Diagnostic
	for dir, checked := range s.checked {
		var qs []*mapper.Query
		var cols [][]practicedb.Column
		for _, c := range checked {
			cs, qds, fatal := mapper.Describe(ctx, a, c.Query, c.Axes, c.Variants)
			if fatal {
				return qds
			}
			ds = append(ds, qds...)
			qs, cols = append(qs, c.Query), append(cols, cs)
		}
		results, rds := mapper.Results(s.apps, dir, qs, cols, func(oid uint32) string { return a.TypeName(ctx, oid) })
		ds = append(ds, rds...)
		for i := range checked {
			checked[i].Result = results[i]
		}
	}
	if len(ds) > 0 {
		return ds
	}
	var sqls []string
	for _, app := range s.apps {
		for _, c := range mapper.CRUDs(app, s.d) {
			for _, st := range []*mapper.Statement{&c.Get, &c.Insert, c.Update, &c.Delete} {
				if st != nil {
					sqls = append(sqls, st.SQL)
				}
			}
		}
		for _, m := range app.Models {
			if s.admin && !m.AdminHidden {
				st := admingen.Statements(m, s.d, s.apps)
				sqls = append(sqls, st.List, st.Count, st.Row)
				for _, ref := range st.Refs {
					sqls = append(sqls, ref.SQL)
				}
				if st.Update != "" {
					sqls = append(sqls, st.Update)
				}
				sqls = append(sqls, st.Exists...)
			}
		}
	}
	for _, sql := range sqls {
		_, err, fatal := a.Describe(ctx, sql)
		if fatal != nil {
			return fatal
		}
		if err != nil {
			return dialect.Bug(err)
		}
	}
	return nil
}

// compared returns the practice database the migrations were applied to,
// which the caller closes, and diff, which reads it and compares it with
// the schema of the models' DDL.
func compared(ctx context.Context, s *state) (*practicedb.DB, func() ([]migrate.Change, []diag.Diagnostic), []diag.Diagnostic) {
	a, ds := applied(ctx, s.d, s.url, s.migrations)
	if len(ds) > 0 {
		return nil, nil, ds
	}
	want, ds := modelSchema(ctx, s.d, s.url, s.ddl)
	if len(ds) > 0 {
		a.Close()
		return nil, nil, ds
	}
	return a, func() ([]migrate.Change, []diag.Diagnostic) {
		got, ds := dialects[s.d].read(ctx, a)
		if len(ds) > 0 {
			return nil, ds
		}
		return migrate.Diff(got, want), nil
	}, nil
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
