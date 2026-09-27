package migrate

import (
	"context"
	"fmt"
	"go/token"
	"slices"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/practicedb"
	"github.com/maago-web/maago/internal/project"
)

// Apply applies files in order to a practice database and stops at the
// first failure, in both dialects (D-61): PostgreSQL cannot go on after
// one anyway. SQLite applies each file with foreign keys off and checks
// them after, as migrate does.
func Apply(ctx context.Context, db *practicedb.DB, d project.Dialect, files []File) []diag.Diagnostic {
	for _, f := range files {
		if ds := applyFile(ctx, db, d, f, true, nil); len(ds) > 0 {
			return ds
		}
	}
	return nil
}

// ApplyDraft applies a draft as Apply applies a file, but without asking
// a rebuild that leaves columns behind for its marker: a draft never has
// one, and check asks for it once the draft is a file.
func ApplyDraft(ctx context.Context, db *practicedb.DB, d project.Dialect, f File) []diag.Diagnostic {
	return applyFile(ctx, db, d, f, false, nil)
}

// applyFile applies f. With record, the statements that record it as
// applied, it runs in a transaction of its own with them, as migrate does
// on a real database: all of it or none. A practice database has no
// record and needs no transaction of its own (D-96).
func applyFile(ctx context.Context, db *practicedb.DB, d project.Dialect, f File, checkLost bool, record []string) (ds []diag.Diagnostic) {
	at := token.Position{Filename: f.Path}
	if d == project.SQLite {
		// Before BEGIN, and back on only after the transaction ends:
		// SQLite ignores the pragma inside one.
		if ds := db.Exec(ctx, at, "PRAGMA foreign_keys = OFF"); len(ds) > 0 {
			return ds
		}
		defer func() {
			if on := db.Exec(ctx, at, "PRAGMA foreign_keys = ON"); len(ds) == 0 {
				ds = on
			}
		}()
	}
	if record != nil {
		if ds := db.Exec(ctx, at, "BEGIN"); len(ds) > 0 {
			return ds
		}
		defer func() {
			if len(ds) > 0 {
				// SQLite may have rolled back already; then this fails,
				// and that is fine.
				db.Exec(ctx, at, "ROLLBACK")
			}
		}()
		if d == project.Postgres {
			// Backslashes in '…' are plain, as dialect.Split reads them.
			if ds := db.Exec(ctx, at, "SET LOCAL standard_conforming_strings = on"); len(ds) > 0 {
				return ds
			}
		}
	}
	for i, st := range f.Stmts {
		if cp := rebuild(f.Stmts, i); checkLost && cp != nil && !st.AllowDrop.IsValid() {
			if ds := lost(ctx, db, st, cp); len(ds) > 0 {
				return ds
			}
		}
		if ds := db.Exec(ctx, st.Pos, st.SQL); len(ds) > 0 {
			return ds
		}
	}
	for _, stmt := range record {
		if ds := db.Exec(ctx, at, stmt); len(ds) > 0 {
			return ds
		}
	}
	if d == project.SQLite {
		if ds := foreignKeyCheck(ctx, db, at); len(ds) > 0 {
			return ds
		}
	}
	if record != nil {
		return db.Exec(ctx, at, "COMMIT")
	}
	return nil
}

// foreignKeyCheck reports rows whose foreign keys point nowhere.
func foreignKeyCheck(ctx context.Context, db *practicedb.DB, at token.Position) []diag.Diagnostic {
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return dialect.Bug(err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return dialect.Bug(err)
		}
		return nil
	}
	var table string
	var rowid, parent, fkid any
	if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
		return dialect.Bug(err)
	}
	return []diag.Diagnostic{diag.New(at, fmt.Sprintf("after this migration, a row of %q has a foreign key that points nowhere", table), "fix the statements or the data of this migration")}
}

// lost reports the columns of the table a rebuild drops that its copy
// does not select, whose values the DROP loses (D-93). A name in the
// SELECT list counts as copied, but not an alias after AS; * copies all.
// The table's columns come from the database itself, the same way in
// both dialects.
func lost(ctx context.Context, db *practicedb.DB, drop dialect.Stmt, cp *dialect.Stmt) []diag.Diagnostic {
	t := drop.Tokens[len(drop.Tokens)-1]
	rows, err := db.QueryContext(ctx, "SELECT * FROM "+t.Text+" LIMIT 0")
	if err != nil {
		return nil // the DROP itself will say what is wrong
	}
	cols, err := rows.Columns()
	rows.Close()
	if err != nil {
		return dialect.Bug(err)
	}
	toks := cp.Tokens
	from := len(toks) - 2
	start := 0
	for start < from && word(toks, start) != "SELECT" {
		start++
	}
	var selected []string
	for i := start + 1; i < from; i++ {
		switch tok := toks[i]; {
		case tok.Text == "*":
			return nil
		case word(toks, i) == "AS":
			i++ // an alias names the new column, not an old one
		case tok.Kind == dialect.Word || tok.Kind == dialect.Quoted:
			selected = append(selected, strings.ToLower(dialect.Name(tok)))
		}
	}
	var gone []string
	for _, c := range cols {
		if !slices.Contains(selected, strings.ToLower(c)) {
			gone = append(gone, c)
		}
	}
	if len(gone) == 0 {
		return nil
	}
	return []diag.Diagnostic{diag.New(drop.Pos, fmt.Sprintf("DROP TABLE %q loses the values of %s, which the copy above does not select", dialect.Name(t), dialect.QuoteAll(gone)),
		`copy them in the INSERT above, or if that is intended, put "-- maago: allow-drop" on the line right before the DROP`)}
}
