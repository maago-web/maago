// Package modeltest loads checked models for tests.
package modeltest

import (
	"go/token"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/project"
)

// Example reads and checks the project at root, failing t on any error. It
// is for tests, here and in the dialects, that need checked models.
func Example(t testing.TB, root string) ([]*model.App, *token.FileSet) {
	t.Helper()
	files, ds := project.GoFiles(root)
	fset := token.NewFileSet()
	var pkgs []*model.Package
	if len(ds) == 0 {
		pkgs, ds = model.Read(fset, root, files)
	}
	var apps []*model.App
	if len(ds) == 0 {
		apps, ds = model.Check(fset, pkgs)
	}
	if len(ds) > 0 {
		var b strings.Builder
		diag.Write(&b, ds)
		t.Fatalf("example %s:\n%s", root, b.String())
	}
	return apps, fset
}
