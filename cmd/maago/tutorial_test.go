package main

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/build"
)

const steps = "../../examples/board/steps"

// The board tutorial walks from its first step to its last as each step
// expects (D-103).
func TestBoardTutorial(t *testing.T) {
	if err := walk(t, steps); err != nil {
		t.Fatal(err)
	}
}

// The walk stops at a step that goes wrong, whichever part of it does:
// the queries, the expected error, or the app's tests.
func TestBoardTutorialCatches(t *testing.T) {
	for _, c := range []struct{ name, patch, old, new, by string }{
		{"query", "0005-Add-post-queries.patch", "p.body, p.status", "p.nope, p.status", "maago gen:\n"},
		{"expected error", "0006-Try-a-model-result-that-misses-a-column.patch", "title, status", "title, body, status", "maago gen: expected\n"},
		{"app test", "0005-Add-post-queries.patch", `{0, "[글 5 글 4]"}`, `{0, "[글 4 글 5]"}`, "go test: "},
		// The expected error is the whole first line, not a prefix of it.
		{"short expectation", "0006-Try-a-model-result-that-misses-a-column.patch",
			`Maago-Expect: board/queries/recent_posts.sql:4:1: result columns do not match model Post: missing "body"`,
			`Maago-Expect: board/queries/recent_posts.sql:4:1: result columns do not match model Post`, "maago gen: expected\n"},
		// Both template errors at once (D-61): expecting one is not enough.
		{"one template error of two", "0009-Try-two-typos-in-the-templates.patch",
			"\nMaago-Expect: board/templates/post_page.html:3:13: PostPage.Post has no field or method \"Titel\"", "", "maago gen: expected\n"},
		{"gofmt", "0002-Add-Board-Post-and-Comment.patch", "+\tdb := maagotest.DB(t)", "+    db := maagotest.DB(t)", "gofmt -l: "},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.CopyFS(dir, os.DirFS(steps)); err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(dir, c.patch)
			src := readFile(t, p)
			if !strings.Contains(src, c.old) {
				t.Fatalf("%s has no %q", c.patch, c.old)
			}
			writeFile(t, p, strings.Replace(src, c.old, c.new, 1))
			if err := walk(t, dir); err == nil || !strings.HasPrefix(err.Error(), c.patch+": "+c.by) {
				t.Errorf("walk: %v", err)
			}
		})
	}
}

// walk applies the patches in dir one by one to a new project, as a reader
// of the tutorial would make them, and checks each step. A step whose
// commit message has Maago-Expect: lines must fail maago gen with exactly
// those diagnostics; any other step must pass gen, check and migrate, and
// the app's tests. The steps share the database maago migrate makes, as a
// reader's would; the app's tests get their own from maagotest (D-113).
func walk(t *testing.T, dir string) error {
	t.Helper()
	noEnv(t) // maago reads .env, as for the reader
	patches, _ := filepath.Glob(filepath.Join(dir, "*.patch"))
	if len(patches) == 0 {
		return fmt.Errorf("no patches in %s", dir)
	}
	// The go command compares go.work's directory with the real path.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		return err
	}
	repo, err := filepath.EvalSymlinks("../..")
	if err != nil {
		return err
	}
	if repo, err = filepath.Abs(repo); err != nil {
		return err
	}
	// A repository of its own, so git apply works on root and nowhere else.
	if out, err := osexec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		return fmt.Errorf("git init: %v\n%s", err, out)
	}
	writeFile(t, filepath.Join(root, "go.work"), "go 1.25.0\n\nuse .\n\nreplace github.com/maago-web/maago => "+repo+"\n")
	writeFile(t, filepath.Join(root, ".env"), "MAAGO_DATABASE_URL=sqlite:board.db\n")
	for _, p := range patches {
		if err := walkStep(root, p); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
	}
	return nil
}

func walkStep(root, patch string) error {
	src, err := os.ReadFile(patch)
	if err != nil {
		return err
	}
	want, err := expected(src)
	if err != nil {
		return err
	}
	abs, _ := filepath.Abs(patch)
	apply := osexec.Command("git", "apply", abs)
	apply.Dir = root
	if out, err := apply.CombinedOutput(); err != nil {
		return fmt.Errorf("git apply: %v\n%s", err, out)
	}

	_, ds := build.Gen(root)
	got := diagnostics(text(ds))
	if want != nil {
		if !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))) {
			return fmt.Errorf("maago gen: expected\n%s\ngot\n%s", strings.Join(want, "\n"), text(ds))
		}
		return nil
	}
	if ds != nil {
		return fmt.Errorf("maago gen:\n%s", text(ds))
	}
	if out, err := osexec.Command("gofmt", "-l", root).CombinedOutput(); err != nil || len(out) > 0 {
		return fmt.Errorf("gofmt -l: %v\n%s", err, out)
	}
	if ds := build.Check(root); ds != nil {
		return fmt.Errorf("maago check:\n%s", text(ds))
	}
	if _, ds := build.Migrate(root); ds != nil {
		return fmt.Errorf("maago migrate:\n%s", text(ds))
	}
	// The app's tests find the root's .env from their package's directory,
	// as a reader's do (D-113): no address is given them.
	test := osexec.Command("go", "test", "./...")
	test.Dir = root
	test.Env = append(os.Environ(),
		// These win over the environment's own: exec.Cmd uses the last
		// value of a key.
		"GOWORK="+filepath.Join(root, "go.work"),
		"GOPROXY=off",           // the module cache only: cmd/maago's tests fetched the same versions
		"GOFLAGS=-mod=readonly", // a stale go.sum fails, whatever GOFLAGS says
	)
	if out, err := test.CombinedOutput(); err != nil {
		return fmt.Errorf("go test: %v\n%s", err, out)
	}
	return nil
}

// expected reads the Maago-Expect: lines of a patch's commit message,
// which ends where the diff starts (steps.sh packs without a diffstat).
// The mail header before it has no Maago- lines.
func expected(patch []byte) ([]string, error) {
	msg, _, ok := strings.Cut(string(patch), "\ndiff --git ")
	if !ok {
		return nil, fmt.Errorf("no diff after the commit message")
	}
	var want []string
	for _, line := range strings.Split(msg, "\n") {
		switch {
		case strings.HasPrefix(line, "Maago-Expect: "):
			want = append(want, strings.TrimPrefix(line, "Maago-Expect: "))
		case strings.HasPrefix(line, "Maago-"):
			return nil, fmt.Errorf("unknown trailer %q; the one maago knows is Maago-Expect:", line)
		}
	}
	return want, nil
}
