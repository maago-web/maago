package build

import (
	"context"
	"errors"
	"fmt"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/migrate"
	"github.com/maago-web/maago/internal/pipeline"
	"github.com/maago-web/maago/internal/project"
)

// MakeMigrations checks the project holding dir as check does and, if the
// models differ from the migrations, writes the next migration: a draft
// for a person or an agent to read and change (D-32). name overrides the
// name made from the changes; each of renames, table.old:new, turns a
// dropped and an added column into a rename (D-36). It returns the path
// it wrote, from the root, or "" when nothing differs.
func MakeMigrations(dir, name string, renames []string) (written string, ds []diag.Diagnostic) {
	root, ds := project.Root(dir)
	if len(ds) > 0 {
		return "", ds
	}
	s := &state{root: root}
	var rs []migrate.Rename
	early := func() []diag.Diagnostic {
		var ds []diag.Diagnostic
		if name != "" && !migrate.ValidName(name) {
			ds = append(ds, diag.New(token.Position{}, fmt.Sprintf("--name %q is not lower-case letters, digits and _", name), "name it like add_blog_post_body"))
		}
		seen := map[string]bool{}
		for _, arg := range renames {
			r, ok := migrate.ParseRename(arg)
			if !ok {
				ds = append(ds, diag.New(token.Position{}, fmt.Sprintf("--rename %q is not table.old:new", arg), "write table.old_column:new_column, with database names"))
				continue
			}
			for _, col := range []string{r.Old, r.New} {
				if seen[r.Table+"."+col] {
					ds = append(ds, diag.New(token.Position{}, fmt.Sprintf("--rename %q names column %q of %q again", arg, col, r.Table), "rename each column once"))
				}
				seen[r.Table+"."+col] = true
			}
			rs = append(rs, r)
		}
		return ds
	}
	final := func() (ds []diag.Diagnostic) {
		written, ds = draftDatabase(s, name, rs)
		return ds
	}
	if ds := pipeline.Generate(&pipeline.Run{Root: root}, stages(s, early, final)); len(ds) > 0 {
		return "", ds
	}
	return written, nil
}

