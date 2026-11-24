package files

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/cmd/maago/internal/gentest"
	"github.com/maago-web/maago/internal/s3fake"
)

// failing is a store whose Delete fails, and counts its calls.
type failing struct {
	maago.MediaStore
	deletes *int
}

func (f failing) Delete(context.Context, string) error {
	*f.deletes++
	return errors.New("the disk is read-only")
}

// SweepMedia deletes the old files no row keeps, of either key column or
// of the other app's, and the old halves a failed save left, on a disk
// and on S3 a page of two names at a time; it leaves the new ones and
// the names Maago does not make.
func TestSweep(t *testing.T) {
	dir, bucket := t.TempDir(), s3fake.New(t)
	for _, c := range []struct {
		name  string
		store maago.MediaStore
		touch func(name string, mod time.Time) // gives name's file, made if none, the time mod
		other []string                         // a name in the store that no store of Maago's makes
	}{
		{"local", maago.LocalMedia{Dir: dir}, func(name string, mod time.Time) {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err != nil {
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chtimes(path, mod, mod); err != nil {
				t.Fatal(err)
			}
		}, nil},
		{"s3", bucket, func(name string, mod time.Time) { s3fake.Touch(bucket, name, mod) }, []string{"a/b"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx, db := gentest.Open(t, "internal/testdata/media")
			ctx = maago.WithMedia(ctx, c.store)
			save := func() string {
				m, err := maago.SaveMediaFrom(ctx, "x.txt", strings.NewReader("x"))
				if err != nil {
					t.Fatal(err)
				}
				return m.Key
			}
			a, b, n, d, e, h := save(), save(), save(), save(), save(), save()
			if _, err := InsertFile(ctx, db, NewFileInput{Name: "a", Main: a, Thumb: &b}); err != nil {
				t.Fatal(err)
			}
			if _, err := InsertFile(ctx, db, NewFileInput{Name: "none"}); err != nil { // Thumb NULL
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, "INSERT INTO notes_note (attachment) VALUES ('"+n+"')"); err != nil {
				t.Fatal(err)
			}
			oldTmp, newTmp := ".tmp-"+a, ".tmp-"+strings.Repeat("f", 32) // a half is deleted though its key is in use
			now, old := time.Now(), time.Now().Add(-48*time.Hour)
			c.touch(h, now.Add(-2*time.Hour)) // within the 24 hours of 0
			for _, name := range []string{".gitignore", ".tmp-x", newTmp} {
				c.touch(name, now)
			}
			for _, name := range append([]string{a, b, n, d, oldTmp, ".gitignore", ".tmp-x"}, c.other...) {
				c.touch(name, old)
			}
			names := func() []string {
				var ns []string
				for {
					after := ""
					if len(ns) > 0 {
						after = ns[len(ns)-1]
					}
					page, err := c.store.List(ctx, after)
					if err != nil {
						t.Fatal(err)
					}
					if len(page) == 0 {
						return ns
					}
					for _, f := range page {
						ns = append(ns, f.Name())
					}
				}
			}
			all := names()

			if err := maago.SweepMedia(ctx, db, 72*time.Hour); err != nil {
				t.Errorf("72 hours: %v", err)
			}
			deletes := 0
			if err := maago.SweepMedia(maago.WithMedia(ctx, failing{c.store, &deletes}), db, 0); err == nil || !strings.Contains(err.Error(), "read-only") || deletes != 1 {
				t.Errorf("a failing Delete: %v after %d deletes, want its error after 1", err, deletes)
			}
			if got := names(); !slices.Equal(got, all) {
				t.Fatalf("before the sweep: %v, want %v", got, all)
			}

			want := slices.DeleteFunc(slices.Clone(all), func(name string) bool { return name == d || name == oldTmp })
			if len(want) != len(all)-2 {
				t.Fatalf("%v has not %s and %s", all, d, oldTmp)
			}
			for i := range 2 {
				if err := maago.SweepMedia(ctx, db, 0); err != nil {
					t.Fatalf("sweep %d: %v", i+1, err)
				}
				if got := names(); !slices.Equal(got, want) {
					t.Errorf("sweep %d left %v, want %v (a %s b %s n %s e %s h %s)", i+1, got, want, a, b, n, e, h)
				}
			}
		})
	}
}
