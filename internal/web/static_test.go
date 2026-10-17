package web

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/golden"
)

// withFiles is the blog project with more files, of any kind.
func withFiles(more ...string) map[string]string {
	files := blogFiles()
	for i := 0; i+1 < len(more); i += 2 {
		files[more[i]] = more[i+1]
	}
	return files
}

// What //go:embed static would leave out or fail on, static/ nothing
// serves, and routes in the way of /static/ are errors (D-136, D-137).
func TestStaticDiags(t *testing.T) {
	routes := func(extra string) string {
		return strings.Replace(blog, "\tmaago.GET(\"/feed/{tag}\", \"feed\", Feed),\n", "\tmaago.GET(\"/feed/{tag}\", \"feed\", Feed),\n"+extra, 1)
	}
	cases := []struct {
		name  string
		files map[string]string
		want  []string // the start of each diagnostic
	}{
		{"a directory", withFiles("blog/static/app.css", "x", "blog/static/img/a.png", "x"),
			[]string{"blog/static/img: directory in static/"}},
		{"names embed leaves out or a URL escapes", withFiles("blog/static/.keep", "", "blog/static/_draft.css", "x", "blog/static/a b.css", "x", "blog/static/a:b.css", "x", "blog/static/app.css", "x"),
			[]string{`blog/static/.keep: static file name ".keep" does not start with a letter or digit`, `blog/static/_draft.css: static file name "_draft.css"`,
				`blog/static/a b.css: static file name "a b.css"`, `blog/static/a:b.css: static file name "a:b.css"`}},
		{"no file", func() map[string]string {
			f := blogFiles()
			f["blog/static/x/y"] = "" // a directory, so static/ exists with no file
			return f
		}(), []string{"blog/static: static/ holds no file to serve", "blog/static/x: directory in static/"}},
		{"a package without routes", withFiles("lib/lib.go", "package lib\n", "lib/static/app.css", "x", "static/app.css", "x"),
			[]string{"lib/static: static/ of a package without routes", "static: static/ of a package without routes"}},
		{"two packages of one name", withFiles("blog/static/a.css", "x", "more/blog/urls.go", "package blog\n\nimport (\n\t\"net/http\"\n\n\t\"github.com/maago-web/maago\"\n)\n\nvar Routes = maago.Routes{maago.GET(\"/more\", \"more\", More)}\n\nfunc More(w http.ResponseWriter, r *http.Request) {}\n",
			"more/blog/static/b.css", "x", "main.go", "package main\n\nimport (\n\t\"example.com/app/blog\"\n\tmore \"example.com/app/more/blog\"\n)\n\nvar _, _ = blog.App, more.App\n\nfunc main() {}\n"),
			[]string{"more/blog/static: apps blog and more/blog are both package blog, so both would serve /static/blog/"}},
		{"routes under /static/", withFiles("blog/urls.go", routes("\tmaago.GET(\"/static/x\", \"x\", Feed),\n\tmaago.GET(\"/static\", \"y\", Feed),\n")),
			[]string{`blog/urls.go:16:12: route path "/static/x" is under /static/`, `blog/urls.go:17:12: route path "/static" is under /static/`}},
		{"a route the static files' pattern conflicts with", withFiles("blog/static/a.css", "x", "blog/urls.go", routes("\tmaago.GET(\"/{x}/blog/a.css\", \"x\", Feed),\n")),
			[]string{`blog/urls.go:16:12: route GET /{x}/blog/a.css conflicts with /static/blog/{name}`}},
		{"a name the static files' code declares", withFiles("blog/static/a.css", "x", "blog/more.go", "package blog\n\nvar maagoStatic int\n"),
			[]string{"blog/urls.go:11:5: the static files of app blog would declare maagoStatic, which blog/more.go:3 declares already"}},
		{"no static/, no pattern", withFiles("blog/urls.go", routes("\tmaago.GET(\"/{x}/blog/a.css\", \"x\", Feed),\n")), nil},
	}
	var all strings.Builder
	for _, c := range cases {
		out, _ := generate(t, c.files)
		lines := slices.DeleteFunc(strings.Split(out, "\n"), func(l string) bool { return l == "" || strings.HasPrefix(l, "  ") })
		ok := len(lines) == len(c.want)
		for i := 0; ok && i < len(lines); i++ {
			ok = strings.HasPrefix(lines[i], c.want[i])
		}
		if !ok {
			t.Errorf("%s:\ngot\n%s\nwant errors starting %q", c.name, out, c.want)
		}
		fmt.Fprintf(&all, "# %s\n%s\n", c.name, out)
	}
	golden.Check(t, "testdata/static.golden", []byte(all.String()))
}

// A link in static/ is no file //go:embed takes.
func TestStaticLink(t *testing.T) {
	files := withFiles("blog/static/app.css", "x")
	out, _ := generateWith(t, files, func(root string) {
		if err := os.Symlink("app.css", filepath.Join(root, "blog/static/link.css")); err != nil {
			t.Skip(err)
		}
	})
	if !strings.HasPrefix(out, "blog/static/link.css: not a regular file") {
		t.Errorf("got\n%s", out)
	}
}

