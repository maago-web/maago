package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/token"
	"io"
	"net/http"
	"os"
	osexec "os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/project"
)

// tailwindVersion is the standalone Tailwind CSS maago tailwind runs
// (D-138), and tailwindBuilds its file for each platform with the file's
// SHA-256, as the release's sha256sums.txt gave them: written here, so a
// changed file at the release is refused. Linux takes the glibc builds,
// and the musl ones where musl's loader is, as on Alpine: neither runs
// with the other's.
const tailwindVersion = "v4.3.3"

var tailwindBuilds = map[string]struct{ asset, sha256 string }{
	"darwin/arm64":     {"tailwindcss-macos-arm64", "cdf646702987a743464dff4d9c60fd4480d1c1e73dd819a9a67f1078815dce9d"},
	"darwin/amd64":     {"tailwindcss-macos-x64", "7922e0953f2110c05976e3bf58f14e643d90427575e766b7d433f5f80cbee7e1"},
	"linux/amd64":      {"tailwindcss-linux-x64", "dc61b3ac6b8c9ca874c0cc4c57b2409791a64c5540404ca5f5367360babc313a"},
	"linux/arm64":      {"tailwindcss-linux-arm64", "55fd0b241214eff3de1e8ee4f22796662f2d2e7a49bcfca7477cfd0bac398195"},
	"linux/amd64/musl": {"tailwindcss-linux-x64-musl", "a04d34ceacc8f52cbe8920ad846cdeb61d3d0021dba32db0d1f77c9d9fad7a6c"},
	"linux/arm64/musl": {"tailwindcss-linux-arm64-musl", "71ea4be79c9de9827545682df3e040053fb535d37c71ed2cfdedf9385a0868e0"},
	"windows/amd64":    {"tailwindcss-windows-x64.exe", "e0e260ce048014e9268f6237ff18f8ccf02cef521cbd0ae04e82c2cdf7aa3955"},
}

// errChanged is a download that is not the build maago knows.
var errChanged = errors.New("it is not the build maago knows")

// tailwindURL is where the builds are; tests serve their own.
var tailwindURL = "https://github.com/tailwindlabs/tailwindcss/releases/download/" + tailwindVersion + "/"

// cmdTailwind writes each app's static/tailwind.css from its
// tailwind.css, with the classes the project's files use (D-138).
func cmdTailwind(stdout, stderr io.Writer) int {
	root, ds := project.Root(".")
	var inputs []string
	if ds == nil {
		inputs, ds = project.TailwindInputs(root)
	}
	if ds == nil && len(inputs) == 0 {
		ds = []diag.Diagnostic{diag.New(token.Position{}, "no tailwind.css in any app",
			`write APP/tailwind.css with the line @import "tailwindcss"; and run maago tailwind again`)}
	}
	var bin string
	if ds == nil {
		bin, ds = tailwindBinary(tailwindPlatform())
	}
	if ds != nil {
		diag.Write(stderr, ds)
		return 1
	}
	for _, in := range inputs {
		out := path.Join(path.Dir(in), "static", "tailwind.css")
		if err := runTailwind(bin, root, in, out); err != nil {
			fmt.Fprintf(stderr, "maago tailwind: %s: %v\n", in, err)
			return 1
		}
		fmt.Fprintf(stdout, "wrote %s\n", out)
	}
	return 0
}

// runTailwind runs the standalone build in root: it finds the classes in
// the project's files from there. It writes a temporary file beside
// static/, never served, and renames it: a failed run leaves the old
// output, and no empty static/. Tailwind's words show only then.
func runTailwind(bin, root, in, out string) error {
	dir := filepath.Join(root, filepath.FromSlash(path.Dir(out)))
	f, err := os.CreateTemp(filepath.Dir(dir), ".tailwind-*.css")
	if err != nil {
		return err
	}
	f.Close()
	defer os.Remove(f.Name())
	cmd := osexec.Command(bin, "-i", filepath.FromSlash(in), "-o", f.Name(), "--minify")
	cmd.Dir = root
	var said bytes.Buffer
	cmd.Stdout, cmd.Stderr = &said, &said
	if err := cmd.Run(); err != nil {
		if said.Len() > 0 {
			return errors.New(strings.TrimSpace(said.String()))
		}
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, "tailwind.css"))
}

// tailwindPlatform is this machine's key in tailwindBuilds.
func tailwindPlatform() string {
	p := runtime.GOOS + "/" + runtime.GOARCH
	if musl, _ := filepath.Glob("/lib/ld-musl-*.so.1"); runtime.GOOS == "linux" && musl != nil {
		p += "/musl"
	}
	return p
}

// tailwindBinary is the path of the standalone build for platform, in the
// user's cache; it is downloaded and its SHA-256 checked the first time.
func tailwindBinary(platform string) (string, []diag.Diagnostic) {
	b, ok := tailwindBuilds[platform]
	if !ok {
		return "", []diag.Diagnostic{diag.New(token.Position{}, "tailwindcss has no build maago knows for "+platform,
			"run maago tailwind on macOS, Linux (amd64, arm64) or Windows (amd64), and commit APP/static/tailwind.css")}
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", []diag.Diagnostic{diag.New(token.Position{}, "no cache directory for tailwindcss: "+diag.OneLine(err.Error()),
			"set HOME, or XDG_CACHE_HOME on Linux")}
	}
	dir := filepath.Join(cache, "maago")
	bin := filepath.Join(dir, "tailwindcss-"+tailwindVersion+"-"+b.asset)
	if _, err := os.Stat(bin); err == nil {
		return bin, nil
	}
	if err := downloadTailwind(dir, bin, b.asset, b.sha256); err != nil {
		fix := "check the network (HTTPS_PROXY is honored), then run maago tailwind again"
		if errors.Is(err, errChanged) {
			fix = "do not use that file; run maago tailwind again, and if it names a SHA-256 again, the release no longer holds the build maago knows"
		}
		return "", []diag.Diagnostic{diag.New(token.Position{}, "cannot get tailwindcss "+tailwindVersion+": "+diag.OneLine(err.Error()), fix)}
	}
	return bin, nil
}

// downloadTailwind writes the build to a temporary file in dir, checks its
// SHA-256, and renames it to bin: a partial or changed file never takes
// bin's name. Two runs at once both rename the same checked bytes.
func downloadTailwind(dir, bin, asset, want string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(tailwindURL + asset)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", tailwindURL+asset, resp.Status)
	}
	f, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name()) // gone once renamed
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("%s has SHA-256 %s, not %s: %w", asset, got, want, errChanged)
	}
	if err := os.Chmod(f.Name(), 0o755); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), bin); err != nil {
		if _, serr := os.Stat(bin); serr == nil {
			return nil // another run put it there first (Windows renames onto no file)
		}
		return err
	}
	return nil
}
