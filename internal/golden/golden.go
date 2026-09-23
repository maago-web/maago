// Package golden compares test output with expected files (D-67).
//
// Rewrite expected files by naming the packages to update:
//
//	go test ./internal/diag -update
//
// go test ./... -update does not work: test binaries that do not import
// this package reject the flag.
package golden

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files with the actual output")

// Check reports a failure if got differs from the file at path, which is
// relative to the test's package directory. With -update it writes got to
// path instead. It fails with Errorf only, so the calling test keeps running.
func Check(t testing.TB, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Errorf("golden: %v", err)
			return
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Errorf("golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("golden: %v\n  fix: run go test with -update in this package to create it", err)
		return
	}
	if !bytes.Equal(got, want) {
		t.Errorf("golden: %s differs at %s\n  fix: correct the code, or if the new output is right, rerun with -update", path, firstDiff(want, got))
	}
}

// firstDiff describes the first line where want and got differ.
// The inputs must differ.
func firstDiff(want, got []byte) string {
	w, g := lines(want), lines(got)
	for i := 0; ; i++ {
		if i >= len(w) || i >= len(g) || w[i] != g[i] {
			return fmt.Sprintf("line %d:\n  want %s\n  got  %s", i+1, lineAt(w, i), lineAt(g, i))
		}
	}
}

// lines splits b keeping each line's newline, so a missing final newline shows.
func lines(b []byte) []string {
	l := strings.SplitAfter(string(b), "\n")
	if l[len(l)-1] == "" {
		l = l[:len(l)-1]
	}
	return l
}

func lineAt(lines []string, i int) string {
	if i >= len(lines) {
		return "end of file"
	}
	return fmt.Sprintf("%q", lines[i])
}