// {{static}} names an app by its package and a file of its static/; a
// template of another app may ask for it (D-137, M7's condition 2).
func TestStaticTemplates(t *testing.T) {
	site := "package site\n\nimport (\n\t\"context\"\n\n\t\"github.com/maago-web/maago\"\n)\n\nvar Routes = maago.Routes{maago.GET(\"/s\", \"s\", S)}\n\ntype SPage struct{}\n\nfunc S(ctx context.Context, in struct{}) (SPage, error) { return SPage{}, nil }\n"
	main := "package main\n\nimport (\n\t\"example.com/app/blog\"\n\t\"example.com/app/site\"\n)\n\nvar _, _ = blog.App, site.App\n\nfunc main() {}\n"
	two := func(detail, s string) map[string]string {
		return withFiles("blog/static/app.css", "x", "blog/static/logo.png", "x", "blog/templates/detail_page.html", detail,
			"site/urls.go", site, "site/templates/s_page.html", s, "main.go", main)
	}
	cases := []struct {
		name, detail, s string
		want            string
	}{
		{"no file", `{{static "blog/ap.css"}}`, `x`, `blog/templates/detail_page.html:1:10: static "blog/ap.css": no file blog/static/ap.css
  fix: did you mean "blog/app.css"?`},
		{"no file, from another app", `x`, `{{static "blog/nope.css"}}`, `site/templates/s_page.html:1:10: static "blog/nope.css": no file blog/static/nope.css`},
		{"no app", `{{static "blgo/app.css"}}`, `x`, `blog/templates/detail_page.html:1:10: static "blgo/app.css": no app blgo with static files
  fix: did you mean "blog"?`},
		{"an app without static/", `{{static "site/app.css"}}`, `x`, `blog/templates/detail_page.html:1:10: static "site/app.css": no app site with static files`},
		{"no app named", `{{static "app.css"}}`, `x`, `blog/templates/detail_page.html:1:10: static "app.css" names no app`},
		{"not a literal", `{{static .Post.Title}}`, `x`, `blog/templates/detail_page.html:1:15: cannot check static: the name is not a string literal`},
		{"two values", `{{static "blog/app.css" 1}}`, `x`, `blog/templates/detail_page.html:1:3: static takes the name of a file, not 2 values`},
	}
	for _, c := range cases {
		if out, _ := generate(t, two(c.detail+"\n", c.s+"\n")); !strings.HasPrefix(out, c.want) {
			t.Errorf("%s:\ngot\n%s\nwant\n%s", c.name, out, c.want)
		}
	}

	out, files := generate(t, two(`<link href="{{static "blog/app.css"}}">`+"\n", `<img src="{{static "blog/logo.png"}}">{{static "blog/app.css"}}`+"\n"))
	if out != "" {
		t.Fatal(out)
	}
	views, urls := string(files["blog/views_gen.go"]), string(files["blog/urls_gen.go"])
	if !strings.Contains(views, "//go:embed static\nvar maagoStaticFiles embed.FS\n") ||
		!strings.Contains(views, `var maagoStatic = rt.Static("blog", maagoStaticFiles, "app.css", "logo.png")`) ||
		!strings.Contains(views, `"static": rt.StaticFunc}`) || !strings.Contains(urls, `rt.NewApp("example.com/app/blog", maagoStatic,`) {
		t.Errorf("blog:\n%s\n%s", views, urls)
	}
	if s := string(files["site/views_gen.go"]); strings.Contains(s, "maagoStatic") || !strings.Contains(s, `"static": rt.StaticFunc}`) ||
		strings.Contains(string(files["site/urls_gen.go"]), "maagoStatic") {
		t.Errorf("site:\n%s", s)
	}
}

// An app of func(w, r) views only with static/ imports what serving the
// files needs.
func TestEscapeOnlyStatic(t *testing.T) {
	out, files := generate(t, map[string]string{
		"main.go":             "package main\n\nimport \"example.com/app/feed\"\n\nvar _ = feed.App\n\nfunc main() {}\n",
		"feed/urls.go":        "package feed\n\nimport (\n\t\"net/http\"\n\n\t\"github.com/maago-web/maago\"\n)\n\nvar Routes = maago.Routes{maago.GET(\"/f/{name}\", \"feed\", Feed)}\n\nfunc Feed(w http.ResponseWriter, r *http.Request) {}\n",
		"feed/static/app.css": "x",
	})
	if out != "" {
		t.Fatal(out)
	}
	if v := string(files["feed/views_gen.go"]); !strings.Contains(v, "\t\"embed\"\n") || !strings.Contains(v, "\t\"github.com/maago-web/maago/rt\"\n") ||
		!strings.Contains(v, `var maagoStatic = rt.Static("feed", maagoStaticFiles)`) {
		t.Errorf("views_gen.go:\n%s", v)
	}
}
