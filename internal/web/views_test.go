package web

import (
	"fmt"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/golden"
	"github.com/maago-web/maago/internal/gotypes"
	"github.com/maago-web/maago/internal/mapper"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/project"
)

// generate runs what maago gen does for routes on a module of files that
// requires this repository's runtime: the checks, the stub, the type
// check and the files. It returns the diagnostics and the generated files.
func generate(t *testing.T, files map[string]string) (string, map[string][]byte) {
	t.Helper()
	repo, _ := filepath.Abs("../..")
	root := t.TempDir()
	files["go.mod"] = "module " + mod + "\n\ngo 1.25\n\nrequire github.com/maago-web/maago v0.0.0\n\nreplace github.com/maago-web/maago => " + repo + "\n"
	for name, src := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	text := func(ds []diag.Diagnostic) string {
		var b strings.Builder
		diag.Write(&b, ds)
		return b.String()
	}
	gofiles, ds := project.GoFiles(root)
	if ds != nil {
		t.Fatal(text(ds))
	}
	fset := token.NewFileSet()
	apps, ds := Read(fset, Parse(fset, root, gofiles), mod)
	if ds == nil {
		ds = append(Check(fset, apps), Views(fset, apps)...)
		pkgs, mds := model.Read(fset, root, gofiles)
		mapps, cds := model.Check(fset, pkgs)
		ds = append(append(append(ds, mds...), cds...), Forms(fset, apps, func(dir string) *model.App {
			for _, a := range mapps {
				if a.Dir == dir {
					return a
				}
			}
			return nil
		})...)
	}
	for _, app := range apps {
		ds = append(ds, mapper.Names(fset, root, app.Dir, nil, nil, More(fset, app))...)
	}
	if ds != nil {
		return text(ds), nil
	}
	out := map[string][]byte{}
	for _, app := range apps {
		out[path.Join(app.Dir, "urls_gen.go")] = URLsFile(app, true)
	}
	res, ds := gotypes.Check(root, out)
	if ds == nil {
		ds = Types(root, fset, res, apps)
	}
	if ds != nil {
		return text(ds), nil
	}
	for _, app := range apps {
		out[path.Join(app.Dir, "urls_gen.go")] = URLsFile(app, false)
		out[path.Join(app.Dir, "views_gen.go")] = ViewsFile(app, apps)
	}
	// The final files are generated after the type check: they must pass it too.
	if _, ds := gotypes.Check(root, out); ds != nil {
		t.Fatalf("generated code does not type-check:\n%s", text(ds))
	}
	return "", out
}

const site = `package site

import (
	"context"
	"net/http"

	"github.com/maago-web/maago"
)

var Routes = maago.Routes{
	maago.GET("/{$}", "home", Home),
	maago.GET("/posts/{id}", "post_detail", PostDetail),
	maago.GET("/tags/{tag}/{n}", "tag_page", TagPage),
	maago.POST("/posts/{id}", "post_update", PostUpdate, maago.LoginRequired),
	maago.GET("/api/posts/{id}", "api_post", APIPost, maago.API),
	maago.GET("/feed.csv", "feed", Feed),
	maago.GET("/by/{post_id}", "by_post_id", ByPostID),
}

type HomePage struct{ N int }

func Home(ctx context.Context, in struct{}) (HomePage, error) { return HomePage{}, nil }

type PostDetailIn struct {
	ID    int64   ` + "`maago:\"path=id\"`" + `
	Page  int32   ` + "`maago:\"query=page\"`" + `
	Draft *bool   ` + "`maago:\"query=draft\"`" + `
	Q     *string ` + "`maago:\"query=q\"`" + `
	Min   float64 ` + "`maago:\"query=min\"`" + `
}

type PostPage struct{ ID int64 }

func PostDetail(ctx context.Context, in PostDetailIn) (PostPage, error) {
	_ = PostDetailURL(in.ID)
	return PostPage{ID: in.ID}, nil
}

type TagIn struct {
	Tag string ` + "`maago:\"path=tag\"`" + `
	N   int32  ` + "`maago:\"path=n\"`" + `
}

func TagPage(ctx context.Context, in TagIn) (PostPage, error) { return PostPage{}, nil }

type UpdateIn struct {
	ID int64 ` + "`maago:\"path=id\"`" + `
}

func PostUpdate(ctx context.Context, in UpdateIn) (maago.Result[PostPage], error) {
	return maago.SeeOther[PostPage](PostDetailURL(in.ID)), nil
}

func APIPost(ctx context.Context, in UpdateIn) ([]int64, error) { return []int64{in.ID}, nil }

func Feed(w http.ResponseWriter, r *http.Request) {}

type ByIn struct {
	PostID int64 ` + "`maago:\"path=post_id\"`" + `
}

func ByPostID(ctx context.Context, in ByIn) (PostPage, error) { return PostPage{}, nil }
`

