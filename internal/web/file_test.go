package web

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/golden"
)

const upload = `package up

import (
	"context"

	"github.com/maago-web/maago"
)

var Routes = maago.Routes{
	maago.GET("/up", "up_form", Show),
	maago.POST("/up", "up_save", Show),
}

type UpForm struct {
	maago.Form
	Title maago.Field[string]
	Doc   maago.Field[maago.File] ` + "`maago:\"accept=png/pdf,label='Document'\"`" + `
}

type UpIn struct {
	Form UpForm ` + "`maago:\"form\"`" + `
}

type UpPage struct{ Form UpForm }

func Show(ctx context.Context, in UpIn) (UpPage, error) { return UpPage{Form: in.Form}, nil }
`

// uploadFiles is the project of the up app, with old replaced by new in
// its urls.go, and its page as given ("" keeps the default).
func uploadFiles(old, new, page string) map[string]string {
	if page == "" {
		page = `<form method="post" enctype="multipart/form-data">{{.Form.Title.Raw}}{{.Form.Doc.Raw}}</form>` + "\n"
	}
	return map[string]string{
		"main.go":                   "package main\n\nimport \"example.com/app/up\"\n\nvar _ = up.App\n\nfunc main() {}\n",
		"up/urls.go":                strings.Replace(upload, old, new, 1),
		"up/templates/up_page.html": page,
	}
}

// A file field: maago.Field[maago.File], accept= of the kinds maago
// knows, read as multipart/form-data by BindFile (D-139).
func TestFileForm(t *testing.T) {
	out, files := generate(t, uploadFiles("", "", ""))
	if out != "" {
		t.Fatal(out)
	}
	if u := string(files["up/urls_gen.go"]); !strings.Contains(u, `Name: "up_save", Files: true, Serve:`) || !strings.Contains(u, `Name: "up_form", Files: true, Serve:`) {
		t.Errorf("urls_gen.go marks no route that reads a file:\n%s", u)
	}
	v := string(files["up/views_gen.go"])
	for _, want := range []string{
		`vals, err := maago.MultipartValues(r, "title", "doc")`,
		`maago.BindString(r.Context(), vals, &f.Title, maago.Rule{Name: "title", Label: "Title"})`,
		`maago.BindFile(r, vals, &f.Doc, maago.Rule{Name: "doc", Label: "Document", Accept: []string{"png", "pdf"}})`,
	} {
		if !strings.Contains(v, want) {
			t.Errorf("views_gen.go has no %s:\n%s", want, v)
		}
	}
	// enctype as HTML lets it be written passes.
	if out, _ := generate(t, uploadFiles("", "", `<FORM ENCTYPE = 'Multipart/Form-Data'></FORM>`+"\n")); out != "" {
		t.Errorf("enctype in capitals and single quotes:\n%s", out)
	}
}

func TestFileFormErrors(t *testing.T) {
	cases := []struct {
		name           string
		old, new, page string
		want           []string
	}{
		{"an unknown kind", "accept=png/pdf", "accept=png/jpg", "", []string{`up/urls.go:17:51: unknown kind "jpg" in accept of form field Doc`}},
		{"accept on text", "Title maago.Field[string]", "Title maago.Field[string] `maago:\"accept=png\"`", "", []string{`up/urls.go:16:36: accept applies to file fields, but form field Title is string`}},
		{"max on a file", "accept=png/pdf,", "max=3,", "", []string{`up/urls.go:17:40: max applies to strings, but form field Doc is maago.File`}},
		{"an API route", `maago.POST("/up", "up_save", Show)`, `maago.POST("/up", "up_save", Show, maago.API)`, "", []string{`up/urls.go:21:2: UpIn.Form reads form UpForm, which has a file, but route up_save is an API route`}},
		{"a model form", "type UpForm struct {\n\tmaago.Form\n", "type Up struct {\n\tmaago.Model\n\tID int64\n\tTitle string\n}\n\ntype UpForm struct {\n\tmaago.Form `maago:\"model=Up\"`\n", "", []string{`up/urls.go:23:8: form field Doc is a file, but a model form takes its fields from model Up, which holds no file`}},
		{"no enctype", "", "", `<form method="post">{{.Form.Title.Raw}}</form>` + "\n", []string{`up/templates/up_page.html: route up_form reads a form with a file, but its page draws no form with enctype="multipart/form-data"`}},
	}
	var all strings.Builder
	for _, c := range cases {
		out, _ := generate(t, uploadFiles(c.old, c.new, c.page))
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
	golden.Check(t, "testdata/file.golden", []byte(all.String()))
}
