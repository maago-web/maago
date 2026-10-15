package main

import (
	"fmt"
	"go/format"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The short tutorial (docs/TUTORIAL-FUTURE.md §2–5) followed as written,
// in an empty directory: its code blocks are the files, its output blocks
// what maago says, and it ends in the admin, signed in, writing, changing
// and deleting a post (D-69, D-143). SQLite, as the tutorial starts.
func TestShortTutorial(t *testing.T) {
	noEnv(t)
	doc := newTutorial(t, "../../docs/TUTORIAL-FUTURE.md", "## 2.", "## 6.")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	// goRun runs the go command in root, offline: cmd/maago's tests
	// fetched the same versions.
	goRun := func(args ...string) {
		t.Helper()
		cmd := osexec.Command("go", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	// maago runs a command of the tutorial and wants code and the output
	// the tutorial shows.
	maago := func(code int, want string, args ...string) {
		t.Helper()
		got, stdout, stderr := runCmd(args...)
		if got != code || stdout+stderr != want {
			t.Fatalf("maago %s: %d, want %d\n%s%s\nwant\n%s", strings.Join(args, " "), got, code, stdout, stderr, want)
		}
	}

	// §2, the commands as the tutorial gives them, but for two: until
	// the release this repository takes the place of go get
	// github.com/maago-web/maago (D-146; task 11 runs the real one), and
	// the driver is cmd/maago's version, offline.
	doc.commands("mkdir myblog && cd myblog\ngo mod init example.com/myblog\ngo get github.com/maago-web/maago\ngo get modernc.org/sqlite\n")
	goRun("mod", "init", "example.com/myblog")
	goRun("mod", "edit", "-require=github.com/maago-web/maago@v0.0.0-00010101000000-000000000000", "-replace=github.com/maago-web/maago="+repo)
	goRun("get", "modernc.org/sqlite@v1.59.0")
	env := doc.block("sh", "# .env")
	writeFile(t, filepath.Join(root, ".env"), strings.TrimPrefix(env, "# .env\n"))
	t.Chdir(root)

	// §3: auth, the model, the error on purpose, and what gen says next.
	doc.commands("maago add auth\n")
	if code, _, stderr := runCmd("add", "auth"); code != 0 {
		t.Fatalf("maago add auth: %d %s", code, stderr)
	}
	models := doc.goFile("// blog/models.go")
	typo := doc.block("go", "\tTitle ")
	bad := regexp.MustCompile("(?m)^\tTitle .*\n").ReplaceAllString(models, typo)
	if bad == models {
		t.Fatal("§3's typo is not a line of the model")
	}
	writeFile(t, filepath.Join(root, "blog", "models.go"), bad)
	doc.commands("maago gen\n")
	maago(1, doc.block("", "blog/models.go:"), "gen")
	writeFile(t, filepath.Join(root, "blog", "models.go"), models)
	maago(1, doc.block("", "auth/urls.go:"), "gen")
	if gens, _ := filepath.Glob(filepath.Join(root, "*", "*_gen.go")); len(gens) > 0 {
		t.Errorf("gen failed and wrote %v; §3 says it writes nothing", gens)
	}

	// §4: the draft, and the table the tutorial shows in it.
	doc.commands("maago makemigrations\n")
	maago(0, doc.block("", "created migrations/"), "makemigrations")
	// What the tutorial shows of the draft, its comments aside, is in it.
	draft := readFile(t, filepath.Join(root, "migrations", "0001_initial.sql"))
	shown := regexp.MustCompile("(?m)^--.*\n").ReplaceAllString(doc.block("sql", "-- migrations/0001_initial.sql"), "")
	for _, stmt := range strings.Split(strings.TrimSpace(shown), "\n\n") {
		if !strings.Contains(draft, stmt) {
			t.Errorf("the draft has no\n%s\nin\n%s", stmt, draft)
		}
	}
	doc.commands("maago showmigrations\n")
	maago(0, doc.block("", "[ ] 0001_initial.sql"), "showmigrations")
	doc.commands("maago migrate\n")
	maago(0, doc.block("", "applied 0001_initial.sql"), "migrate")
	doc.commands("maago makemigrations --rename posts.text:body\n") // a rename: not this flow

	// §5: main, gen, the first staff account, and the admin.
	writeFile(t, filepath.Join(root, "main.go"), doc.goFile("// main.go"))
	// go run . builds main as go build does; the binary serves on a free
	// PORT, which main reads.
	doc.commands("maago gen\nMAAGO_SUPERUSER_EMAIL=me@example.com MAAGO_SUPERUSER_PASSWORD='…' go run . -createsuperuser\ngo run .\n")
	maago(0, "", "gen")
	bin := filepath.Join(t.TempDir(), "myblog")
	goRun("build", "-o", bin, ".")
	const email, password = "me@example.com", "correct horse battery staple"
	su := osexec.Command(bin, "-createsuperuser")
	su.Env = append(os.Environ(), "MAAGO_SUPERUSER_EMAIL="+email, "MAAGO_SUPERUSER_PASSWORD="+password)
	if out, err := su.CombinedOutput(); err != nil {
		t.Fatalf("-createsuperuser: %v\n%s", err, out)
	}
	if err := serve(root, bin, "/admin/", http.StatusSeeOther, func(base string) error {
		return useAdmin(base, email, password)
	}); err != nil {
		t.Fatal(err)
	}
	doc.allUsed()
}

// useAdmin signs in at base and writes, changes and deletes a post in the
// admin, as the end of §5 says it can.
func useAdmin(base, email, password string) error {
	jar, _ := cookiejar.New(nil)
	client := http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	post := func(path string, form url.Values) error {
		resp, err := client.PostForm(base+path, form)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			return fmt.Errorf("POST %s: %d", path, resp.StatusCode)
		}
		return nil
	}
	// list wants the admin's list of posts to have, or not, title.
	list := func(title string, has bool) error {
		resp, err := client.Get(base + "/admin/blog/Post/")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK || strings.Contains(string(b), title) != has {
			return fmt.Errorf("the list of posts (%d) has %q: %v, want %v", resp.StatusCode, title, !has, has)
		}
		return nil
	}
	post1 := url.Values{"author_id": {"1"}, "title": {"첫 글"}, "text": {"본문"}, "status": {"draft"}, "published_at": {""}}
	for _, step := range []func() error{
		func() error { return post("/login", url.Values{"email": {email}, "password": {password}}) },
		func() error { return post("/admin/blog/Post/add/", post1) },
		func() error { return list("첫 글", true) },
		func() error {
			post1.Set("title", "고친 글")
			post1.Set("status", "published")
			post1.Set("published_at", "2026-10-15T09:00")
			return post("/admin/blog/Post/1/", post1)
		},
		func() error { return list("고친 글", true) },
		func() error { return post("/admin/blog/Post/delete/", url.Values{"id": {"1"}}) },
		func() error { return list("고친 글", false) },
	} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// tutorial is the fenced blocks of a part of a document, and which of
// them a test has used: a block it never reads is one it does not follow.
type tutorial struct {
	t      *testing.T
	blocks [][2]string // language, text
	used   []bool
}

// newTutorial reads the fenced blocks of path from the line from to the
// line to.
func newTutorial(t *testing.T, path, from, to string) *tutorial {
	t.Helper()
	src := readFile(t, path)
	start, end := strings.Index(src, "\n"+from), strings.Index(src, "\n"+to)
	if start < 0 || end < start {
		t.Fatalf("%s has no %q before %q", path, from, to)
	}
	tu := &tutorial{t: t}
	for _, m := range regexp.MustCompile("(?s)```(\\w*)\n(.*?)```").FindAllStringSubmatch(src[start:end], -1) {
		tu.blocks = append(tu.blocks, [2]string{m[1], m[2]})
	}
	tu.used = make([]bool, len(tu.blocks))
	return tu
}

// block is the first block in language lang (empty for none) whose text
// starts with prefix.
func (tu *tutorial) block(lang, prefix string) string {
	tu.t.Helper()
	for i, b := range tu.blocks {
		if b[0] == lang && strings.HasPrefix(b[1], prefix) {
			tu.used[i] = true
			return b[1]
		}
	}
	tu.t.Fatalf("the tutorial has no ```%s block starting %q", lang, prefix)
	return ""
}

// commands wants the tutorial's sh block of those commands, as they are:
// the test runs them, and a change to them is a change to the test.
func (tu *tutorial) commands(want string) {
	tu.t.Helper()
	for i, b := range tu.blocks {
		if b[0] == "sh" && b[1] == want {
			tu.used[i] = true
			return
		}
	}
	tu.t.Fatalf("the tutorial has no ```sh block\n%s", want)
}

// goFile is a Go file the tutorial shows, as gofmt has it.
func (tu *tutorial) goFile(first string) string {
	tu.t.Helper()
	src := tu.block("go", first)
	if out, err := format.Source([]byte(src)); err != nil || string(out) != src {
		tu.t.Fatalf("the tutorial's %s is not gofmt's: %v\n%s", first, err, out)
	}
	return src
}

// allUsed fails for a block the test never read.
func (tu *tutorial) allUsed() {
	tu.t.Helper()
	for i, b := range tu.blocks {
		if !tu.used[i] {
			tu.t.Errorf("the test does not follow the tutorial's ```%s block\n%s", b[0], b[1])
		}
	}
}
