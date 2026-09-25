package model_test

import (
	"bytes"
	"go/format"
	"maps"
	"slices"
	"testing"

	"github.com/maago-web/maago/internal/golden"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/model/modeltest"
	"github.com/maago-web/maago/internal/pipeline"
)

func TestFiles(t *testing.T) {
	apps, _ := modeltest.Example(t, "../testdata/example")
	// Synthetic apps: the root package, a package named unlike its
	// directory, and an app with nothing to report.
	apps = append(apps,
		&model.App{Dir: ".", Name: "main", Models: []*model.Model{{Name: "M", Unsupported: []model.Unsupported{{Name: "n", Reason: "unexported field"}}}}},
		&model.App{Dir: "x/y", Name: "z", Models: []*model.Model{
			{Name: "M", Unsupported: []model.Unsupported{{Name: "Count", Reason: "type int is not supported"}}},
			{Name: "N", Unsupported: []model.Unsupported{{Name: "cache", Reason: "unexported field"}}},
		}},
		&model.App{Dir: "clean", Name: "clean", Models: []*model.Model{{Name: "M"}}},
	)
	files := model.Files(apps)

	if got, want := slices.Sorted(maps.Keys(files)), []string{"blog/models_gen.go", "models_gen.go", "x/y/models_gen.go"}; !slices.Equal(got, want) {
		t.Fatalf("files %q, want %q", got, want)
	}
	for p, b := range files {
		formatted, err := format.Source(b)
		if err != nil || string(formatted) != string(b) {
			t.Errorf("%s is not gofmt-clean Go (%v):\n%s", p, err, b)
		}
		if !bytes.HasPrefix(b, []byte(pipeline.Marker)) {
			t.Errorf("%s does not start with the marker", p)
		}
	}
	if !bytes.Contains(files["models_gen.go"], []byte("\npackage main\n")) {
		t.Errorf("root file is not package main:\n%s", files["models_gen.go"])
	}
	golden.Check(t, "testdata/models_gen.golden", files["blog/models_gen.go"])
	golden.Check(t, "testdata/models_gen_z.golden", files["x/y/models_gen.go"])
}
