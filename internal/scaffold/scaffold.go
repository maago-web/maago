// Package scaffold holds the contrib apps that maago add copies into a
// project, and copies them (D-123). The copies are the project's own
// files, without the generated mark (D-67).
package scaffold

import (
	"embed"
	"errors"
	"fmt"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/project"
)

// files holds one directory per app. The go tool does not build testdata,
// so the .go files there wait for the project's generated code.
//
//go:embed testdata
var files embed.FS

// Apps are the apps Add knows.
func Apps() []string {
	entries, _ := files.ReadDir("testdata")
	var apps []string
	for _, e := range entries {
		apps = append(apps, e.Name())
	}
	return apps
}

// Add writes app into the project holding dir, at <root>/<app>/, and
// returns the files it wrote, relative to the root. If any of them is
// there already, it writes nothing (D-123).
func Add(dir, app string) ([]string, []diag.Diagnostic) {
	apps := Apps()
	if !slices.Contains(apps, app) {
		fix := "apps: " + strings.Join(apps, ", ")
		if s := diag.Suggest(app, apps); s != "" {
			fix = fmt.Sprintf("did you mean %q? %s", s, fix)
		}
		return nil, []diag.Diagnostic{diag.New(token.Position{}, fmt.Sprintf("unknown app %q", app), fix)}
	}
	root, ds := project.Root(dir)
	if len(ds) > 0 {
		return nil, ds
	}
	var names []string // slash paths relative to the root
	fs.WalkDir(files, "testdata/"+app, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			names = append(names, strings.TrimPrefix(p, "testdata/"))
		}
		return err
	})
	const fix = "remove it, or run maago add in an empty directory and compare the two with git diff --no-index"
	for _, n := range names {
		switch _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(n))); {
		case err == nil:
			ds = append(ds, diag.New(token.Position{Filename: n}, "something is in the way of maago add "+app, fix))
		case !errors.Is(err, fs.ErrNotExist):
			ds = append(ds, diag.New(token.Position{Filename: n}, "cannot look at the path: "+diag.OneLine(err.Error()), "make the directory readable, then run maago add "+app+" again"))
		}
	}
	if len(ds) > 0 {
		return nil, ds
	}
	var written []string
	for _, n := range names {
		if err := write(root, n); err != nil {
			undo(root, written, n)
			return nil, []diag.Diagnostic{diag.New(token.Position{Filename: n}, "cannot write the file: "+diag.OneLine(err.Error()), "make the directory writable, then run maago add "+app+" again")}
		}
		written = append(written, n)
	}
	return written, nil
}

// undo removes the files of a failed Add, then the directories they and
// the file that failed leave empty, innermost first; a directory that
// holds anything else stays. The failed file itself is write's to remove:
// whatever is there now may not be ours.
func undo(root string, written []string, failed string) {
	dirs := map[string]bool{}
	for _, w := range append(written, failed) {
		if w != failed {
			os.Remove(filepath.Join(root, filepath.FromSlash(w)))
		}
		for d := path.Dir(w); d != "."; d = path.Dir(d) {
			dirs[d] = true
		}
	}
	for _, d := range slices.Sorted(maps.Keys(dirs)) {
		defer os.Remove(filepath.Join(root, filepath.FromSlash(d))) // deferred: deepest goes first
	}
}

// write copies the embedded file n to root, failing if something is
// there by now, and leaves nothing behind when it fails.
func write(root, n string) error {
	src, err := files.ReadFile(path.Join("testdata", n))
	if err != nil {
		return err
	}
	p := filepath.Join(root, filepath.FromSlash(n))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(src)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(p) // ours: O_EXCL made it
	}
	return err
}
