package sqlite

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"go/token"
	"slices"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/schema"
)

// Querier runs a query; a practice database is one.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Read reads the schema of a SQLite database: the catalog, and for what
// the catalog leaves out (CHECK, AUTOINCREMENT, STRICT, constraint names)
// the CREATE TABLE text (D-92). What the schema cannot hold, such as a
// partial index, is reported rather than skipped (PHILOSOPHY 7), since a
// comparison would not see it. Every table is read, so that all such
// reports come at once; a caller compares nothing if there are any.
func Read(ctx context.Context, q Querier) (*schema.Schema, []diag.Diagnostic) {
	r := &reader{ctx: ctx, q: q}
	s := &schema.Schema{}
	shadow := map[string]bool{} // tables a virtual table keeps its data in
	var texts []string          // CREATE TABLE text of each table
	err := r.rows("SELECT name FROM pragma_table_list WHERE schema = 'main' AND type = 'shadow'", func(v []any) {
		shadow[str(v[0])] = true
	})
	if err != nil {
		return nil, bug(err)
	}
	err = r.rows("SELECT type, name, sql FROM sqlite_schema ORDER BY name", func(v []any) {
		kind, name, text := str(v[0]), str(v[1]), str(v[2])
		switch {
		case kind == "view" || kind == "trigger":
			r.fail(fmt.Sprintf("%s %q is not supported", kind, name), "remove it from the migrations")
		case kind != "table" || shadow[name] || strings.HasPrefix(name, "sqlite_") || name == "maago_migrations":
		case strings.HasPrefix(strings.ToUpper(text), "CREATE VIRTUAL"):
			r.fail(fmt.Sprintf("virtual table %q is not supported", name), "remove it from the migrations")
		default:
			s.Tables = append(s.Tables, &schema.Table{Name: name})
			texts = append(texts, text)
		}
	})
	for i, t := range s.Tables {
		if err == nil {
			err = r.table(t, texts[i])
		}
	}
	if err != nil {
		return nil, bug(err)
	}
	r.resolve(s)
	return s, r.ds
}

func bug(err error) []diag.Diagnostic {
	return []diag.Diagnostic{diag.New(token.Position{}, "cannot read the practice database: "+diag.OneLine(err.Error()), "report this as a maago bug")}
}

type reader struct {
	ctx context.Context
	q   Querier
	ds  []diag.Diagnostic
}

func (r *reader) fail(msg, fix string) {
	r.ds = append(r.ds, diag.New(token.Position{}, msg, fix))
}

func (r *reader) unsupported(t *schema.Table, what string) {
	r.fail(fmt.Sprintf("table %q has %s, which maago does not support", t.Name, what), "remove it from the migrations")
}

// rows calls f with the values of each row. f must not query: the
// practice database has one connection, busy until the rows are read.
func (r *reader) rows(query string, f func([]any), args ...any) error {
	rows, err := r.q.QueryContext(r.ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	for rows.Next() {
		v := make([]any, len(cols))
		p := make([]any, len(cols))
		for i := range v {
			p[i] = &v[i]
		}
		if err := rows.Scan(p...); err != nil {
			return err
		}
		f(v)
	}
	return rows.Err()
}

func str(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	}
	return ""
}

func num(v any) int64 {
	n, _ := v.(int64)
	return n
}

