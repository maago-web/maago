package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	// gen and check need a project and a database (D-64); an empty
	// module has nothing to do.
	t.Setenv("MAAGO_DATABASE_URL", "sqlite:app.db")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	const toStdout, toStderr = true, false
	tests := []struct {
		args     []string
		code     int
		toStdout bool
		want     []string // substrings of the non-empty stream
	}{
		{nil, 2, toStderr, []string{"usage:", "gen", "check"}},
		{[]string{"nope"}, 2, toStderr, []string{`unknown command "nope"`, "usage:", "gen", "check"}},
		{[]string{"help"}, 0, toStdout, []string{"usage:", "gen", "check"}},
		{[]string{"-h"}, 0, toStdout, []string{"usage:"}},
		{[]string{"--help"}, 0, toStdout, []string{"usage:"}},
		{[]string{"gen", "-h"}, 0, toStdout, []string{"usage:"}},
		{[]string{"gen", "-x"}, 2, toStderr, []string{"-x", "usage:"}},
		{[]string{"gen", "extra"}, 2, toStderr, []string{`"extra"`, "usage:"}},
		{[]string{"gen", "--"}, 0, toStdout, nil},
		{[]string{"makemigrations", "-h"}, 0, toStdout, []string{"usage:", "makemigrations"}},
		{[]string{"makemigrations", "--nope"}, 2, toStderr, []string{"-nope", "usage:"}},
		{[]string{"migrate", "extra"}, 2, toStderr, []string{`"extra"`, "usage:"}},
		{[]string{"showmigrations", "extra"}, 2, toStderr, []string{`"extra"`, "usage:"}},
		{[]string{"makemigrations", "--name"}, 2, toStderr, []string{"needs an argument", "usage:"}},
		{[]string{"makemigrations", "x", "--name", "y"}, 2, toStderr, []string{`"x"`, "usage:"}},
		{[]string{"makemigrations"}, 0, toStdout, []string{"no changes"}},
		// Both values reach makemigrations, which reports each (D-68: 1, not 2).
		{[]string{"makemigrations", "--rename", "bad", "--rename=worse"}, 1, toStderr, []string{`--rename "bad"`, `--rename "worse"`}},
		{[]string{"showmigrations"}, 0, toStdout, []string{"no migrations"}},
		{[]string{"migrate"}, 0, toStdout, []string{"nothing to apply"}},
		// add takes one app; it writes into this project, so it comes last.
		{[]string{"add"}, 2, toStderr, []string{"missing argument", "usage:"}},
		{[]string{"add", "auth", "x"}, 2, toStderr, []string{`"x"`, "usage:"}},
		{[]string{"add", "autth"}, 1, toStderr, []string{`unknown app "autth"`, `did you mean "auth"?`}},
		{[]string{"add", "auth"}, 0, toStdout, []string{"wrote auth/models.go"}},
		{[]string{"add", "auth"}, 1, toStderr, []string{"auth/models.go: something is in the way of maago add auth"}},
		{[]string{""}, 2, toStderr, []string{`unknown command ""`, "usage:"}},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)
			if code != tt.code {
				t.Errorf("code = %d, want %d", code, tt.code)
			}
			out, other := stderr.String(), stdout.String()
			if tt.toStdout {
				out, other = other, out
			}
			if other != "" {
				t.Errorf("wrong stream got output:\n%s", other)
			}
			if n := strings.Count(out, "usage:"); n > 1 {
				t.Errorf("usage printed %d times:\n%s", n, out)
			}
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("output lacks %q:\n%s", w, out)
				}
			}
		})
	}
}

// The usage text is what agents read first; lock it, including command order.
func TestUsage(t *testing.T) {
	var stdout bytes.Buffer
	run([]string{"help"}, &stdout, &bytes.Buffer{})
	const want = `usage: maago <command> [flags]

commands:
  gen            check the project, then write generated code
  check          check the project and that generated code is up to date
  makemigrations write the next migration from the models; --name NAME, --rename TABLE.OLD:NEW (repeatable)
  showmigrations list the migrations and which the database has applied
  migrate        apply the migrations the database has not applied
  add            copy contrib app APP into the project, yours from then on (apps: auth)
  tailwind       write each app's static/tailwind.css from its tailwind.css, with the standalone Tailwind CSS v4.3.3
`
	if got := stdout.String(); got != want {
		t.Errorf("usage:\n%s\nwant:\n%s", got, want)
	}
}

