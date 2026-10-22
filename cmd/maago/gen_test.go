package main

import (
	"go/token"
	"os"
	"path"
	"slices"
	"testing"

	"github.com/maago-web/maago/internal/admingen"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/dialect/postgres"
	"github.com/maago-web/maago/internal/dialect/sqlite"
	"github.com/maago-web/maago/internal/golden"
	"github.com/maago-web/maago/internal/mapper"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/model/modeltest"
	"github.com/maago-web/maago/internal/practicedb"
	"github.com/maago-web/maago/internal/project"
)

// The fixture apps of internal/gentest: the project root and the app's
// directory in it.
var gentests = []struct{ name, root, dir string }{
	{"orders", "../../internal/testdata/orders", "."},
	{"blog", "../../internal/testdata/example", "blog"},
	{"plain", "../../internal/testdata/plain", "."},
	{"policy", "../../internal/testdata/policy", "."},
}

// The generated files of each fixture app are the files of its gentest
// package, which compiles them and runs them (go test -update writes
// them). Its models.go is the fixture's, and its test is the same in both
// dialects.
func TestGenerated(t *testing.T) {
	for _, d := range []project.Dialect{project.SQLite, project.Postgres} {
		for _, g := range gentests {
			t.Run(string(d)+"/"+g.name, func(t *testing.T) {
				out := path.Join("internal/gentest", string(d), g.name)
				apps, fset := modeltest.Example(t, g.root)
				app := apps[slices.IndexFunc(apps, func(a *model.App) bool { return a.Dir == g.dir })]
				qs, pols := checkedQueries(t, d, g.root, g.dir, fset, apps)
				user := mapper.User{Type: "User"} // the fixtures' user is the app's
				var queries []*mapper.Query
				for _, c := range qs {
					queries = append(queries, c.Query)
				}
				if ds := mapper.Names(fset, g.root, g.dir, app, queries, nil); ds != nil {
					t.Fatalf("names:\n%s", text(ds))
				}
				golden.Check(t, out+"/models_gen.go", mapper.ModelsFile(d, app, pols, user))
				golden.Check(t, out+"/admin_gen.go", admingen.File(d, app, apps, mapper.Guarded(app, pols)))
				if qs != nil {
					golden.Check(t, out+"/queries_gen.go", mapper.QueriesFile(d, app.Name, qs, user))
				} else if _, err := os.Stat(out + "/queries_gen.go"); err == nil {
					t.Errorf("%s/queries_gen.go is left, but the app has no queries", out)
				}
				if readFile(t, out+"/models.go") != readFile(t, path.Join(g.root, g.dir, "models.go")) {
					t.Errorf("%s/models.go is not the fixture's", out)
				}
				if d == project.Postgres {
					test := g.name + "_test.go"
					if readFile(t, out+"/"+test) != readFile(t, path.Join("internal/gentest/sqlite", g.name, test)) {
						t.Errorf("%s/%s is not the same as SQLite's", out, test)
					}
				}
			})
		}
	}
}

// checkedQueries runs the checks of the queries of the app at dir, as a
// run of maago check does, on a practice database with the models'
// tables, and returns them with the app's policies, whose user is the
// app's User. The queries are nil for an app without queries.
func checkedQueries(t *testing.T, d project.Dialect, root, dir string, fset *token.FileSet, apps []*model.App) ([]mapper.Checked, []*mapper.Policy) {
	t.Helper()
	files, ds := project.QueryFiles(root)
	pfiles, pds := project.PolicyFiles(root)
	if ds = append(ds, pds...); ds != nil {
		t.Fatal(text(ds))
	}
	pols, ds := mapper.ReadPolicies(root, pfiles)
	if ds != nil {
		t.Fatalf("policies:\n%s", text(ds))
	}
	qapps, ds := mapper.Read(root, files, pols)
	if ds != nil {
		t.Fatalf("read:\n%s", text(ds))
	}
	var viewer mapper.Viewer
	for _, a := range apps {
		for _, m := range a.Models {
			if m.Name == "User" && a.Dir == dir {
				viewer = mapper.Viewer{Expr: "User", Model: m}
			}
		}
	}
	if pols != nil {
		if ds := mapper.CheckPolicies(apps, pols, qapps, viewer); ds != nil {
			t.Fatalf("policies:\n%s", text(ds))
		}
	}
	i := slices.IndexFunc(qapps, func(a *mapper.App) bool { return a.Dir == dir })
	if i < 0 {
		return nil, pols[dir]
	}
	url, ddl := "", sqlite.DDL
	if d == project.Postgres {
		url, ddl = pgURL(t), postgres.DDL
	}
	db := openPractice(t, d, url)
	src, ds := ddl(fset, apps)
	if ds != nil {
		t.Fatal(text(ds))
	}
	stmts, _ := dialect.Split("", src)
	for _, s := range stmts {
		mustExec(t, db, s.SQL)
	}
	qs := qapps[i].Queries
	var checked []mapper.Checked
	var cols [][]practicedb.Column
	for _, q := range qs {
		axes, vs, ds := q.Variants()
		if ds != nil {
			t.Fatalf("%s:\n%s", q.File, text(ds))
		}
		c, ds, _ := mapper.Describe(ctx(t), db, q, axes, vs)
		if ds != nil {
			t.Fatalf("%s:\n%s", q.File, text(ds))
		}
		checked = append(checked, mapper.Checked{Query: q, Axes: axes, Variants: vs})
		cols = append(cols, c)
	}
	results, ds := mapper.Results(apps, dir, qs, cols, func(oid uint32) string { return db.TypeName(ctx(t), oid) })
	if ds != nil {
		t.Fatalf("results:\n%s", text(ds))
	}
	for i := range checked {
		checked[i].Result = results[i]
	}
	for _, p := range pols[dir] {
		q, v := p.Probe()
		if _, ds, _ := mapper.Describe(ctx(t), db, q, nil, []mapper.Variant{v}); ds != nil {
			t.Fatalf("%s:\n%s", p.File, text(ds))
		}
	}
	return checked, pols[dir]
}
