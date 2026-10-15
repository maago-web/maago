package web

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/golden"
)

const board = `package board

import (
	"context"
	"time"

	"github.com/maago-web/maago"
)

var Routes = maago.Routes{
	maago.GET("/boards/new", "board_new", BoardNew),
	maago.POST("/boards/new", "board_create", BoardNew),
	maago.POST("/notices", "notice_create", NoticeCreate),
	maago.POST("/api/notices", "api_notice_create", APINotice, maago.API),
}

type Board struct {
	maago.Model

	ID      int64
	Name    string    ` + "`maago:\"max=50,unique,label='이름'\"`" + `
	Kind    string    ` + "`maago:\"choices=free/notice,default='free'\"`" + `
	Created time.Time ` + "`maago:\"auto=create\"`" + `
}

type BoardForm struct {
	maago.Form ` + "`maago:\"model=Board\"`" + `
	Name       maago.Field[string]
	Kind       maago.Field[string] ` + "`maago:\"optional\"`" + `
}

type NoticeForm struct {
	maago.Form
	Title  maago.Field[string]    ` + "`maago:\"min=2,max=200,label='제목',help='짧게'\"`" + `
	Days   maago.Field[int32]     ` + "`maago:\"min_value=1,max_value=30\"`" + `
	Rate   maago.Field[float64]   ` + "`maago:\"optional\"`" + `
	Pinned maago.Field[bool]
	Start  maago.Field[time.Time] ` + "`maago:\"optional\"`" + `
}

func (f *NoticeForm) Validate() {}

type BoardIn struct {
	Form BoardForm ` + "`maago:\"form\"`" + `
}

type BoardPage struct{ Form BoardForm }

func BoardNew(ctx context.Context, in BoardIn) (BoardPage, error) { return BoardPage{in.Form}, nil }

type NoticeIn struct {
	Form NoticeForm ` + "`maago:\"form\"`" + `
}

type NoticePage struct{ Form NoticeForm }

func NoticeCreate(ctx context.Context, in NoticeIn) (maago.Result[NoticePage], error) {
	return maago.Show(NoticePage{in.Form}), nil
}

func APINotice(ctx context.Context, in NoticeIn) (NoticePage, error) { return NoticePage{in.Form}, nil }
`

func boardFiles(old, new string) map[string]string {
	src := board
	if old != "" {
		if !strings.Contains(src, old) {
			panic("no " + old)
		}
		src = strings.Replace(src, old, new, 1)
	}
	return map[string]string{
		"main.go":                          "package main\n\nimport \"example.com/app/board\"\n\nvar _ = board.App\n\nfunc main() {}\n",
		"board/urls.go":                    src,
		"board/templates/board_page.html":  "{{with .Form.Name}}<label>{{.Label}}</label><input name=\"{{.Name}}\" value=\"{{.Raw}}\">{{.Error}}{{end}}{{range .Form.Errors}}{{.}}{{end}}\n",
		"board/templates/notice_page.html": "{{.Form.Title.Raw}}{{.Form.Days.Value}}{{.Form.Pinned.Value}}{{.Form.Start.Value.Year}}{{date \"2006-01-02T15:04\" .Form.Start.Value}}{{if .Form.Valid}}ok{{end}}\n",
	}
}

func TestFormsGen(t *testing.T) {
	out, files := generate(t, boardFiles("", ""))
	if out != "" {
		t.Fatalf("errors:\n%s", out)
	}
	golden.Check(t, "testdata/forms_views_gen.go.golden", files["board/views_gen.go"])
	golden.Check(t, "testdata/forms_forms_gen.go.golden", files["board/forms_gen.go"])
}

