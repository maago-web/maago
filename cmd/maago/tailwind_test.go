package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeTailwind serves a stand-in for the standalone build, a script that
// writes "/* fake */" to its output, under the asset of this platform,
// whose SHA-256 the table then holds (or wrong, when sum is given). It
// points the user's cache at a new directory and returns it.
func fakeTailwind(t *testing.T, sum string) (cache string, downloads *atomic.Int32) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	script := []byte("#!/bin/sh\nwhile [ \"$1\" != -o ]; do shift; done\necho '/* fake */' > \"$2\"\n")
	h := sha256.Sum256(script)
	if sum == "" {
		sum = hex.EncodeToString(h[:])
	}
	platform := tailwindPlatform()
	old, oldURL := tailwindBuilds[platform], tailwindURL
	asset := "tailwindcss-test"
	tailwindBuilds[platform] = struct{ asset, sha256 string }{asset, sum}
	downloads = &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + asset: // as GitHub does, to where the file is
			http.Redirect(w, r, "/files/"+asset, http.StatusFound)
		case "/files/" + asset:
			downloads.Add(1)
			w.Write(script)
		default:
			http.NotFound(w, r)
		}
	}))
	tailwindURL = srv.URL + "/"
	t.Cleanup(func() { tailwindBuilds[platform], tailwindURL = old, oldURL; srv.Close() })
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(cache, "maago"), downloads
}

// tailwindProject is the web fixture with an app's Tailwind input.
func tailwindProject(t *testing.T) string {
	t.Helper()
	root := webProject(t)
	writeFile(t, filepath.Join(root, "site", "tailwind.css"), "@import \"tailwindcss\";\n")
	t.Chdir(root)
	return root
}

func runCmd(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = run(args, &out, &errb)
	return code, out.String(), errb.String()
}

// The build is downloaded once, through a redirect, its SHA-256 checked
// before it takes its name; each app's tailwind.css gives its
// static/tailwind.css (D-138).
func TestTailwindDownload(t *testing.T) {
	cache, downloads := fakeTailwind(t, "")
	root := tailwindProject(t)
	os.RemoveAll(filepath.Join(root, "site", "static")) // made for the output
	for i := range 2 {
		code, stdout, stderr := runCmd("tailwind")
		if code != 0 || stdout != "wrote site/static/tailwind.css\n" || stderr != "" {
			t.Fatalf("run %d: %d %q %q", i, code, stdout, stderr)
		}
	}
	if n := downloads.Load(); n != 1 {
		t.Errorf("downloaded %d times", n)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "site", "static", "tailwind.css")); string(b) != "/* fake */\n" {
		t.Errorf("output %q", b)
	}
	entries, _ := os.ReadDir(cache)
	if len(entries) != 1 || entries[0].Name() != "tailwindcss-"+tailwindVersion+"-tailwindcss-test" {
		t.Errorf("cache: %v", entries)
	}
	if left, _ := filepath.Glob(filepath.Join(root, "site", ".tailwind-*")); left != nil {
		t.Errorf("the temporary output is left: %v", left)
	}
}

func TestTailwindRefuses(t *testing.T) {
	cache, _ := fakeTailwind(t, strings.Repeat("0", 64))
	plain := webProject(t) // no tailwind.css: copied before the Chdir below
	tailwindProject(t)
	code, _, stderr := runCmd("tailwind")
	if code != 1 || !strings.Contains(stderr, "cannot get tailwindcss "+tailwindVersion+": tailwindcss-test has SHA-256 ") ||
		!strings.Contains(stderr, "it is not the build maago knows") || !strings.Contains(stderr, "fix: do not use that file") {
		t.Errorf("a changed build: %d %q", code, stderr)
	}
	if entries, _ := os.ReadDir(cache); len(entries) != 0 {
		t.Errorf("the changed build is left: %v", entries)
	}

	if _, ds := tailwindBinary("plan9/386"); ds == nil || !strings.Contains(text(ds), "tailwindcss has no build maago knows for plan9/386") {
		t.Errorf("an unknown platform: %s", text(ds))
	}
	t.Chdir(plain)
	if code, _, stderr := runCmd("tailwind"); code != 1 || !strings.Contains(stderr, "no tailwind.css in any app") {
		t.Errorf("no input: %d %q", code, stderr)
	}
}

// A failed Tailwind run says why, and leaves the output it had.
func TestTailwindFails(t *testing.T) {
	fakeTailwind(t, "")
	root := tailwindProject(t)
	runCmd("tailwind")
	platform := tailwindPlatform()
	cache, _ := os.UserCacheDir()
	bin := filepath.Join(cache, "maago", "tailwindcss-"+tailwindVersion+"-"+tailwindBuilds[platform].asset)
	// It writes half an output, then fails.
	os.WriteFile(bin, []byte("#!/bin/sh\nwhile [ \"$1\" != -o ]; do shift; done\necho 'half' > \"$2\"\necho 'CssSyntaxError: bad' >&2\nexit 1\n"), 0o755)
	code, stdout, stderr := runCmd("tailwind")
	if code != 1 || stdout != "" || stderr != "maago tailwind: site/tailwind.css: CssSyntaxError: bad\n" {
		t.Errorf("%d %q %q", code, stdout, stderr)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "site", "static", "tailwind.css")); string(b) != "/* fake */\n" {
		t.Errorf("the output it had: %q", b)
	}
	if left, _ := filepath.Glob(filepath.Join(root, "site", ".tailwind-*")); left != nil {
		t.Errorf("the temporary output is left: %v", left)
	}
	// A first run that fails makes no static/, which maago gen would refuse empty.
	os.RemoveAll(filepath.Join(root, "site", "static"))
	runCmd("tailwind")
	if _, err := os.Stat(filepath.Join(root, "site", "static")); !os.IsNotExist(err) {
		t.Errorf("static/ after a failed first run: %v", err)
	}
}

// The real standalone build, without Node: the CSS holds the classes the
// templates use and not those they do not (M7's condition 4). It runs
// when the build is in the user's cache, and always in CI (D-91).
func TestTailwind(t *testing.T) {
	platform := tailwindPlatform()
	b, ok := tailwindBuilds[platform]
	if !ok {
		t.Skip("no build for " + platform)
	}
	cache, _ := os.UserCacheDir()
	if _, err := os.Stat(filepath.Join(cache, "maago", "tailwindcss-"+tailwindVersion+"-"+b.asset)); err != nil && os.Getenv("CI") == "" {
		t.Skip("tailwindcss " + tailwindVersion + " is not in the cache; run maago tailwind once, or set CI")
	}
	root := tailwindProject(t)
	base := filepath.Join(root, "site", "templates", "base.html")
	src, _ := os.ReadFile(base)
	writeFile(t, base, strings.Replace(string(src), "<body>", `<body class="mx-auto text-red-700">`, 1))
	t.Setenv("PATH", t.TempDir())        // no node, nor anything else
	t.Chdir(filepath.Join(root, "site")) // from inside an app, as an agent may
	code, stdout, stderr := runCmd("tailwind")
	if code != 0 || stdout != "wrote site/static/tailwind.css\n" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	css, _ := os.ReadFile(filepath.Join(root, "site", "static", "tailwind.css"))
	if !bytes.Contains(css, []byte(".mx-auto{")) || !bytes.Contains(css, []byte(".text-red-700{")) || bytes.Contains(css, []byte(".text-blue-700{")) {
		t.Errorf("the CSS:\n%.2000s", css)
	}
}
