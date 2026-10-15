package project

import (
	"encoding/json"
	"go/token"
	"os/exec"
	"regexp"
	"runtime/debug"

	"github.com/maago-web/maago/internal/diag"
)

// Module is maago's module path: the runtime's and this generator's (D-151).
const Module = "github.com/maago-web/maago"

// release is a tagged version: not a pseudo-version, a pre-release,
// +dirty or (devel).
var release = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

// OwnVersion is the version of maago this binary was built from: the main
// module's for go install, the dependency's for go tool. It is "" when the
// dependency is replaced.
func OwnVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return ownVersion(bi)
}

func ownVersion(bi *debug.BuildInfo) string {
	if bi.Main.Path == Module {
		return bi.Main.Version
	}
	for _, d := range bi.Deps {
		if d.Path == Module && d.Replace == nil {
			return d.Version
		}
	}
	return ""
}

// Version checks that the project at root uses the runtime of the same
// release as own, the generator's (D-151). It says nothing unless both are
// releases and the project does not replace the module: a checkout, and
// this repository's tests, generate for the runtime beside them.
func Version(root, own string) []diag.Diagnostic {
	if !release.MatchString(own) {
		return nil
	}
	cmd := exec.Command("go", "list", "-m", "-e", "-json", Module)
	cmd.Dir = root
	out, err := cmd.Output()
	var m struct {
		Version string
		Replace *struct{}
	}
	// A broken go command or go.mod is reported where it matters (D-117).
	if err != nil || json.Unmarshal(out, &m) != nil || m.Replace != nil || !release.MatchString(m.Version) || m.Version == own {
		return nil
	}
	return []diag.Diagnostic{diag.New(token.Position{Filename: "go.mod"},
		"this maago is "+own+" but the project uses "+Module+" "+m.Version,
		"go install "+Module+"/cmd/maago@"+m.Version+", or go get "+Module+"@"+own)}
}
