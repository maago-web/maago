package project

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
)

func TestOwnVersion(t *testing.T) {
	tests := []struct {
		name string
		bi   debug.BuildInfo
		want string
	}{
		{"go install", debug.BuildInfo{Main: debug.Module{Path: Module, Version: "v0.1.0"}}, "v0.1.0"},
		{"go tool", debug.BuildInfo{Main: debug.Module{Path: "example.com/app"}, Deps: []*debug.Module{{Path: "x.org/y", Version: "v1.0.0"}, {Path: Module, Version: "v0.1.0"}}}, "v0.1.0"},
		{"go tool, replaced", debug.BuildInfo{Main: debug.Module{Path: "example.com/app"}, Deps: []*debug.Module{{Path: Module, Version: "v0.1.0", Replace: &debug.Module{Path: "../maago"}}}}, ""},
		{"neither", debug.BuildInfo{Main: debug.Module{Path: "example.com/app"}}, ""},
	}
	for _, tt := range tests {
		if got := ownVersion(&tt.bi); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestVersion(t *testing.T) {
	t.Setenv("GOPROXY", "off") // go list -m reports the required version without fetching it
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	const differ = "go.mod: this maago is v0.2.0 but the project uses github.com/maago-web/maago v0.1.0\n" +
		"  fix: go install github.com/maago-web/maago/cmd/maago@v0.1.0, or go get github.com/maago-web/maago@v0.2.0\n"
	tests := []struct {
		name, own, mod, want string
	}{
		{"same", "v0.1.0", "require github.com/maago-web/maago v0.1.0\n", ""},
		{"differ", "v0.2.0", "require github.com/maago-web/maago v0.1.0\n", differ},
		{"dirty", "v0.2.0+dirty", "require github.com/maago-web/maago v0.1.0\n", ""},
		{"checkout", "v0.2.1-0.20261015000000-68c152df0257", "require github.com/maago-web/maago v0.1.0\n", ""},
		{"devel", "(devel)", "require github.com/maago-web/maago v0.1.0\n", ""},
		{"unknown", "", "require github.com/maago-web/maago v0.1.0\n", ""},
		{"pre-release", "v0.2.0-rc.1", "require github.com/maago-web/maago v0.1.0\n", ""},
		{"project pseudo", "v0.2.0", "require github.com/maago-web/maago v0.0.0-00010101000000-000000000000\n", ""},
		{"replaced", "v0.2.0", "require github.com/maago-web/maago v0.1.0\n\nreplace github.com/maago-web/maago => ../maago\n", ""},
		{"not required", "v0.2.0", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.25.0\n\n"+tt.mod), 0o644); err != nil {
				t.Fatal(err)
			}
			var b strings.Builder
			diag.Write(&b, Version(root, tt.own))
			if got := b.String(); got != tt.want {
				t.Errorf("got\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}
