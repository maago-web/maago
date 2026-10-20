package web

import (
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/pipeline"
)

// An app whose views take the user needs a test that calls
// maagotest.Permissions (D-156); one without such a view does not.
func TestPermissionsCall(t *testing.T) {
	const call = "package blog\n\nimport (\n\t\"testing\"\n\n\tmt \"github.com/maago-web/maago/maagotest\"\n)\n\nfunc TestPermissions(t *testing.T) { table(t) }\n\nfunc table(t *testing.T) { mt.Permissions(t, nil, nil) }\n"
	for _, c := range []struct {
		name  string
		user  bool
		files map[string]string
		want  string // the fix, or "" for no diagnostic
	}{
		{"no user", false, nil, ""},
		{"no test", true, nil, "write blog/permissions_test.go calling"},
		{"an alias, in a helper", true, map[string]string{"blog/p_test.go": call}, ""},
		{"the _test package", true, map[string]string{"blog/p_test.go": strings.Replace(call, "package blog", "package blog_test", 1)}, ""},
		{"another directory's test", true, map[string]string{"other/p_test.go": call}, "write blog/permissions_test.go calling"},
		{"not a test file", true, map[string]string{"blog/p.go": call}, "write blog/permissions_test.go calling"},
		{"a generated test file", true, map[string]string{"blog/p_test.go": pipeline.Marker + "\n\n" + call}, "write blog/permissions_test.go calling"},
		{"another package's Permissions", true, map[string]string{"blog/p_test.go": strings.Replace(call, `mt "github.com/maago-web/maago/maagotest"`, `mt "example.com/app/mytest"`, 1)}, "write blog/permissions_test.go calling"},
		{"maagotest's other function", true, map[string]string{"blog/p_test.go": strings.Replace(call, "mt.Permissions(t, nil, nil)", "mt.DB(t)", 1)}, "write blog/permissions_test.go calling"},
		{"a test file that does not parse", true, map[string]string{"blog/p_test.go": call + "func {"}, "blog/p_test.go does not parse"},
		{"a dot import", true, map[string]string{"blog/p_test.go": strings.Replace(strings.Replace(call, `mt "`, `. "`, 1), "mt.Permissions", "Permissions", 1)}, "import github.com/maago-web/maago/maagotest by name"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			var files []string
			for name, src := range c.files {
				p := filepath.Join(root, filepath.FromSlash(name))
				os.MkdirAll(filepath.Dir(p), 0o755)
				if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
					t.Fatal(err)
				}
				files = append(files, name)
			}
			slices.Sort(files)
			fset := token.NewFileSet()
			app := &App{Dir: "blog", Name: "blog", Routes: []Route{{Name: "me", view: &view{user: c.user}}, {Name: "home", view: &view{}}}}
			ds := Permissions(fset, root, files, []*App{app})
			var b strings.Builder
			diag.Write(&b, ds)
			switch got := b.String(); {
			case c.want == "" && got != "":
				t.Errorf("got:\n%s", got)
			case c.want != "" && (len(ds) != 1 || !strings.Contains(got, "app blog has routes whose views take the user, but no test calls maagotest.Permissions") || !strings.Contains(got, c.want)):
				t.Errorf("got:\n%s\nwant a fix with %q", got, c.want)
			}
		})
	}
}