// The whole command on the shared example: gen, check, a hand edit, the
// maxlen mistake (M1 completion condition 1), and a run from a subdirectory.
func TestGenCheck(t *testing.T) {
	root := exampleProject(t)
	maago := func(dir string, args ...string) (int, string, string) {
		t.Chdir(dir)
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}
	blog := filepath.Join(root, "blog")

	if code, out, errs := maago(root, "gen"); code != 0 || out != "generated 1 file\n" || errs != "" {
		t.Fatalf("gen: %d %q %q", code, out, errs)
	}
	if code, _, errs := maago(blog, "check"); code != 0 {
		t.Fatalf("check from blog/: %d %s", code, errs)
	}

	gen := filepath.Join(blog, "models_gen.go")
	b, _ := os.ReadFile(gen)
	os.WriteFile(gen, append(b, "// edited\n"...), 0o644)
	if code, _, errs := maago(blog, "check"); code != 1 || !strings.Contains(errs, "blog/models_gen.go: generated file is out of date") {
		t.Errorf("check after an edit: %d %s", code, errs)
	}

	stale := filepath.Join(root, "old_gen.go")
	os.WriteFile(stale, []byte("// Code generated by maago. DO NOT EDIT.\n\npackage app\n"), 0o644)
	if code, out, _ := maago(root, "gen"); code != 0 || out != "removed old_gen.go\ngenerated 1 file\n" {
		t.Errorf("gen with a stale file: %d %q", code, out)
	}

	models := filepath.Join(blog, "models.go")
	src, _ := os.ReadFile(models)
	os.WriteFile(models, bytes.Replace(src, []byte("max=200"), []byte("maxlen=200"), 1), 0o644)
	code, _, errs := maago(blog, "gen")
	if code != 1 || !strings.Contains(errs, `blog/models.go:15:32: unknown tag key "maxlen" in Post.Title`) || !strings.Contains(errs, `did you mean "max"?`) {
		t.Errorf("gen with maxlen: %d %s", code, errs)
	}
}

// The migration commands as a project uses them.
func TestMigrationCommands(t *testing.T) {
	root := draftSQLite.makeProject(t, makeBase)
	t.Chdir(root)
	maago := func(args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}
	if code, out, errs := maago("makemigrations"); code != 0 || out != "created migrations/0001_initial.sql\n" {
		t.Fatalf("makemigrations: %d %q %q", code, out, errs)
	}
	if code, out, _ := maago("showmigrations"); code != 0 || out != "[ ] 0001_initial.sql\n0 of 1 applied\n" {
		t.Errorf("showmigrations: %d %q", code, out)
	}
	if code, out, errs := maago("migrate"); code != 0 || out != "applied 0001_initial.sql\n" {
		t.Errorf("migrate: %d %q %q", code, out, errs)
	}
	if code, out, _ := maago("migrate"); code != 0 || out != "nothing to apply\n" {
		t.Errorf("migrate again: %d %q", code, out)
	}
	// The draft's warnings come to the terminal too, where they are read.
	setModels(t, root, strings.Replace(makeBase, "	Text     string ", "	Body     string ", 1))
	code, out, errs := maago("makemigrations")
	if code != 0 || out != "created migrations/0002_blog_post.sql\n" || !strings.HasPrefix(errs, "migrations/0002_blog_post.sql:1: ⚠ Column \"text\" of \"blog_post\" is dropped") || !strings.Contains(errs, "run maago makemigrations --rename blog_post.text:body\n") || strings.Count(errs, "\n") != 3 {
		t.Errorf("makemigrations of a rename: %d %q %q", code, out, errs)
	}
	// migrate refuses the draft, and its fix says the rename first: following
	// it does not lose the values.
	if code, _, errs := maago("migrate"); code != 1 || !strings.Contains(errs, "fix: if a column was renamed, delete the file and run maago makemigrations --rename blog_post.text:NEW;") {
		t.Errorf("migrate of the rename's draft: %d %q", code, errs)
	}
	os.Remove(filepath.Join(root, "migrations", "0002_blog_post.sql"))
	setModels(t, root, makeBase)
	if code, _, errs := maago("makemigrations", "--name", "Bad"); code != 1 || !strings.Contains(errs, `--name "Bad"`) {
		t.Errorf("makemigrations --name Bad: %d %q", code, errs)
	}
	// A failing file stops migrate; what it applied before is still said.
	writeFile(t, filepath.Join(root, "migrations", "0002_ok.sql"), "CREATE TABLE ok (id INTEGER);\n")
	writeFile(t, filepath.Join(root, "migrations", "0003_bad.sql"), "CREATE TABLE bad (id INTEGER,, x INTEGER);\n")
	if code, out, errs := maago("migrate"); code != 1 || out != "applied 0002_ok.sql\n" || !strings.HasPrefix(errs, "migrations/0003_bad.sql:1:") {
		t.Errorf("migrate with a bad file: %d %q %q", code, out, errs)
	}
	// Failing on the first pending file: no "applied", no "nothing to apply".
	if code, out, _ := maago("migrate"); code != 1 || out != "" {
		t.Errorf("migrate again with a bad file: %d %q", code, out)
	}
	// A changed applied file is a diagnostic for showmigrations too.
	writeFile(t, filepath.Join(root, "migrations", "0002_ok.sql"), "CREATE TABLE ok (id INTEGER, y INTEGER);\n")
	if code, out, errs := maago("showmigrations"); code != 1 || out != "" || !strings.HasPrefix(errs, "migrations/0002_ok.sql: migration was changed after it was applied") {
		t.Errorf("showmigrations with a changed file: %d %q %q", code, out, errs)
	}
}