func TestFormErrors(t *testing.T) {
	cases := []struct {
		name     string
		old, new string
		want     []string
	}{
		{"unknown tag", "`maago:\"min=2,", "`maago:\"mim=2,", []string{`board/urls.go:34:40: unknown key "mim" in the tag of form field Title`}},
		{"not a Field", "Rate   maago.Field[float64]", "Rate   float64            ", []string{`board/urls.go:36:9: form field Rate is float64, not maago.Field[T]`}},
		{"a type forms do not read", "Rate   maago.Field[float64]", "Rate   maago.Field[uint8]  ", []string{`board/urls.go:36:9: form field Rate is maago.Field[uint8], not maago.Field[T]`}},
		{"unexported", "Rate   maago.Field[float64]", "rate   maago.Field[float64]", []string{`board/urls.go:36:2: form field rate is not exported`}},
		{"min on a number", "`maago:\"min_value=1,max_value=30\"`", "`maago:\"min=1\"`", []string{`board/urls.go:35:40: min applies to strings, but form field Days is int32`}},
		{"min_value on a string", "`maago:\"min=2,max=200,", "`maago:\"min_value=2,max=200,", []string{`board/urls.go:34:40: min_value applies to numbers, but form field Title is string`}},
		{"a value past the type", "max_value=30", "max_value=3000000000", []string{`board/urls.go:35:62: max_value of form field Days is "3000000000", not a int32`}},
		{"min above max", "min=2,max=200", "min=300,max=200", []string{`board/urls.go:34:2: form field Title has min=300 above max=200`}},
		{"optional checkbox", "Pinned maago.Field[bool]", "Pinned maago.Field[bool] `maago:\"optional\"`", []string{`board/urls.go:37:35: form field Pinned is a checkbox, which is never required`}},
		{"choices on a number", "`maago:\"min_value=1,max_value=30\"`", "`maago:\"choices=1/2\"`", []string{`board/urls.go:35:40: choices apply to strings, but form field Days is int32`}},
		{"no such model", "`maago:\"model=Board\"`", "`maago:\"model=Bord\"`", []string{`board/urls.go:27:27: form BoardForm names model Bord, which this package does not declare`}},
		{"no such model field", "\tName       maago.Field[string]\n", "\tNam        maago.Field[string]\n", []string{`board/urls.go:28:2: model Board has no field Nam`}},
		{"a model form's own rule", "Kind       maago.Field[string] `maago:\"optional\"`", "Kind       maago.Field[string] `maago:\"max=3\"`", []string{`board/urls.go:29:41: form field Kind takes its rules from model Board`}},
		{"model type differs", "\tName       maago.Field[string]\n", "\tName       maago.Field[int64]\n", []string{`board/urls.go:28:13: form field Name is maago.Field[int64], but model Board has it as string`}},
		{"auto in a model form", "\tName       maago.Field[string]\n", "\tName       maago.Field[string]\n\tCreated    maago.Field[time.Time]\n", []string{`board/urls.go:29:2: field Created of model Board is not for a form to set`}},
		{"two forms in an In", "\tForm BoardForm `maago:\"form\"`\n}", "\tForm BoardForm `maago:\"form\"`\n\tMore BoardForm `maago:\"form\"`\n}", []string{`board/urls.go:45:2: BoardIn reads a second form, More`}},
		{"not a form", "\tForm NoticeForm `maago:\"form\"`", "\tForm NoticePage `maago:\"form\"`", []string{`board/urls.go:52:2: NoticeIn.Form reads form NoticePage, which does not embed maago.Form`}},
		{"another package's form", "\tForm NoticeForm `maago:\"form\"`", "\tForm time.Time `maago:\"form\"`", []string{`board/urls.go:52:7: form Form is not of a type of this package: time.Time`}},
		{"form with a value", "\tForm NoticeForm `maago:\"form\"`", "\tForm NoticeForm `maago:\"form=x\"`", []string{`board/urls.go:52:26: maago:"form" takes no value`}},
		{"a typo before model", "`maago:\"model=Board\"`", "`maago:\"foo,model=Bord\"`", []string{`board/urls.go:27:21: unknown key "foo" in the tag of maago.Form`, `board/urls.go:27:31: form BoardForm names model Bord`}},
		{"ID in a model form", "\tName       maago.Field[string]\n", "\tName       maago.Field[string]\n\tID         maago.Field[int64]\n", []string{`board/urls.go:29:2: field ID of model Board is not for a form to set`}},
		{"a form on DELETE", `maago.POST("/notices"`, `maago.DELETE("/notices"`, []string{`board/urls.go:52:2: NoticeIn.Form reads a form, but route notice_create is a DELETE`}},
		{"a form on a GET API route", `maago.POST("/api/notices"`, `maago.GET("/api/notices"`, []string{`board/urls.go:52:2: NoticeIn.Form reads a form, but API route api_notice_create is a GET`}},
		{"a generated name taken", "func (f *NoticeForm) Validate() {}", "func (f *NoticeForm) Validate() {}\n\nfunc invalidNoticeForm() {}", []string{`board/urls.go:54:2: form NoticeForm would declare invalidNoticeForm`}},
		{"a pointer form", "\tForm NoticeForm `maago:\"form\"`", "\tForm *NoticeForm `maago:\"form\"`", []string{`board/urls.go:52:7: form Form is a pointer`}},
		{"Validate on the value", "func (f *NoticeForm) Validate() {}", "func (f NoticeForm) Validate() {}", []string{`board/urls.go:41:21: Validate of form NoticeForm has a value receiver`}},
		{"a Valid method", "func (f *NoticeForm) Validate() {}", "func (f *NoticeForm) Validate() {}\n\nfunc (f NoticeForm) Valid() bool { return true }", []string{`board/urls.go:43:21: form NoticeForm declares Valid, which maago gen writes for it`}},
		{"a field named Valid", "Pinned maago.Field[bool]", "Valid  maago.Field[bool]", []string{`board/urls.go:37:2: form field Valid takes the name of the form's method Valid`}},
		{"a form as the In", "in BoardIn) (BoardPage, error) { return BoardPage{in.Form}, nil }", "f *BoardForm) (BoardPage, error) { return BoardPage{*f}, nil }", []string{`board/urls.go:49:38: view BoardNew takes form BoardForm as its In`}},
		{"a form value as the In", "in BoardIn) (BoardPage, error) { return BoardPage{in.Form}, nil }", "f BoardForm) (BoardPage, error) { return BoardPage{f}, nil }", []string{`board/urls.go:49:38: view BoardNew takes form BoardForm as its In`}},
		{"two forms at once", "`maago:\"min=2,", "`maago:\"mim=2,", nil}, // with the next edit too: see below
		{"Validate's signature", "func (f *NoticeForm) Validate() {}", "func (f *NoticeForm) Validate() error { return nil }", []string{`board/urls.go:41:22: Validate of form NoticeForm is not a method func()`}},
	}
	var all strings.Builder
	for _, c := range cases {
		files := boardFiles(c.old, c.new)
		if c.name == "two forms at once" { // D-61: both forms' errors in one run
			files["board/urls.go"] = strings.Replace(files["board/urls.go"], "`maago:\"model=Board\"`", "`maago:\"model=Bord\"`", 1)
			c.want = []string{`board/urls.go:27:27: form BoardForm names model Bord`, `board/urls.go:34:40: unknown key "mim"`}
		}
		out, _ := generate(t, files)
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
	golden.Check(t, "testdata/forms.golden", []byte(all.String()))
}

// A form no In reads yet has Valid too: a view written before its route
// still type-checks.
func TestFormValidUnread(t *testing.T) {
	out, files := generate(t, boardFiles("func BoardNew(", "type DraftForm struct {\n\tmaago.Form\n\tNote maago.Field[string]\n}\n\nfunc draftOK(f DraftForm) bool { return f.Valid() }\n\nfunc BoardNew("))
	if out != "" {
		t.Fatalf("errors:\n%s", out)
	}
	if !strings.Contains(string(files["board/forms_gen.go"]), "func (f DraftForm) Valid() bool {\n\treturn maago.FormValid(f.Form) && f.Note.Error == \"\"\n}") {
		t.Errorf("no Valid of DraftForm:\n%s", files["board/forms_gen.go"])
	}
}

// A model form's field whose model field is a pointer, which may be NULL,
// is optional without saying so.
func TestModelFormPointerOptional(t *testing.T) {
	files := boardFiles("\tCreated time.Time", "\tNote    *string\n\tCreated time.Time")
	files["board/urls.go"] = strings.Replace(files["board/urls.go"], "\tKind       maago.Field[string] `maago:\"optional\"`\n", "\tKind       maago.Field[string] `maago:\"optional\"`\n\tNote       maago.Field[string]\n", 1)
	out, gen := generate(t, files)
	if out != "" {
		t.Fatalf("errors:\n%s", out)
	}
	if want := `maago.Rule{Name: "note", Label: "Note", Optional: true}`; !strings.Contains(string(gen["board/views_gen.go"]), want) {
		t.Errorf("no %s in\n%s", want, gen["board/views_gen.go"])
	}
}