// draftDatabase compares the migrations with the models as checkDatabase
// does and writes the difference as the next migration.
func draftDatabase(s *state, name string, rs []migrate.Rename) (string, []diag.Diagnostic) {
	ctx := context.Background()
	a, read, ds := compared(ctx, s)
	if len(ds) > 0 {
		return "", ds
	}
	defer a.Close()
	cs, ds := read()
	if len(ds) > 0 {
		return "", ds
	}

	// A rename is applied first, then the rest is drafted: the draft
	// knows nothing of renames.
	var parts []string
	for _, r := range rs {
		not := func(col, what string) {
			ds = append(ds, diag.New(token.Position{}, fmt.Sprintf("--rename %s.%s:%s: column %q of %q is not %s", r.Table, r.Old, r.New, col, r.Table, what),
				"check the names: a rename turns a column the migrations have into one the models have"))
		}
		if !slices.ContainsFunc(cs, func(c migrate.Change) bool {
			return c.Kind == migrate.DropColumn && c.Table() == r.Table && c.Old.Name == r.Old
		}) {
			not(r.Old, "dropped")
		}
		if !slices.ContainsFunc(cs, func(c migrate.Change) bool {
			return c.Kind == migrate.AddColumn && c.Table() == r.Table && c.New.Name == r.New
		}) {
			not(r.New, "added")
		}
	}
	if len(ds) > 0 {
		return "", ds
	}
	for _, r := range rs {
		stmt := fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s;", dialect.Quote(r.Table), dialect.Quote(r.Old), dialect.Quote(r.New))
		if ds := a.Exec(ctx, token.Position{}, stmt); len(ds) > 0 {
			return "", ds
		}
		parts = append(parts, stmt)
	}
	if len(rs) > 0 {
		if cs, ds = read(); len(ds) > 0 {
			return "", ds
		}
	}
	if len(cs) == 0 && len(rs) == 0 {
		return "", nil
	}

	number := 1
	if n := len(s.migrations); n > 0 {
		number = s.migrations[n-1].Number + 1
	}
	switch {
	case name != "":
	case number == 1:
		name = "initial"
	default:
		name = migrate.Name(cs, rs)
	}
	path := fmt.Sprintf("%s/%04d_%s.sql", migrate.Dir, number, name)
	if draft := dialects[s.d].draft(s.apps, cs); draft != "" {
		parts = append(parts, strings.TrimSuffix(draft, "\n"))
	}
	src := renameWarnings(cs) + strings.Join(parts, "\n\n") + "\n"

	// Apply the draft once more, renames aside, before writing it.
	stmts, sds := dialect.Split(path, src)
	if len(sds) > 0 || len(stmts) < len(rs) {
		return "", []diag.Diagnostic{diag.New(token.Position{}, "the draft of "+path+" does not split into statements", "report this as a maago bug")}
	}
	if ds := migrate.ApplyDraft(ctx, a, s.d, migrate.File{Path: path, Stmts: stmts[len(rs):]}); len(ds) > 0 {
		lead := diag.New(token.Position{}, "maago did not write "+path+": its draft fails in the practice database", "write this migration by hand, starting from the error below")
		return "", append([]diag.Diagnostic{lead}, ds...)
	}
	left, ds := read()
	if len(ds) > 0 {
		return "", ds
	}
	if len(left) > 0 {
		return "", []diag.Diagnostic{diag.New(token.Position{}, "the draft leaves the migrations unlike the models: "+strings.TrimPrefix(left[0].Message(), "migrations do not match models: "), "report this as a maago bug")}
	}

	if ds := writeMigration(s.root, path, src); len(ds) > 0 {
		return "", ds
	}
	return path, nil
}

// writeMigration writes a new migration. It is not a generated file: it is
// never overwritten and has no marker (ARCHITECTURE §5). A failed write
// leaves no half-written file behind.
func writeMigration(root, path, src string) []diag.Diagnostic {
	full := filepath.Join(root, filepath.FromSlash(path))
	err := os.MkdirAll(filepath.Dir(full), 0o755)
	var f *os.File
	if err == nil {
		f, err = os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	}
	if err == nil {
		_, err = f.WriteString(src)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(full)
		}
	}
	switch {
	case errors.Is(err, fs.ErrExist):
		return []diag.Diagnostic{diag.New(token.Position{Filename: path}, "migration already exists", "run maago makemigrations again")}
	case err != nil:
		return []diag.Diagnostic{diag.New(token.Position{Filename: path}, "cannot write the migration: "+diag.OneLine(err.Error()), "make sure "+migrate.Dir+" can be written")}
	}
	return nil
}

// renameWarnings warns, for each table that drops one column and adds
// one, that the two may be a rename (D-36): a drop and an add lose the
// values a rename keeps. The file holds an unmarked DROP, so check fails
// until it is deleted or marked.
func renameWarnings(cs []migrate.Change) string {
	drops, adds := map[string][]string{}, map[string][]string{}
	for _, c := range cs {
		switch c.Kind {
		case migrate.DropColumn:
			drops[c.Table()] = append(drops[c.Table()], c.Old.Name)
		case migrate.AddColumn:
			adds[c.Table()] = append(adds[c.Table()], c.New.Name)
		}
	}
	var b strings.Builder
	for _, c := range cs {
		if t := c.Table(); c.Kind == migrate.DropColumn && len(drops[t]) == 1 && len(adds[t]) == 1 {
			old, new := drops[t][0], adds[t][0]
			fmt.Fprintf(&b, "-- ⚠ Column %q of %q is dropped and column %q is added. If %q is %q renamed, keep its values: delete this file, then run maago makemigrations --rename %s.%s:%s\n\n",
				old, t, new, new, old, t, old, new)
		}
	}
	return b.String()
}
