package golden

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeT records failures instead of failing the real test.
type fakeT struct {
	testing.TB
	errs []string
}

func (f *fakeT) Helper() {}

func (f *fakeT) Errorf(format string, args ...any) {
	f.errs = append(f.errs, fmt.Sprintf(format, args...))
}

func TestCheck(t *testing.T) {
	defer func(old bool) { *update = old }(*update)
	*update = false // compare mode, even under go test -update

	dir := t.TempDir()
	path, empty := filepath.Join(dir, "x.golden"), filepath.Join(dir, "empty.golden")
	if err := os.WriteFile(path, []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
		got  string
		want []string // substrings of the failure; none means pass
	}{
		{"same", path, "a\nb\n", nil},
		{"one char", path, "a\nc\n", []string{path, "line 2", `want "b\n"`, `got  "c\n"`}},
		{"truncated", path, "a\n", []string{"line 2", `want "b\n"`, "got  end of file"}},
		{"empty want", empty, "\n", []string{"line 1", "want end of file", `got  "\n"`}},
		{"no final newline", path, "a\nb", []string{"line 2", `"b\n"`, `"b"`}},
		{"extra newline", path, "a\nb\n\n", []string{"line 3", "end of file"}},
		{"missing", path + ".nope", "a\n", []string{"-update"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeT{TB: t}
			Check(f, tt.path, []byte(tt.got))
			if len(tt.want) == 0 {
				if len(f.errs) > 0 {
					t.Fatalf("unexpected failure: %s", f.errs)
				}
				return
			}
			if len(f.errs) != 1 {
				t.Fatalf("want one failure, got %q", f.errs)
			}
			for _, w := range tt.want {
				if !strings.Contains(f.errs[0], w) {
					t.Errorf("failure lacks %q:\n%s", w, f.errs[0])
				}
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "x.golden")
	defer func(old bool) { *update = old }(*update)

	*update = true
	f := &fakeT{TB: t}
	Check(f, path, []byte("fresh\n"))
	if len(f.errs) > 0 {
		t.Fatalf("update failed: %s", f.errs)
	}

	*update = false
	Check(f, path, []byte("fresh\n"))
	if len(f.errs) > 0 {
		t.Fatalf("compare after update failed: %s", f.errs)
	}
}
