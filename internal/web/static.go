package web

import (
	"errors"
	"go/ast"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/rtcore"
)

// ReadStatic reads the static/ of each app with routes into its Static
// (D-136), and finds static/ where nothing would serve it: beside a
// package without routes. parsed are the project's packages, by directory.
// What //go:embed static would leave out, or fail on, is an error here, so
// what maago gen lets through is what the process serves.
func ReadStatic(root string, parsed map[string][]*ast.File, apps []*App) []diag.Diagnostic {
	var ds []diag.Diagnostic
	byDir := map[string]*App{}
	for _, app := range apps {
		byDir[app.Dir] = app
	}
	byName := map[string]*App{} // the apps with static files, by package name
	for _, dir := range slices.Sorted(maps.Keys(parsed)) {
		sdir := path.Join(dir, "static")
		entries, err := fs.ReadDir(os.DirFS(filepath.Join(root, filepath.FromSlash(dir))), "static")
		if errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			ds = append(ds, diag.New(token.Position{Filename: sdir}, "cannot read static/: "+err.Error(), "make the directory readable"))
			continue
		}
		app := byDir[dir]
		if app == nil {
			ds = append(ds, diag.New(token.Position{Filename: sdir}, "static/ of a package without routes",
				"keep static files in an app with a route table, which serves them at /static/APP/"))
			continue
		}
		for _, e := range entries {
			at := token.Position{Filename: path.Join(sdir, e.Name())}
			switch {
			case e.IsDir():
				ds = append(ds, diag.New(at, "directory in static/", "keep the static files in static/ itself"))
			case !e.Type().IsRegular():
				ds = append(ds, diag.New(at, "not a regular file", "put the file itself there; //go:embed leaves out links"))
			default:
				if err := rtcore.StaticName(e.Name()); err != nil {
					ds = append(ds, diag.New(at, err.Error(), "rename it, such as app.css"))
				} else {
					app.Static, app.used = append(app.Static, e.Name()), map[string]bool{}
				}
			}
		}
		switch other := byName[app.Name]; {
		case len(app.Static) == 0:
			ds = append(ds, diag.New(token.Position{Filename: sdir}, "static/ holds no file to serve", "add a file, or remove the directory: //go:embed needs one"))
		case other != nil:
			ds = append(ds, diag.New(token.Position{Filename: sdir}, "apps "+other.Dir+" and "+app.Dir+" are both package "+app.Name+", so both would serve /static/"+app.Name+"/",
				"rename one package, or keep the static files in one of them"))
		default:
			byName[app.Name] = app
		}
	}
	return ds
}
