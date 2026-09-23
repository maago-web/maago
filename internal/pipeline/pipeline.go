// Package pipeline runs the stages of maago gen and writes what they
// generate (D-60, D-61). A stage is a list of steps; checks and generators
// are both steps, and a generator is a step that fills Run.Files.
package pipeline

import (
	"bytes"
	"errors"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/maago-web/maago/internal/diag"
)

// Run is what the steps share.
type Run struct {
	Root  string            // project root
	Files map[string][]byte // generated files, keyed by slash path relative to Root
}

// Step checks or generates part of the project.
type Step func(*Run) []diag.Diagnostic

// Gen runs the stages and writes the generated files only if no step
// reported an error.
func Gen(root string, stages [][]Step) []diag.Diagnostic {
	r := &Run{Root: root, Files: map[string][]byte{}}
	if ds := Generate(r, stages); len(ds) > 0 {
		return ds
	}
	return Write(root, r.Files)
}

// Generate runs the stages in order. Every step of a stage runs, so all of
// its independent errors are reported together; a stage with errors stops
// the run, so later stages cannot add errors that follow from them.
func Generate(r *Run, stages [][]Step) []diag.Diagnostic {
	for _, stage := range stages {
		var ds []diag.Diagnostic
		for _, step := range stage {
			ds = append(ds, step(r)...)
		}
		if len(ds) > 0 {
			return ds
		}
	}
	return nil
}

// Write writes files under root. Files whose content is already on disk
// are skipped. Every other file is first written to a temporary file next
// to it; only when all of those succeed are they renamed into place. A
// failed rename can leave some files updated, and running again writes the
// rest.
func Write(root string, files map[string][]byte) []diag.Diagnostic {
	paths := slices.Sorted(maps.Keys(files))
	var ds []diag.Diagnostic
	for _, p := range paths {
		if !filepath.IsLocal(filepath.FromSlash(p)) || strings.ContainsAny(p, "\r\n") {
			ds = append(ds, diag.New(token.Position{Filename: p},
				"generated file path is outside the project", "this is a bug in maago; please report it"))
		}
	}
	if len(ds) > 0 {
		return ds
	}

	type rename struct{ tmp, dst, path string }
	var renames []rename
	removeTemps := func(rs []rename) {
		for _, r := range rs {
			os.Remove(r.tmp)
		}
	}
	for _, p := range paths {
		dst := filepath.Join(root, filepath.FromSlash(p))
		old, err := os.ReadFile(dst)
		if err == nil && bytes.Equal(old, files[p]) {
			continue
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			removeTemps(renames)
			return []diag.Diagnostic{diag.New(token.Position{Filename: p},
				"cannot read generated file: "+err.Error(), "make sure the file can be read, then rerun maago gen")}
		}
		tmp, err := writeTemp(dst, files[p])
		if err != nil {
			removeTemps(renames)
			return []diag.Diagnostic{diag.New(token.Position{Filename: p},
				"cannot write generated file: "+err.Error(), "make sure the directory can be written, then rerun maago gen")}
		}
		renames = append(renames, rename{tmp, dst, p})
	}
	for i, r := range renames {
		if err := os.Rename(r.tmp, r.dst); err != nil {
			removeTemps(renames[i:])
			return []diag.Diagnostic{diag.New(token.Position{Filename: r.path},
				"cannot replace generated file: "+err.Error(), "rerun maago gen; files already written are skipped")}
		}
	}
	return nil
}

// writeTemp writes b to a new temporary file in dst's directory, creating
// the directory if needed, and returns the temporary file's name.
func writeTemp(dst string, b []byte) (string, error) {
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".maago-*.tmp")
	if err != nil {
		return "", err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Chmod(0o644)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