func (r *reader) table(t *schema.Table, text string) error {
	fkNames, collated := r.parse(t, text)
	pk := map[int64]string{} // by position in the key
	err := r.rows("SELECT name, type, \"notnull\", dflt_value, pk, hidden FROM pragma_table_xinfo(?) ORDER BY cid", func(v []any) {
		c := schema.Column{Name: str(v[0]), Type: strings.ToUpper(str(v[1])), NotNull: num(v[2]) == 1}
		if v[3] != nil {
			c.Default = normalize(str(v[3]))
		}
		if num(v[5]) != 0 {
			r.unsupported(t, fmt.Sprintf("generated column %q", c.Name))
		}
		t.Columns = append(t.Columns, c)
		if n := num(v[4]); n > 0 {
			pk[n] = c.Name
		}
	}, t.Name)
	if err != nil {
		return err
	}
	for n := int64(1); pk[n] != ""; n++ {
		t.PK = append(t.PK, pk[n])
	}

	type index struct {
		name            string
		unique, partial bool
		origin          string
	}
	var indexes []index
	err = r.rows("SELECT name, \"unique\", origin, partial FROM pragma_index_list(?) ORDER BY name", func(v []any) {
		indexes = append(indexes, index{str(v[0]), num(v[1]) == 1, num(v[3]) == 1, str(v[2])})
	}, t.Name)
	if err != nil {
		return err
	}
	for _, ix := range indexes {
		what := fmt.Sprintf("index %q", ix.name)
		if ix.origin == "pk" {
			what = "primary key" // checked like an index, but t.PK holds it
		}
		if ix.partial {
			r.unsupported(t, fmt.Sprintf("partial index %q", ix.name))
			continue
		}
		out := schema.Index{Name: ix.name, Unique: ix.unique, Constraint: ix.origin == "u"}
		ok := true
		err := r.rows("SELECT cid, name, \"desc\", coll FROM pragma_index_xinfo(?) WHERE key = 1 ORDER BY seqno", func(v []any) {
			col := str(v[1])
			switch {
			case num(v[0]) == -2:
				r.unsupported(t, what+" on an expression")
				ok = false
			case num(v[2]) == 1:
				r.unsupported(t, "descending "+what)
				ok = false
			case !strings.EqualFold(str(v[3]), "BINARY") && !collated[col]:
				r.unsupported(t, what+" with COLLATE")
				ok = false
			}
			out.Columns = append(out.Columns, col)
		}, ix.name)
		if err != nil {
			return err
		}
		if ok && ix.origin != "pk" {
			t.Indexes = append(t.Indexes, out)
		}
	}

	byID := map[int64]*schema.FK{}
	var ids []int64
	err = r.rows("SELECT id, \"table\", \"from\", \"to\", on_update, on_delete FROM pragma_foreign_key_list(?) ORDER BY id, seq", func(v []any) {
		id := num(v[0])
		fk := byID[id]
		if fk == nil {
			fk = &schema.FK{RefTable: str(v[1]), OnUpdate: str(v[4]), OnDelete: str(v[5])}
			byID[id] = fk
			ids = append(ids, id)
		}
		fk.Columns = append(fk.Columns, str(v[2]))
		if v[3] != nil {
			fk.RefColumns = append(fk.RefColumns, str(v[3]))
		}
	}, t.Name)
	if err != nil {
		return err
	}
	for _, id := range ids {
		fk := byID[id]
		fk.Name = fkNames[fkKey(fk.Columns, fk.RefTable)]
		t.FKs = append(t.FKs, *fk)
	}
	return nil
}

func normalize(sql string) string {
	stmts, _ := dialect.Split("", sql)
	if len(stmts) != 1 {
		return sql
	}
	return dialect.Normalize(stmts[0].Tokens)
}

// parse reads from the CREATE TABLE text what the catalog leaves out. It
// returns the names of foreign keys by their columns, and the columns
// declared with COLLATE, whose indexes are then not reported again.
func (r *reader) parse(t *schema.Table, text string) (fkNames map[string]string, collated map[string]bool) {
	fkNames, collated = map[string]string{}, map[string]bool{}
	stmts, _ := dialect.Split("", text) // stored text is well formed; markers were reported with the file
	if len(stmts) != 1 {
		r.fail(fmt.Sprintf("cannot read the definition of table %q", t.Name), "report this as a maago bug")
		return
	}
	// SQLite stores even CREATE TABLE … AS SELECT as a list of columns.
	toks := stmts[0].Tokens
	open := slices.IndexFunc(toks, func(tok dialect.Token) bool { return tok.Text == "(" })
	if open < 0 {
		r.fail(fmt.Sprintf("cannot read the definition of table %q", t.Name), "report this as a maago bug")
		return
	}
	end := closing(toks, open)
	for _, tok := range toks[end+1:] {
		switch word(tok) {
		case "STRICT":
			t.Strict = true
		case "WITHOUT":
			r.unsupported(t, "WITHOUT ROWID")
		}
	}
	for _, def := range splitTop(toks[open+1 : end]) {
		col := ""
		switch word(def[0]) {
		case "CONSTRAINT", "PRIMARY", "UNIQUE", "CHECK", "FOREIGN":
		default:
			if def[0].Kind != dialect.Word && def[0].Kind != dialect.Quoted { // [name] or 'name'
				r.fail(fmt.Sprintf("table %q has a column name quoted with %s, which maago does not support", t.Name, def[0].Text), `quote the name with "…" instead`)
				continue
			}
			col = name(def[0])
		}
		r.definition(t, col, def, fkNames, collated)
	}
	return
}

