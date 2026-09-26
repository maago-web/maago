package sqlite

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/schema"
)

// Read reads the schema of a SQLite database: the catalog, and for what
// the catalog leaves out (CHECK, AUTOINCREMENT, STRICT, constraint names)
// the CREATE TABLE text (D-92). What the schema cannot hold, such as a
// partial index, is reported rather than skipped (PHILOSOPHY 7), since a
// comparison would not see it. Every table is read, so that all such
// reports come at once; a caller compares nothing if there are any.
func Read(ctx context.Context, q dialect.Querier) (*schema.Schema, []diag.Diagnostic) {
	r := &reader{dialect.Reader{Ctx: ctx, Q: q}}
	s := &schema.Schema{}
	shadow := map[string]bool{} // tables a virtual table keeps its data in
	var texts []string          // CREATE TABLE text of each table
	err := r.Rows("SELECT name FROM pragma_table_list WHERE schema = 'main' AND type = 'shadow'", func(v []any) {
		shadow[dialect.Str(v[0])] = true
	})
	if err != nil {
		return nil, dialect.Bug(err)
	}
	err = r.Rows("SELECT type, name, sql FROM sqlite_schema ORDER BY name", func(v []any) {
		kind, name, text := dialect.Str(v[0]), dialect.Str(v[1]), dialect.Str(v[2])
		switch {
		case kind == "view" || kind == "trigger":
			r.Fail(fmt.Sprintf("%s %q is not supported", kind, name), "remove it from the migrations")
		case kind != "table" || shadow[name] || strings.HasPrefix(name, "sqlite_") || name == "maago_migrations":
		case strings.HasPrefix(strings.ToUpper(text), "CREATE VIRTUAL"):
			r.Fail(fmt.Sprintf("virtual table %q is not supported", name), "remove it from the migrations")
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
		return nil, dialect.Bug(err)
	}
	r.resolve(s)
	return s, r.Diags
}

type reader struct{ dialect.Reader }

func (r *reader) table(t *schema.Table, text string) error {
	fkNames, collated := r.parse(t, text)
	pk := map[int64]string{} // by position in the key
	err := r.Rows("SELECT name, type, \"notnull\", dflt_value, pk, hidden FROM pragma_table_xinfo(?) ORDER BY cid", func(v []any) {
		c := schema.Column{Name: dialect.Str(v[0]), Type: strings.ToUpper(dialect.Str(v[1])), NotNull: dialect.Num(v[2]) == 1}
		if v[3] != nil {
			c.Default = dialect.NormalizeSQL(dialect.Str(v[3]))
		}
		if dialect.Num(v[5]) != 0 {
			r.Unsupported(t.Name, fmt.Sprintf("generated column %q", c.Name))
		}
		t.Columns = append(t.Columns, c)
		if n := dialect.Num(v[4]); n > 0 {
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
	err = r.Rows("SELECT name, \"unique\", origin, partial FROM pragma_index_list(?) ORDER BY name", func(v []any) {
		indexes = append(indexes, index{dialect.Str(v[0]), dialect.Num(v[1]) == 1, dialect.Num(v[3]) == 1, dialect.Str(v[2])})
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
			r.Unsupported(t.Name, fmt.Sprintf("partial index %q", ix.name))
			continue
		}
		out := schema.Index{Name: ix.name, Unique: ix.unique, Constraint: ix.origin == "u"}
		ok := true
		err := r.Rows("SELECT cid, name, \"desc\", coll FROM pragma_index_xinfo(?) WHERE key = 1 ORDER BY seqno", func(v []any) {
			col := dialect.Str(v[1])
			switch {
			case dialect.Num(v[0]) == -2:
				r.Unsupported(t.Name, what+" on an expression")
				ok = false
			case dialect.Num(v[2]) == 1:
				r.Unsupported(t.Name, "descending "+what)
				ok = false
			case !strings.EqualFold(dialect.Str(v[3]), "BINARY") && !collated[col]:
				r.Unsupported(t.Name, what+" with COLLATE")
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
	err = r.Rows("SELECT id, \"table\", \"from\", \"to\", on_update, on_delete FROM pragma_foreign_key_list(?) ORDER BY id, seq", func(v []any) {
		id := dialect.Num(v[0])
		fk := byID[id]
		if fk == nil {
			fk = &schema.FK{RefTable: dialect.Str(v[1]), OnUpdate: dialect.Str(v[4]), OnDelete: dialect.Str(v[5])}
			byID[id] = fk
			ids = append(ids, id)
		}
		fk.Columns = append(fk.Columns, dialect.Str(v[2]))
		if v[3] != nil {
			fk.RefColumns = append(fk.RefColumns, dialect.Str(v[3]))
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

// parse reads from the CREATE TABLE text what the catalog leaves out. It
// returns the names of foreign keys by their columns, and the columns
// declared with COLLATE, whose indexes are then not reported again.
func (r *reader) parse(t *schema.Table, text string) (fkNames map[string]string, collated map[string]bool) {
	fkNames, collated = map[string]string{}, map[string]bool{}
	stmts, _ := dialect.Split("", text) // stored text is well formed; markers were reported with the file
	if len(stmts) != 1 {
		r.Fail(fmt.Sprintf("cannot read the definition of table %q", t.Name), "report this as a maago bug")
		return
	}
	// SQLite stores even CREATE TABLE … AS SELECT as a list of columns.
	toks := stmts[0].Tokens
	open := slices.IndexFunc(toks, func(tok dialect.Token) bool { return tok.Text == "(" })
	if open < 0 {
		r.Fail(fmt.Sprintf("cannot read the definition of table %q", t.Name), "report this as a maago bug")
		return
	}
	end := dialect.Closing(toks, open)
	for _, tok := range toks[end+1:] {
		switch word(tok) {
		case "STRICT":
			t.Strict = true
		case "WITHOUT":
			r.Unsupported(t.Name, "WITHOUT ROWID")
		}
	}
	for _, def := range splitTop(toks[open+1 : end]) {
		col := ""
		switch word(def[0]) {
		case "CONSTRAINT", "PRIMARY", "UNIQUE", "CHECK", "FOREIGN":
		default:
			if def[0].Kind != dialect.Word && def[0].Kind != dialect.Quoted { // [name] or 'name'
				r.Fail(fmt.Sprintf("table %q has a column name quoted with %s, which maago does not support", t.Name, def[0].Text), `quote the name with "…" instead`)
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
				end := dialect.Closing(def, i+1)
				t.Checks = append(t.Checks, schema.Check{Name: constraint, Column: col, Expr: dialect.Normalize(def[i+2 : end])})
				i = end
			}
		case "FOREIGN": // KEY (cols) … REFERENCES
			fkName = constraint
			if i+2 < len(def) && def[i+2].Text == "(" {
				end := dialect.Closing(def, i+2)
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
				end := dialect.Closing(def, i+2)
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
			r.Unsupported(t.Name, what)
		}
		if def[i].Text == "(" {
			i = dialect.Closing(def, i) // only depth 0 counts
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
				r.Fail(fmt.Sprintf("foreign key of %q references unknown table %q", t.Name, fk.RefTable), "create that table in the migrations, or fix the name after REFERENCES")
			case len(fk.RefColumns) == 0:
				fk.RefColumns = ref.PK
			}
		}
	}
	s.Sort()
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
