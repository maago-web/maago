package model

import (
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/golden"
	"github.com/maago-web/maago/internal/project"
)

// load writes files into a new project and runs Read and then, if Read
// found no errors, Check, as the pipeline will.
func load(t *testing.T, files map[string]string) ([]*App, string, *token.FileSet) {
	t.Helper()
	root := t.TempDir()
	files["go.mod"] = "module example.com/app\n"
	for p, src := range files {
		path := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gofiles, ds := project.GoFiles(root)
	if len(ds) > 0 {
		t.Fatal(text(ds))
	}
	fset := token.NewFileSet()
	pkgs, ds := Read(fset, root, gofiles)
	if len(ds) > 0 {
		return nil, text(ds), fset
	}
	apps, ds := Check(fset, pkgs)
	return apps, text(ds), fset
}

func text(ds []diag.Diagnostic) string {
	var b strings.Builder
	diag.Write(&b, ds)
	return b.String()
}

// M1 completion condition 1.
func TestMaxlen(t *testing.T) {
	_, errs, _ := load(t, map[string]string{"blog/models.go": `package blog

import "github.com/maago-web/maago"

type Post struct {
	maago.Model

	ID    int64
	Title string ` + "`" + `maago:"maxlen=200"` + "`" + `
}
`})
	want := "blog/models.go:9:23: unknown tag key \"maxlen\" in Post.Title\n  fix: did you mean \"max\"? allowed: max, unique, index, default, choices, label, help, fk\n"
	if errs != want {
		t.Errorf("got:\n%s\nwant:\n%s", errs, want)
	}
}

func TestCheckErrors(t *testing.T) {
	q := "`"
	_, errs, _ := load(t, map[string]string{
		"blog/models.go": `package blog

import (
	"github.com/maago-web/maago"
	"time"
)

type Post struct {
	maago.Model ` + q + `maago:"tabel=posts,ordering=-Titel,unique=(Title,Title),index=Title,unique=(Foo,Bar)"` + q + `

	ID       int64
	Title    string    ` + q + `maago:"max=0,uniqe,label=Title"` + q + `
	Status   string    ` + q + `maago:"choices=a/b/a,default=c"` + q + `
	Count    int64     ` + q + `maago:"max=10,default='1',choices=x"` + q + `
	Rate     float64   ` + q + `maago:"default=1e999"` + q + `
	Small    int32     ` + q + `maago:"default=3000000000"` + q + `
	On       bool      ` + q + `maago:"default=yes"` + q + `
	At       time.Time ` + q + `maago:"default='now'"` + q + `
	Name     string    ` + q + `maago:"default=bob,label='a',label='b'"` + q + `
	Mood     string    ` + q + `maago:"default='x',choices=a/b"` + q + `
	AuthorID int64     ` + q + `maago:"fk=Autor"` + q + `
	OwnerID  string    ` + q + `maago:"fk=Post"` + q + `
	Legacy   int       ` + q + `maago:"index"` + q + `
	secret   string    ` + q + `maago:"label='hidden'"` + q + `
}

type Author struct {
	maago.Model ` + q + `maago:"table=Blog-Authors"` + q + `

	ID   string ` + q + `maago:"unique"` + q + `
	Name string ` + q + `maago:"unique"` + q + `
}

type Tag struct {
	maago.Model ` + q + `maago:"table=blog_post"` + q + `

	Name string
}

type Dup struct {
	maago.Model ` + q + `maago:"unique=(Slug),index=(Slug,ID),index=(Slug,ID),unique=(ID,Slug),unique=(Slug,ID),index=(ID,Slug)"` + q + `

	ID       int64
	Slug     string ` + q + `maago:"unique"` + q + `
	AuthorID int64
	AuthorId int64
}

type Nullable struct {
	maago.Model

	ID *int64
}
`,
		"shop/models.go": `package blog

import "github.com/maago-web/maago"

type Post struct {
	maago.Model

	ID int64 ` + q + `maago:"unique"` + q + `
}
`,
	})
	golden.Check(t, "testdata/errors.golden", []byte(errs))
}

// Read's errors: a syntax error drops its whole package, so the tag error
// in the other file of that package is not reported, while other packages
// are still read; and the ways of embedding maago.Model that are refused.
func TestReadErrors(t *testing.T) {
	q := "`"
	_, errs, _ := load(t, map[string]string{
		"blog/a.go":    "package blog\n\nfunc (\n",
		"blog/b.go":    "package blog\n\nimport \"github.com/maago-web/maago\"\n\ntype Post struct {\n\tmaago.Model\n\tID int64 " + q + "maago:\"max=\"" + q + "\n}\n",
		"shop/c.go":    "package shop\n\nimport \"github.com/maago-web/maago\"\n\ntype Item struct {\n\tmaago.Model\n\tID int64\n\tName string " + q + "maago:\"max=,\"" + q + "\n}\n",
		"shop/d.go":    "package shop\n",
		"mixed/a.go":   "package a\n",
		"mixed/b.go":   "package b\n",
		"broken/x.go":  "package broken\n\ntype T struct {\n\tA int " + q + "maago:\"\n}\n",
		"ptr/p.go":     "package ptr\n\nimport \"github.com/maago-web/maago\"\n\ntype P struct {\n\t*maago.Model\n\tID int64\n}\n",
		"twice/t.go":   "package twice\n\nimport (\n\t\"github.com/maago-web/maago\"\n\tm \"github.com/maago-web/maago\"\n)\n\ntype T struct {\n\tmaago.Model\n\tm.Model\n\tID int64\n}\n",
		"generic/g.go": "package generic\n\nimport \"github.com/maago-web/maago\"\n\ntype G[X any] struct {\n\tmaago.Model\n\tID int64\n}\n",
		"dot/d.go":     "package dot\n\nimport . \"github.com/maago-web/maago\"\n\ntype D struct {\n\tModel\n\tID int64\n}\n",
	})
	golden.Check(t, "testdata/read.golden", []byte(errs))
}

func TestSnake(t *testing.T) {
	for in, want := range map[string]string{
		"ID": "id", "AuthorID": "author_id", "HTTPStatus": "http_status", "PublishedAt": "published_at",
		"Field2": "field2", "OAuth2Token": "o_auth2_token", "Already_Snake": "already_snake", "blog": "blog", "URLs": "urls",
		"IDs": "ids", "UserIDs": "user_ids", "HTTPServer": "http_server", "Class": "class",
	} {
		if got := snake(in); got != want {
			t.Errorf("snake(%q) = %q, want %q", in, got, want)
		}
	}
}
