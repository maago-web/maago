// Package build assembles the stages of maago gen and maago check. It is
// the one place that knows every stage, so the CLI only dispatches (D-68)
// and pipeline stays unaware of models.
package build

import (
	"go/token"
	"maps"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/pipeline"
	"github.com/maago-web/maago/internal/project"
)

// Gen checks the project holding dir and, if nothing is wrong, writes its
// generated files. It returns the generated files it removed.
func Gen(dir string) (removed []string, ds []diag.Diagnostic) {
	root, ds := project.Root(dir)
	if len(ds) > 0 {
		return nil, ds
	}
	return pipeline.Gen(root, stages(root))
}

// Check checks the project holding dir and that its generated files are up
// to date. It writes nothing.
func Check(dir string) []diag.Diagnostic {
	root, ds := project.Root(dir)
	if len(ds) > 0 {
		return ds
	}
	r := &pipeline.Run{Root: root, Files: map[string][]byte{}}
	if ds := pipeline.Generate(r, stages(root)); len(ds) > 0 {
		return ds
	}
	return pipeline.Check(root, r.Files)
}

// stages returns fresh stages for one run. What a stage hands to the next
// lives in these variables, so each run starts clean. A stage runs only if
// the ones before it found nothing wrong, so none of them sees
// half-finished work (D-61).
func stages(root string) [][]pipeline.Step {
	fset := token.NewFileSet()
	var files []string
	var pkgs []*model.Package
	var apps []*model.App
	step := func(f func() []diag.Diagnostic) []pipeline.Step {
		return []pipeline.Step{func(*pipeline.Run) []diag.Diagnostic { return f() }}
	}
	return [][]pipeline.Step{
		step(func() (ds []diag.Diagnostic) { files, ds = project.GoFiles(root); return }),
		step(func() (ds []diag.Diagnostic) { pkgs, ds = model.Read(fset, root, files); return }),
		step(func() (ds []diag.Diagnostic) { apps, ds = model.Check(fset, pkgs); return }),
		// Name clashes fail in every database, so they are checked before
		// the dialect is known. PostgreSQL's length limit waits for M2.
		step(func() []diag.Diagnostic { return dialect.CheckNames(fset, apps, 0) }),
		{func(r *pipeline.Run) []diag.Diagnostic { maps.Copy(r.Files, model.Files(apps)); return nil }},
	}
}