// definition reads one column definition (col != "") or table constraint.
func (r *reader) definition(t *schema.Table, col string, def []dialect.Token, fkNames map[string]string, collated map[string]bool) {
	constraint := ""
	var fkCols []string // of a FOREIGN KEY waiting for its REFERENCES
	fkName := ""
	start := 0
	if col != "" {
		start = 1 // past the column's name, which may be a word such as match
	}
	for i := start; i < len(def); i++ {
		switch w := word(def[i]); w {
		case "CONSTRAINT":
			if i+1 < len(def) {
				constraint = name(def[i+1])
				i++
			}
			continue
		case "CHECK":
			if i+1 < len(def) && def[i+1].Text == "(" {
				end := closing(def, i+1)
				t.Checks = append(t.Checks, schema.Check{Name: constraint, Column: col, Expr: dialect.Normalize(def[i+2 : end])})
				i = end
			}
		case "FOREIGN": // KEY (cols) … REFERENCES
			fkName = constraint
			if i+2 < len(def) && def[i+2].Text == "(" {
				end := closing(def, i+2)
				for _, tok := range def[i+3 : end] {
					if tok.Text != "," {
						fkCols = append(fkCols, name(tok))
					}
				}
				i = end
			}
		case "REFERENCES":
			if fkCols == nil { // on a column
				fkCols, fkName = []string{col}, constraint
			}
			if i+1 < len(def) {
				fkNames[fkKey(fkCols, name(def[i+1]))] = fkName
			}
			fkCols = nil
		case "AUTOINCREMENT":
			t.PKAuto = "AUTOINCREMENT"
		case "PRIMARY": // KEY (col AUTOINCREMENT) is allowed too
			if i+2 < len(def) && def[i+2].Text == "(" {
				end := closing(def, i+2)
				if slices.ContainsFunc(def[i+3:end], func(tok dialect.Token) bool { return word(tok) == "AUTOINCREMENT" }) {
					t.PKAuto = "AUTOINCREMENT"
				}
				i = end
			}
		case "COLLATE", "CONFLICT", "DEFERRABLE", "MATCH":
			what := w
			if w == "CONFLICT" {
				what = "ON CONFLICT"
			}
			if col != "" {
				what += " on column " + fmt.Sprintf("%q", col)
				if w == "COLLATE" {
					collated[col] = true
				}
			}
			r.unsupported(t, what)
		}
		if def[i].Text == "(" {
			i = closing(def, i) // only depth 0 counts
		}
		constraint = "" // a name belongs to the constraint right after it
	}
}

// resolve fills in foreign keys that name no columns, which reference the
// primary key, and reports ones referencing a table that does not exist.
func (r *reader) resolve(s *schema.Schema) {
	tables := map[string]*schema.Table{}
	for _, t := range s.Tables {
		tables[t.Name] = t
	}
	for _, t := range s.Tables {
		for i := range t.FKs {
			fk := &t.FKs[i]
			ref := tables[fk.RefTable]
			switch {
			case ref == nil:
				r.fail(fmt.Sprintf("foreign key of %q references unknown table %q", t.Name, fk.RefTable), "create that table in the migrations, or fix the name after REFERENCES")
			case len(fk.RefColumns) == 0:
				fk.RefColumns = ref.PK
			}
		}
		slices.SortFunc(t.FKs, func(a, b schema.FK) int {
			return cmp.Or(slices.Compare(a.Columns, b.Columns), cmp.Compare(a.RefTable, b.RefTable))
		})
		slices.SortFunc(t.Indexes, func(a, b schema.Index) int {
			return cmp.Or(compareBool(a.Unique, b.Unique), slices.Compare(a.Columns, b.Columns), cmp.Compare(a.Name, b.Name))
		})
		slices.SortFunc(t.Checks, func(a, b schema.Check) int {
			return cmp.Or(cmp.Compare(a.Expr, b.Expr), cmp.Compare(a.Column, b.Column))
		})
	}
}

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// fkKey identifies a foreign key for finding its name: two on the same
// columns reference different tables.
func fkKey(cols []string, table string) string {
	return strings.Join(cols, ",") + " " + table
}

// word returns an unquoted word upper-cased, or "".
func word(tok dialect.Token) string {
	if tok.Kind != dialect.Word {
		return ""
	}
	return strings.ToUpper(tok.Text)
}

// name returns the name a Word or Quoted token stands for.
func name(tok dialect.Token) string {
	if tok.Kind != dialect.Quoted {
		return tok.Text
	}
	q := tok.Text[:1]
	return strings.ReplaceAll(tok.Text[1:len(tok.Text)-1], q+q, q)
}

// closing returns the index of the parenthesis closing toks[open].
func closing(toks []dialect.Token, open int) int {
	depth := 0
	for i := open; i < len(toks); i++ {
		switch toks[i].Text {
		case "(":
			depth++
		case ")":
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return len(toks) - 1
}

// splitTop splits toks at commas outside parentheses.
func splitTop(toks []dialect.Token) [][]dialect.Token {
	var out [][]dialect.Token
	depth, start := 0, 0
	for i, tok := range toks {
		switch {
		case tok.Text == "(":
			depth++
		case tok.Text == ")":
			depth--
		case tok.Text == "," && depth == 0:
			out = append(out, toks[start:i])
			start = i + 1
		}
	}
	return append(out, toks[start:])
}
