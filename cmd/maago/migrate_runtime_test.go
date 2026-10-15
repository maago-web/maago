package main

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/internal/migrate"
	"github.com/maago-web/maago/internal/project"
)

// maagoMigrate is maago.Migrate as a binary runs it, from the project
// root, of the files it embeds.
func (r realDB) maagoMigrate(t *testing.T) ([]string, error) {
	t.Helper()
	t.Chdir(r.root)
	return maago.Migrate(ctx(t), maago.DBConfig{}, os.DirFS(r.root))
}

// maago.Migrate and maago migrate keep one record: each sees what the
// other applied, and both refuse what the other refuses (D-153).
func TestMaagoMigrate(t *testing.T) {
	eachDialect(t, func(t *testing.T, d project.Dialect) {
		r := newRealDB(t, d)
		r.write(t, map[string]string{"0001_a.sql": threeFiles["0001_a.sql"], "0002_b.sql": threeFiles["0002_b.sql"]})
		applied, err := r.maagoMigrate(t)
		if err != nil || !slices.Equal(applied, []string{"0001_a.sql", "0002_b.sql"}) {
			t.Fatalf("Migrate: %v %v", applied, err)
		}
		if _, err := os.Stat(filepath.Join(r.root, "app.db")); d == project.SQLite && err != nil {
			t.Errorf("Migrate made no SQLite file: %v", err)
		}
		// WAL already: OpenDB's switch would need the file alone, which
		// replicas that start at once do not leave it.
		if d == project.SQLite {
			if got := rowsText(t, r.open(t), "PRAGMA journal_mode"); got != "wal\n" {
				t.Errorf("journal mode: %q", got)
			}
		}
		r.migrate(t) // nothing for the CLI
		r.write(t, map[string]string{"0003_add_y.sql": threeFiles["0003_add_y.sql"]})
		r.migrate(t, "0003_add_y.sql")
		if applied, err := r.maagoMigrate(t); err != nil || applied != nil {
			t.Errorf("Migrate after the CLI: %v %v", applied, err)
		}
		if got, want := r.show(t), "[x] 0001_a.sql\n[x] 0002_b.sql\n[x] 0003_add_y.sql\n3 of 3 applied\n"; got != want {
			t.Errorf("show:\n%s", got)
		}

		r.write(t, map[string]string{"0002_b.sql": strings.Replace(threeFiles["0002_b.sql"], "points", "Points", 1)})
		if _, err := r.maagoMigrate(t); err == nil || !strings.HasPrefix(err.Error(), "maago: migrations/0002_b.sql: migration was changed after it was applied\n") {
			t.Errorf("a changed file: %v", err)
		}
		r.write(t, map[string]string{"0002_b.sql": threeFiles["0002_b.sql"]})
		os.Remove(filepath.Join(r.root, "migrations", "0003_add_y.sql")) // an older binary's files
		if _, err := r.maagoMigrate(t); err == nil || !strings.Contains(err.Error(), "applied migration 0003_add_y.sql has no file: the database is newer than this code") {
			t.Errorf("a newer database: %v", err)
		}
		sub, _ := fs.Sub(os.DirFS(r.root), "migrations")
		if _, err := maago.Migrate(ctx(t), maago.DBConfig{}, sub); err == nil || !strings.Contains(err.Error(), "fsys has no migrations directory") {
			t.Errorf("the migrations directory as fsys: %v", err)
		}
	})
}

// Processes that migrate at once, as replicas that start together do,
// apply each file once between them; on SQLite, one waits for a write
// that is going on (D-153).
func TestMaagoMigrateAtOnce(t *testing.T) {
	eachDialect(t, func(t *testing.T, d project.Dialect) {
		r := newRealDB(t, d)
		r.write(t, threeFiles)
		t.Chdir(r.root)
		if d == project.SQLite { // the file, and a write of the app's that holds it
			db, err := sql.Open("sqlite", "file:"+filepath.Join(r.root, "app.db")+"?_pragma=journal_mode(WAL)")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			conn, err := db.Conn(ctx(t))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := conn.ExecContext(ctx(t), "BEGIN IMMEDIATE"); err != nil {
				t.Fatal(err)
			}
			go func() {
				time.Sleep(300 * time.Millisecond)
				conn.ExecContext(context.Background(), "COMMIT")
				conn.Close()
			}()
		}
		var mu sync.Mutex
		var all []string
		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() {
				applied, err := maago.Migrate(ctx(t), maago.DBConfig{}, os.DirFS(r.root))
				if err != nil {
					t.Error(err)
				}
				mu.Lock()
				all = append(all, applied...)
				mu.Unlock()
			})
		}
		wg.Wait()
		if slices.Sort(all); !slices.Equal(all, []string{"0001_a.sql", "0002_b.sql", "0003_add_y.sql"}) {
			t.Errorf("applied %v between them", all)
		}
		if got := rowsText(t, r.open(t), "SELECT name FROM "+migrate.Table+" ORDER BY name"); got != "0001_a.sql\n0002_b.sql\n0003_add_y.sql\n" {
			t.Errorf("records: %q", got)
		}
	})
}