// siteFiles is a project with the app site, which main uses, and the
// templates of its pages.
func siteFiles(more ...string) map[string]string {
	files := map[string]string{
		"main.go":                       "package main\n\nimport (\n\t\"github.com/maago-web/maago\"\n\n\t\"example.com/app/site\"\n)\n\nvar _ = []maago.App{site.App}\n\nfunc main() {}\n",
		"site/urls.go":                  site,
		"site/templates/home_page.html": "<p>{{.N}}</p>\n",
		"site/templates/post_page.html": "<p>{{.ID}}</p>\n",
	}
	for i := 0; i+1 < len(more); i += 2 {
		if more[i+1] == "" {
			delete(files, more[i])
		} else {
			files[more[i]] = more[i+1]
		}
	}
	return files
}

func TestGenerate(t *testing.T) {
	out, files := generate(t, siteFiles())
	if out != "" {
		t.Fatal(out)
	}
	golden.Check(t, "testdata/urls_gen.go.golden", files["site/urls_gen.go"])
	golden.Check(t, "testdata/views_gen.go.golden", files["site/views_gen.go"])
}

func TestEscapeOnly(t *testing.T) {
	out, files := generate(t, map[string]string{
		"main.go":      "package main\n\nimport \"example.com/app/feed\"\n\nvar _ = feed.App\n\nfunc main() {}\n",
		"feed/urls.go": "package feed\n\nimport (\n\t\"net/http\"\n\n\t\"github.com/maago-web/maago\"\n)\n\nvar Routes = maago.Routes{maago.GET(\"/f/{name}\", \"feed\", Feed)}\n\nfunc Feed(w http.ResponseWriter, r *http.Request) {}\n",
	})
	if out != "" {
		t.Fatal(out)
	}
	if v := string(files["feed/views_gen.go"]); strings.Contains(v, "\"github.com/maago-web/maago\"") || strings.Contains(v, "embed") {
		t.Errorf("an app of func(w, r) views imports what it does not use:\n%s", v)
	}
	if u := string(files["feed/urls_gen.go"]); !strings.Contains(u, "func FeedURL(name string) string {\n\treturn maago.Prefix() + \"/f/\" + maago.PathSegment(name)") {
		t.Errorf("urls_gen.go:\n%s", u)
	}
}

