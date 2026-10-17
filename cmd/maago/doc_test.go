package main

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/build"
	"github.com/maago-web/maago/internal/ref"
	"github.com/maago-web/maago/internal/scaffold"
)

// example splits a topic's example into the commands to run first and
// its files.
func example(t *testing.T, topic string) (cmds []string, files map[string]string) {
	t.Helper()
	src := readFile(t, filepath.Join("docs", topic+".txt"))
	files = map[string]string{}
	var name string
	for line := range strings.Lines(src) {
		switch {
		case strings.HasPrefix(line, "-- ") && strings.HasSuffix(line, " --\n"):
			name = strings.TrimSuffix(strings.TrimPrefix(line, "-- "), " --\n")
			files[name] = ""
		case name != "":
			files[name] += line
		case strings.HasPrefix(line, "$ "):
			cmds = append(cmds, strings.TrimSpace(line[2:]))
		case strings.TrimSpace(line) != "":
			t.Fatalf("%s: line before the first file: %q", topic, line)
		}
	}
	return cmds, files
}

// docProject writes topic's example into a new project, runs its
// commands, and returns the root.
func docProject(t *testing.T, topic string) string {
	t.Helper()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "go.mod"), strings.Replace(readFile(t, "testdata/web/go.mod"), "module example.com/web", "module example.com/app", 1))
	writeFile(t, filepath.Join(root, "go.sum"), readFile(t, "testdata/web/go.sum"))
	writeFile(t, filepath.Join(root, "go.work"), "go 1.25.0\n\nuse .\n\nreplace github.com/maago-web/maago => "+repo+"\n")
	t.Setenv("GOWORK", filepath.Join(root, "go.work"))
	t.Setenv("MAAGO_DATABASE_URL", "sqlite:app.db")
	cmds, files := example(t, topic)
	for _, c := range cmds {
		app, ok := strings.CutPrefix(c, "maago add ")
		if !ok {
			t.Fatalf("%s: cannot run %q", topic, c)
		}
		if _, ds := scaffold.Add(root, app); ds != nil {
			t.Fatal(text(ds))
		}
	}
	for name, src := range files {
		if strings.HasSuffix(name, ".go") {
			if b, err := format.Source([]byte(src)); err != nil || string(b) != src {
				t.Errorf("%s: %s is not gofmt'ed (%v)", topic, name, err)
			}
		}
		writeFile(t, filepath.Join(root, name), src)
	}
	return root
}

// Each topic's example is a project that makemigrations, gen and check
// take, that compiles, and whose tests pass (D-152).
func TestDocExamples(t *testing.T) {
	for _, topic := range ref.Topics {
		t.Run(topic.Name+"/postgres", func(t *testing.T) {
			url := pgURL(t)
			root := docProject(t, topic.Name)
			t.Setenv("MAAGO_DATABASE_URL", url)
			makeMigrations(t, root, "")
			if _, ds := build.Gen(root); ds != nil {
				t.Fatalf("gen:\n%s", text(ds))
			}
			expect(t, root)
		})
		t.Run(topic.Name, func(t *testing.T) {
			root := docProject(t, topic.Name)
			makeMigrations(t, root, "")
			if _, ds := build.Gen(root); ds != nil {
				t.Fatalf("gen:\n%s", text(ds))
			}
			expect(t, root)
			vet := osexec.Command("go", "vet", "./...")
			vet.Dir = root
			vet.Env = append(os.Environ(), "GOFLAGS=-mod=readonly", "GOPROXY=off")
			if out, err := vet.CombinedOutput(); err != nil {
				t.Fatalf("go vet: %v\n%s", err, out)
			}
			goTest(t, root)
		})
	}
}

// readLists are the lists the checkers read as they check.
var readLists = []*ref.List{ref.ModelTags, ref.FieldTags, ref.QueryKeys, ref.InTags, ref.RouteOptions, ref.FormTags}

// The checkers read the table: with any key or value of a topic's lists
// taken out of it, maago check refuses the example that uses it (D-152).
func TestDocKeysAreRead(t *testing.T) {
	for _, l := range readLists {
		if !slices.ContainsFunc(ref.Topics, func(t *ref.Topic) bool { return slices.Contains(t.Lists, l) }) {
			t.Errorf("no topic shows %s", l.Name)
		}
	}
	for _, topic := range ref.Topics {
		var lists []*ref.List
		for _, l := range topic.Lists {
			if slices.Contains(readLists, l) {
				lists = append(lists, l)
			}
		}
		if len(lists) == 0 {
			continue
		}
		t.Run(topic.Name, func(t *testing.T) {
			root := docProject(t, topic.Name)
			makeMigrations(t, root, "")
			if _, ds := build.Gen(root); ds != nil {
				t.Fatalf("gen:\n%s", text(ds))
			}
			refused := func(what string) {
				if ds := build.Check(root); len(ds) == 0 {
					t.Errorf("without %s, check takes the example", what)
				}
			}
			for _, l := range lists {
				items := l.Items
				for i, it := range items {
					l.Items = slices.Delete(slices.Clone(items), i, i+1)
					refused(it.Name)
					l.Items = items
					for j, v := range it.Values {
						l.Items[i].Values = slices.Delete(slices.Clone(it.Values), j, j+1)
						refused(it.Name + "=" + v)
						l.Items[i].Values = it.Values
					}
				}
			}
		})
	}
}

