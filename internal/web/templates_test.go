package web

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/golden"
)

const blog = `package blog

import (
	"context"
	"net/http"
	"time"

	"github.com/maago-web/maago"
)

var Routes = maago.Routes{
	maago.GET("/{$}", "post_list", List),
	maago.GET("/rows", "post_rows", List, maago.Block("rows")),
	maago.GET("/posts/{id}", "post_detail", Detail),
	maago.GET("/feed/{tag}", "feed", Feed),
}

type Author struct{ Name string }

type Post struct {
	ID     int64
	Title  string
	Meta   map[string]string
	Author *Author
	secret string
	제목     string
}

func (p Post) Excerpt() string     { return p.Title }
func (p *Post) PtrMethod() string  { return p.Title }
func (p Post) Words(n int) string  { return p.Title }

type ListPage struct {
	Posts  []Post
	Count  int
	ByName map[string]Post
	All    func(yield func(int, Post) bool)
	Any    any
	Name   string
	Ch     chan Post
	At     time.Time
	AtP    *time.Time
}

func List(ctx context.Context, in struct{}) (ListPage, error) { return ListPage{}, nil }

type DetailIn struct {
	ID int64 ` + "`maago:\"path=id\"`" + `
}

type Key string

type DetailPage struct {
	Post  Post
	PP    **Post
	PS    *[]Post
	ByKey map[Key]Post
}

func Detail(ctx context.Context, in DetailIn) (DetailPage, error) { return DetailPage{}, nil }

func Feed(w http.ResponseWriter, r *http.Request) {}
`

// blogFiles is the project of the blog app, with its templates as given
// after the defaults: name, content pairs; an empty content removes.
func blogFiles(templates ...string) map[string]string {
	files := map[string]string{
		"main.go":                         "package main\n\nimport \"example.com/app/blog\"\n\nvar _ = blog.App\n\nfunc main() {}\n",
		"blog/urls.go":                    blog,
		"blog/templates/list_page.html":   "<ul>{{range .Posts}}<li>{{.Title}}</li>{{end}}</ul>{{block \"rows\" .}}<p>{{.Count}}</p>{{end}}\n",
		"blog/templates/detail_page.html": "<h1>{{.Post.Title}}</h1>\n",
	}
	for i := 0; i+1 < len(templates); i += 2 {
		if templates[i+1] == "" {
			delete(files, "blog/templates/"+templates[i])
		} else {
			files["blog/templates/"+templates[i]] = templates[i+1]
		}
	}
	return files
}

func TestTemplatesPass(t *testing.T) {
	list := `{{$p := .Posts}}{{range $i, $e := $p}}{{$e.Title}}{{$i}}{{$.Count}}{{.Excerpt}}{{.Meta.k}}{{.Author.Name}}{{break}}{{end}}` +
		`{{range .Count}}{{.}}{{end}}{{range $k, $v := .ByName}}{{$k}}{{$v.ID}}{{end}}{{range $i, $p := .All}}{{$p.Title}}{{end}}{{range $i := .All}}{{$i}}{{.}}{{end}}{{range .Ch}}{{.Title}}{{end}}{{range .Posts}}{{.PtrMethod}}{{end}}{{date "2006-01-02" .At}}{{.At | date "2006"}}{{date "15:04" .AtP}}` +
		`{{with .Posts}}{{len .}}{{else}}none{{end}}{{if .Count}}{{.Name}}{{else if .Name}}x{{end}}{{printf "%d" .Count}}` +
		`<a href="{{url "post_detail" 3}}">{{url "post_list"}}</a>{{range .Posts}}<a href="{{url "post_detail" .ID}}"></a>{{end}}{{url "feed" 3}}{{url "feed" "go"}}{{url "feed" .Count}}{{.Count | url "feed"}}` +
		`{{index .Posts 0}}{{.Any}}{{block "rows" .}}<p>{{.Count}}</p>{{end}}{{define "row"}}{{.Title}}{{$.ID}}{{end}}{{range .Posts}}{{template "row" .}}{{end}}`
	detail := `{{.PP.Title}}{{range .PS}}{{.Title}}{{.PtrMethod}}{{end}}`
	if out, _ := generate(t, blogFiles("list_page.html", list+"\n", "detail_page.html", detail+"\n")); out != "" {
		t.Errorf("errors:\n%s", out)
	}
	layout := blogFiles(
		"base.html", "<main>{{block \"content\" .}}{{end}}</main>\n",
		"list_page.html", "{{define \"content\"}}{{.Count}}{{block \"rows\" .}}<p>{{.Count}}</p>{{end}}{{end}}\n",
		"detail_page.html", "{{define \"content\"}}{{.Post.Title}}{{end}}\n")
	if out, _ := generate(t, layout); out != "" {
		t.Errorf("layout:\n%s", out)
	}
}