func TestViewErrors(t *testing.T) {
	view := func(decl string) map[string]string {
		return siteFiles("site/more.go", "package site\n\nimport (\n\t\"context\"\n\n\t\"github.com/maago-web/maago\"\n)\n\nvar _ = maago.API\nvar _ context.Context\n\n"+decl)
	}
	routes := func(table, decls string) map[string]string {
		return siteFiles("site/urls.go", strings.Replace(site, "var Routes = maago.Routes{", "var Routes = maago.Routes{\n\t"+table, 1)+decls)
	}
	cases := []struct {
		name  string
		files map[string]string
		want  []string // the start of each diagnostic
	}{
		// What the syntax shows (Views).
		{"three values", routes(`maago.GET("/a", "a", A),`, "\nfunc A(ctx context.Context, a, b int) (HomePage, error) { return HomePage{}, nil }\n"),
			[]string{"site/urls.go:65:6: view A takes 3 values and returns 2"}},
		{"In of another package", routes(`maago.GET("/a", "a", A),`, "\nfunc A(ctx context.Context, in http.Request) (HomePage, error) { return HomePage{}, nil }\n"),
			[]string{"site/urls.go:65:32: the In of view A is not struct{} or a struct type named in package site"}},
		{"untagged field", routes(`maago.GET("/a", "a", A),`, "\ntype AIn struct{ X int64 }\n\nfunc A(ctx context.Context, in AIn) (HomePage, error) { return HomePage{}, nil }\n"),
			[]string{"site/urls.go:65:18: field X says not where its value comes from"}},
		{"unknown key", routes(`maago.GET("/a", "a", A),`, "\ntype AIn struct{ X int64 `maago:\"qery=x\"` }\n\nfunc A(ctx context.Context, in AIn) (HomePage, error) { return HomePage{}, nil }\n"),
			[]string{"site/urls.go:65:34: unknown key \"qery\" in the tag of X"}},
		{"rest of the path", routes(`maago.GET("/a/{p...}", "a", A),`, "\ntype AIn struct{ P string `maago:\"path=p\"` }\n\nfunc A(ctx context.Context, in AIn) (HomePage, error) { return HomePage{}, nil }\n"),
			[]string{"site/urls.go:11:12: route a reads the rest of the path with {p...}"}},
		{"two fields of one wildcard", routes(`maago.GET("/a/{x}", "a", A),`, "\ntype AIn struct {\n\tX int64 `maago:\"path=x\"`\n\tY int64 `maago:\"path=x\"`\n}\n\nfunc A(ctx context.Context, in AIn) (HomePage, error) { return HomePage{}, nil }\n"),
			[]string{"site/urls.go:67:2: two fields of AIn read path value \"x\""}},
		{"wildcard without a field", routes(`maago.GET("/a/{x}", "a", A),`, "\nfunc A(ctx context.Context, in struct{}) (HomePage, error) { return HomePage{}, nil }\n"),
			[]string{"site/urls.go:11:12: view A reads no {x} of route a"}},
		{"field without a wildcard", routes(`maago.GET("/a", "a", A),`, "\ntype AIn struct{ X int64 `maago:\"path=x\"` }\n\nfunc A(ctx context.Context, in AIn) (HomePage, error) { return HomePage{}, nil }\n"),
			[]string{"site/urls.go:11:12: route a has no {x}, which AIn.X reads"}},
		{"path types", routes(`maago.GET("/a/{x}/{y}/{z}", "a", A),`, "\ntype ID int64\n\ntype AIn struct {\n\tX float64 `maago:\"path=x\"`\n\tY ID `maago:\"path=y\"`\n\tZ *int64 `maago:\"path=z\"`\n}\n\nfunc A(ctx context.Context, in AIn) (HomePage, error) { return HomePage{}, nil }\n"),
			[]string{"site/urls.go:68:4: path value X cannot be read into float64", "site/urls.go:69:4: path value Y cannot be read into ID", "site/urls.go:70:4: path value Z cannot be read into *int64"}},
		{"query types", routes(`maago.GET("/a", "a", A),`, "\ntype AIn struct {\n\tX []string `maago:\"query=x\"`\n\tY **int64 `maago:\"query=y\"`\n}\n\nfunc A(ctx context.Context, in AIn) (HomePage, error) { return HomePage{}, nil }\n"),
			[]string{"site/urls.go:66:4: query value X cannot be read into []string", "site/urls.go:67:4: query value Y cannot be read into **int64"}},
		{"two views wrong, a route name taken", routes(`maago.GET("/a", "a", A), maago.GET("/b", "a", B),`,
			"\ntype AIn struct{ X int64 }\n\nfunc A(ctx context.Context, in AIn) (HomePage, error) { return HomePage{}, nil }\n\nfunc B(ctx context.Context, in struct{}) (HomePage, error) { return HomePage{}, nil }\n"),
			[]string{"site/urls.go:11:43: route name \"a\" is taken by site/urls.go:11", "site/urls.go:65:18: field X says not where its value comes from"}},
		{"names clash", view("func PostDetailURL() {}\n\nvar App int\nvar strconv int\n\nvar maagoPages int\n"),
			[]string{"site/more.go:15:5: strconv is declared at the top level", "site/urls.go:10:5: the routes of app site would declare App, which site/more.go:14 declares already",
				"site/urls.go:10:5: the templates of app site would declare maagoPages", "site/urls.go:12:27: route post_detail would declare PostDetailURL, which site/more.go:12 declares already"}},

		{"In with fields, not named", routes(`maago.GET("/a", "a", A),`, "\nfunc A(ctx context.Context, in struct{ X int64 `maago:\"query=x\"` }) (HomePage, error) { return HomePage{}, nil }\n"),
			[]string{"site/urls.go:65:32: the In of view A is not struct{} or a struct type named in package site"}},
		{"two sources", routes(`maago.GET("/a", "a", A),`, "\ntype AIn struct{ X int64 `maago:\"query=x,path=x\"` }\n\nfunc A(ctx context.Context, in AIn) (HomePage, error) { return HomePage{}, nil }\n"),
			[]string{"site/urls.go:65:42: field X names more than one source"}},
		{"embedded field", routes(`maago.GET("/a", "a", A),`, "\ntype AIn struct{ HomePage }\n\nfunc A(ctx context.Context, in AIn) (HomePage, error) { return HomePage{}, nil }\n"),
			[]string{"site/urls.go:65:18: In of view A embeds a field"}},
		{"generic view", routes(`maago.GET("/a", "a", A),`, "\nfunc A[T any](ctx context.Context, in struct{}) (HomePage, error) { return HomePage{}, nil }\n"),
			[]string{"site/urls.go:65:6: view A has type parameters"}},

		// What the types show (Types).
		{"first not a context", routes(`maago.GET("/a", "a", A),`, "\nfunc A(ctx int, in struct{}) (HomePage, error) { return HomePage{}, nil }\n"),
			[]string{"site/urls.go:65:6: view A takes int first, not a context.Context"}},
		{"second not an error", routes(`maago.GET("/a", "a", A),`, "\nfunc A(ctx context.Context, in struct{}) (HomePage, int) { return HomePage{}, 0 }\n"),
			[]string{"site/urls.go:65:6: view A returns int second, not an error"}},
		{"page unnamed", routes(`maago.GET("/a", "a", A),`, "\nfunc A(ctx context.Context, in struct{}) (struct{}, error) { return struct{}{}, nil }\n"),
			[]string{"site/urls.go:65:6: view A returns struct{}, which names no template"}},
		{"page of another package", routes(`maago.GET("/a", "a", A),`, "\nfunc A(ctx context.Context, in struct{}) (http.Request, error) { return http.Request{}, nil }\n"),
			[]string{"site/urls.go:65:6: view A returns net/http.Request, which names no template"}},
		{"generic page", routes(`maago.GET("/a", "a", A),`, "\ntype G[T any] struct{ X T }\n\nfunc A(ctx context.Context, in struct{}) (G[int], error) { return G[int]{}, nil }\n"),
			[]string{"site/urls.go:67:6: view A returns G[int], which names no template"}},
		{"page not a struct", routes(`maago.GET("/a", "a", A),`, "\ntype P int\n\nfunc A(ctx context.Context, in struct{}) (P, error) { return 0, nil }\n"),
			[]string{"site/urls.go:67:6: view A returns P, which is not a struct"}},
		{"view in a file go build leaves out", map[string]string{
			"main.go":      "package main\n\nimport \"example.com/app/site\"\n\nvar _ = site.App\n\nfunc main() {}\n",
			"site/doc.go":  "package site\n",
			"site/urls.go": "//go:build ignore\n\npackage site\n\nimport (\n\t\"context\"\n\n\t\"github.com/maago-web/maago\"\n)\n\nvar Routes = maago.Routes{maago.GET(\"/a\", \"a\", A)}\n\nfunc A(ctx context.Context, in struct{}) (int, error) { return 0, nil }\n",
		}, []string{"site/urls.go:13:6: view A is in a file go build leaves out"}},
		{"a broken file no page uses", siteFiles("site/templates/base.html", "{{define \"x\"}}{{.N}\n"),
			[]string{"site/templates/base.html:1: bad character U+007D"}},
		{"a template name in other letters", siteFiles("site/templates/home_page.html", "", "site/templates/Home_Page.html", "<p></p>\n"),
			[]string{"site/templates/Home_Page.html: template Home_Page.html is no page's and not base.html", "site/urls.go:22:6: page HomePage has no template site/templates/home_page.html"}},
		{"func(w, r) of other types", routes(`maago.GET("/a", "a", A),`, "\nfunc A(w int, r string) {}\n"),
			[]string{"site/urls.go:65:6: view A takes int and string, not an http.ResponseWriter and an *http.Request"}},
		{"no template", siteFiles("site/templates/home_page.html", ""),
			[]string{"site/urls.go:22:6: page HomePage has no template site/templates/home_page.html"}},
		{"template syntax", siteFiles("site/templates/home_page.html", "<p>\n{{.N}\n"),
			[]string{"site/templates/home_page.html:2: bad character U+007D"}},
		{"a type error first", siteFiles("site/more.go", "package site\n\nvar X int = \"x\"\n", "site/templates/home_page.html", ""),
			[]string{"site/more.go:3:13: cannot use \"x\" (untyped string constant) as int value in variable declaration"}},
	}
	var all strings.Builder
	for _, c := range cases {
		out, _ := generate(t, c.files)
		lines := slices.DeleteFunc(strings.Split(out, "\n"), func(l string) bool { return l == "" || strings.HasPrefix(l, "  fix:") })
		ok := len(lines) == len(c.want)
		for i := 0; ok && i < len(lines); i++ {
			ok = strings.HasPrefix(lines[i], c.want[i])
		}
		if !ok {
			t.Errorf("%s:\ngot\n%s\nwant errors starting %q", c.name, out, c.want)
		}
		fmt.Fprintf(&all, "# %s\n%s\n", c.name, out)
	}
	golden.Check(t, "testdata/views.golden", []byte(all.String()))
}
