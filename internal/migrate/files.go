package migrate

import (
	"errors"
	"fmt"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
)

// Dir is the one migrations directory of a project, under its root (D-38).
const Dir = "migrations"

var fileName = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.sql$`)

// ValidName reports whether name can name a migration file.
func ValidName(name string) bool {
	return fileName.MatchString("0000_" + name + ".sql")
}

// File is one migration file.
type File struct {
	Path   string // slash path from the project root
	Number int
	Stmts  []dialect.Stmt
}

// ReadFiles reads and splits the migration files of the project at root,
// in number order. Numbers run from 1 without gaps or repeats (D-94).
// Without a migrations directory there are no files.
func ReadFiles(root string) ([]File, []diag.Diagnostic) {
	entries, err := os.ReadDir(filepath.Join(root, Dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []diag.Diagnostic{diag.New(token.Position{Filename: Dir}, "cannot read the migrations: "+diag.OneLine(err.Error()), "make sure "+Dir+" can be read")}
	}
	var files []File
	var ds []diag.Diagnostic
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		p := Dir + "/" + e.Name()
		m := fileName.FindStringSubmatch(e.Name())
		if m == nil {
			ds = append(ds, diag.New(token.Position{Filename: p}, fmt.Sprintf("migration name %q is not like 0001_initial.sql", e.Name()),
				"name it with four digits, _, then lower-case letters, digits and _"))
			continue
		}
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			ds = append(ds, diag.New(token.Position{Filename: p}, "cannot read the migration: "+diag.OneLine(err.Error()), "make sure it can be read"))
			continue
		}
		stmts, sds := dialect.Split(p, string(src))
		ds = append(ds, sds...)
		n, _ := strconv.Atoi(m[1])
		files = append(files, File{Path: p, Number: n, Stmts: stmts})
	}
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Path, b.Path) }) // number, then name
	prev := 0
	for _, f := range files {
		if same := slices.IndexFunc(files, func(o File) bool { return o.Number == f.Number && o.Path != f.Path }); same >= 0 {
			// Two branches that each made the next migration.
			ds = append(ds, diag.New(token.Position{Filename: f.Path}, fmt.Sprintf("migration number %04d is also used by %s", f.Number, files[same].Path),
				fmt.Sprintf("renumber the migration merged later to %04d, then run maago check again", files[len(files)-1].Number+1)))
		}
		if f.Number > prev+1 {
			ds = append(ds, diag.New(token.Position{Filename: f.Path}, fmt.Sprintf("migration %04d is missing before %s", prev+1, f.Path), "number the migrations from 0001 without gaps"))
		}
		prev = f.Number
	}
	return files, ds
}

// CheckFiles reports statements a migration must not hold, and drops
// without their marker (D-37). It needs no database.
func CheckFiles(files []File) []diag.Diagnostic {
	var ds []diag.Diagnostic
	for _, f := range files {
		trigger := false
		for i, st := range f.Stmts {
			// SQLite's trigger body splits at its semicolons; its closing
			// END has the same cause as the trigger, already reported.
			if trigger && len(st.Tokens) == 1 && word(st.Tokens, 0) == "END" {
				trigger = false
				continue
			}
			msg, fix := refused(st.Tokens)
			if msg == "triggers are not supported" {
				trigger = true // until its END, past the body's statements
			}
			if msg != "" {
				ds = append(ds, diag.New(st.Pos, msg, fix))
			}
			drops := dropping(st.Tokens)
			switch {
			case drops && !st.AllowDrop.IsValid() && rebuild(f.Stmts, i) == nil:
				what := "ALTER TABLE … DROP COLUMN"
				if word(st.Tokens, 0) == "DROP" {
					what = "DROP TABLE"
				}
				ds = append(ds, diag.New(st.Pos, what+" deletes data", `if that is intended, put "-- maago: allow-drop" on the line right before it`))
			case !drops && st.AllowDrop.IsValid():
				ds = append(ds, diag.New(st.AllowDrop, "-- maago: allow-drop is not before a DROP TABLE or DROP COLUMN statement", "remove it, or move it to the line right before the DROP"))
			}
		}
	}
	return ds
}

// word is the upper-cased text of toks[i] if it is an unquoted word.
func word(toks []dialect.Token, i int) string {
	if i < len(toks) && toks[i].Kind == dialect.Word {
		return strings.ToUpper(toks[i].Text)
	}
	return ""
}

// refused says what is wrong with a statement a migration must not hold,
// or returns "".
func refused(toks []dialect.Token) (msg, fix string) {
	const settings = "remove it; maago sets up the connection each migration runs on"
	outside := func(what string) (string, string) {
		return what + " is outside the schema maago checks", "create everything in the default schema"
	}
	w0, w1 := word(toks, 0), word(toks, 1)
	switch {
	case slices.Contains([]string{"BEGIN", "COMMIT", "END", "ROLLBACK", "ABORT", "SAVEPOINT", "RELEASE"}, w0),
		(w0 == "START" || w0 == "PREPARE") && w1 == "TRANSACTION":
		if w1 == "TRANSACTION" {
			w0 += " " + w1
		}
		return w0 + " is not allowed in a migration", "remove it; maago runs each migration in one transaction"
	case w0 == "SET" || w0 == "RESET" || w0 == "PRAGMA":
		return w0 + " is not allowed in a migration", settings
	case w0 == "DO":
		return "DO runs code maago cannot check", "write its statements as plain SQL"
	case w0 == "ATTACH" || w0 == "DETACH":
		return outside(w0 + " DATABASE")
	}
	for i := range toks {
		if word(toks, i) == "SET_CONFIG" && i+1 < len(toks) && toks[i+1].Text == "(" {
			return "set_config is not allowed in a migration", settings
		}
	}
	for i := range min(5, len(toks)) {
		if slices.Contains([]string{"CREATE", "DROP", "REINDEX"}, w0) && word(toks, i) == "CONCURRENTLY" {
			return "CONCURRENTLY cannot run inside a transaction, and maago runs each migration in one", "remove CONCURRENTLY"
		}
	}
	if w0 == "CREATE" {
		i := skip(toks, 1, "OR", "REPLACE", "GLOBAL", "LOCAL", "TEMP", "TEMPORARY", "UNIQUE", "MATERIALIZED", "UNLOGGED")
		obj := word(toks, i)
		switch {
		case slices.ContainsFunc(toks[1:i], func(t dialect.Token) bool { return strings.HasPrefix(strings.ToUpper(t.Text), "TEMP") }):
			return outside("a temporary " + strings.ToLower(obj))
		case obj == "VIEW" || obj == "TRIGGER" || obj == "EXTENSION":
			return strings.ToLower(obj) + "s are not supported", "remove it from the migrations"
		case obj == "SCHEMA":
			return outside("CREATE SCHEMA")
		case obj == "INDEX":
			j := skip(toks, i+1, "CONCURRENTLY", "IF", "NOT", "EXISTS")
			if word(toks, j) != "ON" && qualified(toks, j) {
				return outside("a schema-qualified name")
			}
			for j < len(toks) && word(toks, j) != "ON" {
				j++
			}
			if qualified(toks, skip(toks, j+1, "ONLY")) {
				return outside("a schema-qualified name")
			}
		case obj == "TABLE":
			if qualified(toks, skip(toks, i+1, "IF", "NOT", "EXISTS")) {
				return outside("a schema-qualified name")
			}
		}
	}
	if (w0 == "ALTER" || w0 == "DROP") && w1 == "TABLE" || w0 == "INSERT" && w1 == "INTO" {
		if qualified(toks, skip(toks, 2, "IF", "EXISTS", "ONLY")) {
			return outside("a schema-qualified name")
		}
	}
	return "", ""
}

// skip returns the index of the first token from i that is none of words.
func skip(toks []dialect.Token, i int, words ...string) int {
	for slices.Contains(words, word(toks, i)) {
		i++
	}
	return i
}

// qualified reports whether the name at toks[i] has a schema: s.t.
func qualified(toks []dialect.Token, i int) bool {
	return i+1 < len(toks) && (toks[i].Kind == dialect.Word || toks[i].Kind == dialect.Quoted) && toks[i+1].Text == "."
}

// dropping reports whether a statement deletes data: DROP TABLE, or an
// ALTER TABLE action that drops a column (PostgreSQL may leave out
// COLUMN), not a constraint and not ALTER COLUMN … DROP DEFAULT.
func dropping(toks []dialect.Token) bool {
	switch {
	case word(toks, 0) == "DROP" && word(toks, 1) == "TABLE":
		return true
	case word(toks, 0) != "ALTER" || word(toks, 1) != "TABLE":
		return false
	}
	name := skip(toks, 2, "IF", "EXISTS", "ONLY")
	depth := 0
	for i := name + 1; i < len(toks); i++ {
		switch toks[i].Text {
		case "(":
			depth++
		case ")":
			depth--
		}
		if depth == 0 && word(toks, i) == "DROP" && (i == name+1 || toks[i-1].Text == ",") && word(toks, i+1) != "CONSTRAINT" {
			return true
		}
	}
	return false
}

// rebuild returns the copy of the SQLite rebuild whose DROP TABLE t is
// stmts[i], or nil if stmts[i] is not one (D-93). The shape is the draft's:
// before it, an INSERT INTO x … SELECT … that ends at FROM t, so that no
// WHERE leaves rows behind; after it, ALTER TABLE x RENAME TO t.
func rebuild(stmts []dialect.Stmt, i int) *dialect.Stmt {
	toks := stmts[i].Tokens
	if word(toks, 0) != "DROP" || word(toks, 1) != "TABLE" {
		return nil
	}
	t := skip(toks, 2, "IF", "EXISTS")
	if t != len(toks)-1 {
		return nil // more than one table
	}
	for k := i - 1; k >= 0; k-- {
		ins := stmts[k].Tokens
		n := len(ins)
		if word(ins, 0) != "INSERT" || word(ins, 1) != "INTO" || n < 4 || word(ins, n-2) != "FROM" || !sameName(ins[n-1], toks[t]) {
			continue
		}
		for _, after := range stmts[i+1:] {
			r := after.Tokens
			if len(r) == 6 && word(r, 0) == "ALTER" && word(r, 1) == "TABLE" && sameName(r[2], ins[2]) &&
				word(r, 3) == "RENAME" && word(r, 4) == "TO" && sameName(r[5], toks[t]) {
				return &stmts[k]
			}
		}
	}
	return nil
}

// sameName compares two names as SQLite does: quotes off, case ignored.
func sameName(a, b dialect.Token) bool {
	return (a.Kind == dialect.Word || a.Kind == dialect.Quoted) && strings.EqualFold(dialect.Name(a), dialect.Name(b))
}
