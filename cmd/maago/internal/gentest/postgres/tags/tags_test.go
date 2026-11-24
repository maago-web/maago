package tags

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/cmd/maago/internal/gentest"
)

// A program that links only this app, which keeps no media, sweeps with
// the project's media keys: it keeps the file of another app's row.
func TestSweep(t *testing.T) {
	ctx, db := gentest.Open(t, "internal/testdata/media")
	dir := t.TempDir()
	kept, orphan := strings.Repeat("a", 32), strings.Repeat("b", 32)
	old := time.Now().Add(-48 * time.Hour)
	for _, key := range []string{kept, orphan} {
		path := filepath.Join(dir, key)
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO files_file (name, main) VALUES ('kept', '"+kept+"')"); err != nil {
		t.Fatal(err)
	}
	if err := maago.SweepMedia(maago.WithMedia(ctx, maago.LocalMedia{Dir: dir}), db, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, kept)); err != nil {
		t.Errorf("the kept file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, orphan)); !os.IsNotExist(err) {
		t.Errorf("the orphan: %v, want it gone", err)
	}
}