// Every key and value of the closed lists a topic shows is in its example,
// so each is taken by maago gen as maago doc says. The lists of types and
// methods are not: an example holds a few of them.
func TestDocExamplesUseEveryKey(t *testing.T) {
	keyLists := append(slices.Clone(readLists), ref.QueryKinds, ref.TemplateFuncs)
	for _, topic := range ref.Topics {
		_, files := example(t, topic.Name)
		var all strings.Builder
		for _, src := range files {
			all.WriteString(src)
		}
		for _, l := range topic.Lists {
			if !slices.Contains(keyLists, l) {
				continue
			}
			for _, it := range l.Items {
				for _, word := range append([]string{it.Name}, it.Values...) {
					if !regexp.MustCompile(`(^|[^\w])` + regexp.QuoteMeta(word) + `\b`).MatchString(all.String()) {
						t.Errorf("%s: the example does not use %s %q", topic.Name, l.Name, word)
					}
				}
			}
		}
	}
}

// maago doc lists the topics, prints each with every item of its lists,
// and refuses a topic it does not know.
func TestDoc(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"doc"}, &out, &errOut); code != 0 {
		t.Fatalf("maago doc: %d\n%s", code, errOut.String())
	}
	list := out.String()
	for _, topic := range ref.Topics {
		if !strings.Contains(list, "\n  "+topic.Name+" ") {
			t.Errorf("maago doc does not list %s:\n%s", topic.Name, list)
		}
		out.Reset()
		if code := run([]string{"doc", topic.Name}, &out, &errOut); code != 0 {
			t.Fatalf("maago doc %s: %d\n%s", topic.Name, code, errOut.String())
		}
		for _, l := range topic.Lists {
			for _, it := range l.Items {
				if strings.TrimSpace(it.Text) == "" {
					t.Errorf("%s: %s has no text", topic.Name, it.Name)
				}
				_, section, _ := strings.Cut(out.String(), "\n"+l.Name+":\n")
				_, line, found := strings.Cut("\n"+section, "\n  "+it.Name+" ")
				line, _, _ = strings.Cut(line, "\n")
				if !found {
					t.Errorf("maago doc %s does not show %s", topic.Name, it.Name)
				} else if len(it.Values) > 0 && !strings.Contains(line, "Values: "+strings.Join(it.Values, ", ")+".") {
					t.Errorf("maago doc %s shows not the values of %s: %s", topic.Name, it.Name, line)
				}
			}
		}
		if !strings.Contains(out.String(), "\n-- ") {
			t.Errorf("maago doc %s has no example", topic.Name)
		}
	}
	errOut.Reset()
	if code := run([]string{"doc", "modle"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), `did you mean "model"?`) {
		t.Errorf("maago doc modle: %d\n%s", code, errOut.String())
	}
	if code := run([]string{"doc", "model", "query"}, &out, &errOut); code != 2 {
		t.Errorf("maago doc model query: %d", code)
	}
}

// No string of maago's code says (D-xx): what the user reads points at
// maago doc, and the decisions are the design's record (D-152). Comments
// may; maago's tests and test data are not read by users.
func TestNoDecisionsInStrings(t *testing.T) {
	err := filepath.WalkDir("../..", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// The scaffold's testdata is copied into the user's project.
		if d.IsDir() && (d.Name() == "testdata" && !strings.HasSuffix(filepath.ToSlash(p), "internal/scaffold/testdata") || d.Name() == "gentest" || strings.HasPrefix(d.Name(), ".")) && p != "../.." {
			return filepath.SkipDir
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && regexp.MustCompile(`\(D-\d`).MatchString(lit.Value) {
				t.Errorf("%s: %s", fset.Position(lit.Pos()), lit.Value)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Each checker's diagnostics point at its topic: one error made in each
// example, and the see line it gets (D-152, D-62).
func TestDocSee(t *testing.T) {
	for _, c := range []struct{ topic, file, old, new string }{
		{ref.Model, "blog/models.go", "max=200", "maxx=200"},
		{ref.Query, "blog/queries/most_views.sql", ":one int64", ":onee int64"},
		{ref.View, "blog/urls.go", "maago.API)", "maago.APII)"},
		{ref.Form, "blog/forms.go", "min_value=1", "min_valu=1"},
		{ref.Template, "blog/templates/post_detail_page.html", ".Post.Title}}</h1>", ".Post.Titel}}</h1>"},
		{ref.Admin, "main.go", "blog.Admin, ", ""},
	} {
		t.Run(c.topic, func(t *testing.T) {
			root := docProject(t, c.topic)
			makeMigrations(t, root, "")
			p := filepath.Join(root, c.file)
			src := readFile(t, p)
			if !strings.Contains(src, c.old) {
				t.Fatalf("no %q in %s", c.old, c.file)
			}
			writeFile(t, p, strings.Replace(src, c.old, c.new, 1))
			_, ds := build.Gen(root)
			if out := text(ds); len(ds) == 0 || strings.Count(out, "  see: maago doc "+c.topic+"\n") != len(ds) {
				t.Errorf("want every diagnostic to see %s:\n%s", c.topic, out)
			}
		})
	}
}
