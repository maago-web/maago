package migrate

import (
	"context"
	"fmt"
	"go/token"
	"maps"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/practicedb"
	"github.com/maago-web/maago/internal/project"
)

// Table records the migrations a database has applied, by file name and
// hash (D-95).
const Table = "maago_migrations"

// Records returns the applied migrations of db, file name to hash. A
// database without the table has applied none.
func Records(ctx context.Context, db *practicedb.DB, d project.Dialect) (map[string]string, []diag.Diagnostic) {
	exists := "SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = '" + Table + "'"
	if d == project.Postgres {
		exists = "SELECT (to_regclass('" + Table + "') IS NOT NULL)::int"
	}
	records := map[string]string{}
	var n int64
	r := &dialect.Reader{Ctx: ctx, Q: db}
	cannot := func(err error) []diag.Diagnostic {
		// Permissions, or a table of that name made by something else.
		return []diag.Diagnostic{diag.New(token.Position{}, "cannot read "+Table+": "+diag.OneLine(err.Error()), "check MAAGO_DATABASE_URL and the table "+Table)}
	}
	if err := r.Rows(exists, func(v []any) { n = dialect.Num(v[0]) }); err != nil {
		return nil, cannot(err)
	}
	if n == 0 {
		return records, nil
	}
	if err := r.Rows("SELECT name, hash FROM "+Table, func(v []any) { records[dialect.Str(v[0])] = dialect.Str(v[1]) }); err != nil {
		return nil, cannot(err)
	}
	return records, nil
}

// Match compares the files with what the database applied, and returns
// the files not applied, in number order: also those numbered before an
// applied one, which merging branches makes (D-99). A file changed since
// it was applied, or a record without its file, is an error (D-95).
func Match(files []File, records map[string]string) (pending []File, ds []diag.Diagnostic) {
	byName := map[string]bool{}
	for _, f := range files {
		name := path.Base(f.Path)
		byName[name] = true
		hash, ok := records[name]
		switch {
		case !ok:
			pending = append(pending, f)
		case hash != f.Hash:
			ds = append(ds, diag.New(token.Position{Filename: f.Path}, "migration was changed after it was applied", "undo the change, or make it a new migration"))
		}
	}
	renamed := map[string]bool{} // files a fix already gives a record's name
	for _, name := range slices.Sorted(maps.Keys(records)) {
		if byName[name] {
			continue
		}
		var same []File
		for _, f := range pending {
			if f.Hash == records[name] && !renamed[f.Path] {
				same = append(same, f)
			}
		}
		switch len(same) {
		case 0:
			ds = append(ds, diag.New(token.Position{}, fmt.Sprintf("applied migration %s has no file", name), "restore it from version control"))
		case 1:
			// Renumbered, as when merging two branches.
			renamed[same[0].Path] = true
			ds = append(ds, diag.New(token.Position{Filename: same[0].Path}, "migration was applied as "+name,
				fmt.Sprintf("run on the database: UPDATE %s SET name = %s WHERE name = %s;", Table, dialect.String(path.Base(same[0].Path)), dialect.String(name))))
		default:
			var paths []string
			for _, f := range same {
				paths = append(paths, f.Path)
			}
			ds = append(ds, diag.New(token.Position{}, fmt.Sprintf("applied migration %s matches several files: %s", name, strings.Join(paths, ", ")),
				"restore its file under its applied name"))
		}
	}
	if len(ds) > 0 {
		return nil, ds
	}
	return pending, nil
}

// Run applies files to db, each in a transaction of its own with its
// record, and stops at the first failure: the files before it stay. It
// returns the names of the files it applied.
func Run(ctx context.Context, db *practicedb.DB, d project.Dialect, files []File) (applied []string, ds []diag.Diagnostic) {
	for _, f := range files {
		name := path.Base(f.Path)
		record := []string{
			"CREATE TABLE IF NOT EXISTS " + Table + " (name TEXT PRIMARY KEY, hash TEXT NOT NULL, applied_at TEXT NOT NULL)",
			fmt.Sprintf("INSERT INTO %s (name, hash, applied_at) VALUES (%s, %s, %s)", Table,
				dialect.String(name), dialect.String(f.Hash), dialect.String(time.Now().UTC().Format(time.RFC3339))),
		}
		if ds := applyFile(ctx, db, d, f, true, record); len(ds) > 0 {
			return applied, ds
		}
		applied = append(applied, name)
	}
	return applied, nil
}

// Show lists each file as applied [x] or not [ ], then how many are.
func Show(files []File, records map[string]string) string {
	if len(files) == 0 {
		return "no migrations\n"
	}
	var b strings.Builder
	n := 0
	for _, f := range files {
		mark := "[ ]"
		if _, ok := records[path.Base(f.Path)]; ok {
			mark = "[x]"
			n++
		}
		fmt.Fprintf(&b, "%s %s\n", mark, path.Base(f.Path))
	}
	fmt.Fprintf(&b, "%d of %d applied\n", n, len(files))
	return b.String()
}