func TestTemplateErrors(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  []string // the start of each diagnostic
	}{
		{"field", blogFiles("detail_page.html", "<h1>{{.Post.Titel}}</h1>\n"),
			[]string{`blog/templates/detail_page.html:1:13: DetailPage.Post has no field or method "Titel"`}},
		{"in range", blogFiles("list_page.html", "{{range .Posts}}{{.Titel}}{{end}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:20: an element of ListPage.Posts has no field or method "Titel"`}},
		{"$ is the page", blogFiles("list_page.html", "{{range .Posts}}{{$.Cont}}{{end}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:21: ListPage has no field or method "Cont"`}},
		{"variable", blogFiles("list_page.html", "{{$p := .Posts}}{{range $i, $e := $p}}{{$e.Titel}}{{end}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:44: an element of ListPage.Posts has no field or method "Titel"`}},
		{"map element", blogFiles("detail_page.html", "{{.Post.Meta.k.X}}\n"),
			[]string{`blog/templates/detail_page.html:1:16: DetailPage.Post.Meta.k has no field or method "X"`}},
		{"pointer field", blogFiles("detail_page.html", "{{.Post.Author.Nme}}\n"),
			[]string{`blog/templates/detail_page.html:1:16: DetailPage.Post.Author has no field or method "Nme"`}},
		{"pointer receiver", blogFiles("detail_page.html", "{{.Post.PtrMethod}}\n"),
			[]string{`blog/templates/detail_page.html:1:9: method PtrMethod of DetailPage.Post has a pointer receiver`}},
		{"method with arguments", blogFiles("detail_page.html", "{{.Post.Words 3}}\n"),
			[]string{`blog/templates/detail_page.html:1:3: method called with arguments`}},
		{"piped into a method", blogFiles("detail_page.html", "{{.Post.Title | .Post.Excerpt}}\n"),
			[]string{`blog/templates/detail_page.html:1:17: method called with arguments`}},
		{"assigned", blogFiles("list_page.html", "{{$p := .Name}}{{with .Count}}{{$p = $.Posts}}{{end}}{{range $p}}{{.Titel}}{{end}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:69: an element of ListPage.Posts has no field or method "Titel"`}},
		{"unexported", blogFiles("detail_page.html", "{{.Post.secret}}\n"),
			[]string{`blog/templates/detail_page.html:1:9: DetailPage.Post.secret is not exported`}},
		{"with", blogFiles("list_page.html", "{{with .Posts}}{{.Count}}{{end}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:19: ListPage.Posts has no field or method "Count"`}},
		{"template with a value", blogFiles("list_page.html", "{{define \"n\"}}{{.X}}{{end}}{{template \"n\" .Count}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:18: ListPage.Count has no field or method "X"`}},
		{"template without a value", blogFiles("list_page.html", "{{define \"n\"}}{{.Count}}{{end}}{{template \"n\"}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:18: cannot read .Count: {{template}} called this template without a value`}},
		{"after index", blogFiles("list_page.html", "{{(index .Posts 0).Title}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:20: cannot check .Title: the result of index has no type maago can know`}},
		{"an interface", blogFiles("list_page.html", "{{.Any.X}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:8: cannot check .X: ListPage.Any is an interface`}},
		{"an argument", blogFiles("list_page.html", "{{printf \"%d\" .Cont}}{{if .Nme}}{{end}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:16: ListPage has no field or method "Cont"`, `blog/templates/list_page.html:1:28: ListPage has no field or method "Nme"`}},
		{"range over an int", blogFiles("list_page.html", "{{range .Count}}{{.X}}{{end}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:20: an element of ListPage.Count has no field or method "X"`}},
		{"range over a map", blogFiles("list_page.html", "{{range $k, $v := .ByName}}{{$v.Titel}}{{end}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:33: an element of ListPage.ByName has no field or method "Titel"`}},
		{"range over an iterator", blogFiles("list_page.html", "{{range $i, $p := .All}}{{$p.Titel}}{{end}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:30: an element of ListPage.All has no field or method "Titel"`}},
		{"iterator of two, one variable", blogFiles("list_page.html", "{{range $p := .All}}{{$p.Title}}{{.Title}}{{end}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:26: an element of ListPage.All has no field or method "Title"`,
				`blog/templates/list_page.html:1:36: an element of ListPage.All has no field or method "Title"`}},
		{"int with two variables", blogFiles("list_page.html", "{{range $i, $v := .Count}}{{end}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:9: cannot range over ListPage.Count of type int, with two variables`}},
		{"named map key", blogFiles("detail_page.html", "{{.ByKey.x}}\n"),
			[]string{`blog/templates/detail_page.html:1:10: cannot read .x of DetailPage.ByKey: its keys are Key, not string`}},
		{"method past two pointers", blogFiles("detail_page.html", "{{.PP.Excerpt}}\n"),
			[]string{`blog/templates/detail_page.html:1:7: DetailPage.PP has no field or method "Excerpt"`}},
		{"range over a string", blogFiles("list_page.html", "{{range .Name}}{{end}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:9: cannot range over ListPage.Name`}},
		{"no template", blogFiles("list_page.html", "{{template \"nope\" .}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:12: no template "nope"`}},
		{"date", blogFiles("list_page.html", "{{date .Name .At}}{{date \"2006\" .Count}}{{date \"2006\"}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:8: the layout of date is not a string literal`, `blog/templates/list_page.html:1:33: date shows a time.Time, not int`, `blog/templates/list_page.html:1:43: date takes a layout and a time, not 1 values`}},
		{"unknown function", blogFiles("list_page.html", "{{nope .Count}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1: function "nope" not defined`}},

		// {{url}}.
		{"url name", blogFiles("list_page.html", "{{url \"post_detial\" 1}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:7: unknown route name "post_detial"`}},
		{"url values", blogFiles("list_page.html", "{{url \"post_detail\"}}{{url \"post_detail\" \"x\"}}{{url .Name 1}}{{url \"feed\" .At}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:3: route post_detail has 1 wildcards, but url got 0 values`,
				`blog/templates/list_page.html:1:42: {id} of route post_detail is int64, not a string constant`,
				`blog/templates/list_page.html:1:53: cannot check url: the route name is not a string literal`,
				`blog/templates/list_page.html:1:75: {tag} of route feed is string or int, not time.Time`}},

		// Blocks and files.
		{"no block", blogFiles("list_page.html", "{{.Count}}\n"),
			[]string{`blog/urls.go:13:40: template blog/templates/list_page.html has no block "rows"`}},
		{"block of a handler", func() map[string]string {
			f := blogFiles()
			f["blog/urls.go"] = strings.Replace(blog, `"feed", Feed)`, `"feed", Feed, maago.Block("rows"))`, 1)
			return f
		}(), []string{`blog/urls.go:15:2: route feed renders a block, but its view returns no page`}},
		{"escaping", blogFiles("detail_page.html", "<a href=\"{{.Post.Title}}\n"),
			[]string{`blog/templates/detail_page.html: ends in a non-text context`}},
		{"walked before executed", blogFiles("detail_page.html", "<a href=\"{{.Post.Titel}}\n"),
			[]string{`blog/templates/detail_page.html:1:18: DetailPage.Post has no field or method "Titel"`}},
		{"a block that does not escape alone", blogFiles("list_page.html", "{{block \"rows\" .}}<a href=\"{{end}}\">x</a>\n"),
			[]string{`blog/templates/list_page.html: block rows, sent alone, ends in a non-text context`}},
		{"outside define with a layout", blogFiles("base.html", "{{block \"content\" .}}{{end}}\n", "list_page.html", "hello{{define \"content\"}}{{block \"rows\" .}}{{end}}{{end}}\n",
			"detail_page.html", "{{define \"content\"}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:1: content outside {{define}} is never rendered`}},
		{"only defines without a layout", blogFiles("detail_page.html", "{{define \"content\"}}{{.Post.Title}}{{end}}\n"),
			[]string{`blog/templates/detail_page.html: page renders nothing`}},
		{"escaping in a page under a layout", blogFiles("base.html", "<main>{{block \"content\" .}}{{end}}</main>\n",
			"list_page.html", "{{define \"content\"}}"+strings.Repeat("x", 600)+"\n{{if .Count}}<a href=\"{{end}}\">{{block \"rows\" .}}{{end}}{{end}}\n",
			"detail_page.html", "{{define \"content\"}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:2:`}},
		{"a block with the page as its dot", blogFiles("list_page.html", "{{template \"rows\" .Posts}}{{define \"rows\"}}{{range .}}{{.Title}}{{end}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:`}},
		{"a block only a route renders", blogFiles("list_page.html", "{{.Count}}{{define \"rows\"}}{{.Nope}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:31: ListPage has no field or method "Nope"`}},
		{"a directory", blogFiles("sub/x.html", "x\n"),
			[]string{`blog/templates/sub: directory in templates/`}},
		{"an app without pages", func() map[string]string {
			f := blogFiles()
			f["blog/urls.go"] = strings.Replace(blog, "\tmaago.GET(\"/{$}\", \"post_list\", List),\n\tmaago.GET(\"/rows\", \"post_rows\", List, maago.Block(\"rows\")),\n\tmaago.GET(\"/posts/{id}\", \"post_detail\", Detail),\n", "", 1)
			return f
		}(), []string{`blog/templates/detail_page.html: template of an app without HTML pages`, `blog/templates/list_page.html: template of an app without HTML pages`}},
		{"range over an interface", blogFiles("list_page.html", "{{range .Any}}{{end}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/list_page.html:1:9: cannot check what range ranges over: ListPage.Any is an interface`}},
		{"a name of no case", blogFiles("detail_page.html", "{{.Post.제목}}\n"),
			[]string{`blog/templates/detail_page.html:1:9: DetailPage.Post.제목 is not exported`}},
		{"a file of no page and a field", blogFiles("extra.html", "x\n", "detail_page.html", "{{.Post.Titel}}\n"),
			[]string{`blog/templates/detail_page.html:1:9: DetailPage.Post has no field or method "Titel"`, `blog/templates/extra.html: template extra.html is no page's`}},
		{"a file of no page", blogFiles("extra.html", "x\n"),
			[]string{`blog/templates/extra.html: template extra.html is no page's and not base.html`}},
		{"two files, two errors", blogFiles("detail_page.html", "<h1>{{.Post.Titel}}</h1>\n", "list_page.html", "{{url \"post_detial\" 1}}{{block \"rows\" .}}{{end}}\n"),
			[]string{`blog/templates/detail_page.html:1:13: DetailPage.Post has no field or method "Titel"`, `blog/templates/list_page.html:1:7: unknown route name "post_detial"`}},
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
	golden.Check(t, "testdata/templates.golden", []byte(all.String()))
}
