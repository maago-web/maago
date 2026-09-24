// Package project finds the user's project: its root and its Go files.
package project

import (
	"errors"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/maago-web/maago/internal/diag"
)

// Root returns the directory holding go.mod, searching from dir upward.
// Symbolic links are resolved first.
func Root(dir string) (string, []diag.Diagnostic) {
	start, err := filepath.Abs(dir)
	if err == nil {
		start, err = filepath.EvalSymlinks(start)
	}
	if err != nil {
		return "", []diag.Diagnostic{diag.New(token.Position{}, "cannot read directory: "+diag.OneLine(err.Error()), "run maago inside a Go module")}
	}
	for d := start; ; {
		_, err := os.Stat(filepath.Join(d, "go.mod"))
		if err == nil {
			return d, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", []diag.Diagnostic{diag.New(token.Position{}, "cannot read go.mod: "+diag.OneLine(err.Error()), "make sure go.mod can be read")}
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", []diag.Diagnostic{diag.New(token.Position{},
				"no go.mod in "+diag.OneLine(start)+" or any parent directory",
				"run maago inside a Go module; create one with go mod init")}
		}
		d = parent
	}
}

// GoFiles returns the Go files under root, tests included, as sorted slash
// paths relative to root. It looks where go's ./... pattern looks: it
// skips directories and files starting with "." or "_", testdata, vendor,
// and directories holding another module. It follows root if root is a
// symbolic link, but no link inside it.
func GoFiles(root string) ([]string, []diag.Diagnostic) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, []diag.Diagnostic{diag.New(token.Position{}, "cannot read project: "+diag.OneLine(err.Error()), "run maago in the project directory")}
	}
	var files []string
	var ds []diag.Diagnostic
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		rel, _ := filepath.Rel(root, path)
		p := filepath.ToSlash(rel)
		if err != nil {
			ds = append(ds, diag.New(token.Position{Filename: p}, "cannot read project: "+diag.OneLine(err.Error()), "make sure the directory can be read"))
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path == root {
				return nil
			}
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor" {
				return filepath.SkipDir
			}
			switch _, err := os.Stat(filepath.Join(path, "go.mod")); {
			case err == nil:
				return filepath.SkipDir // another module
			case !errors.Is(err, fs.ErrNotExist):
				// Unsure whether it is another module: stay out, since gen removes files.
				ds = append(ds, diag.New(token.Position{Filename: p}, "cannot read directory: "+diag.OneLine(err.Error()), "make sure the directory can be read"))
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() && strings.HasSuffix(name, ".go") && !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "_") {
			files = append(files, p)
		}
		return nil
	})
	slices.Sort(files) // WalkDir lists "blog/" before "blog.go"
	return files, ds
}
