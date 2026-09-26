package postgres

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/dialect"
	"github.com/maago-web/maago/internal/schema"
)

// notExtension keeps out what an extension created: the extension itself
// is reported once instead. %s is the catalog of the object.
const notExtension = `NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.classid = '%s'::regclass AND d.objid = o.oid AND d.deptype = 'e')`

// Read reads the schema of current_schema(), the practice schema, from the
// catalog. What the schema cannot hold is reported rather than skipped
// (PHILOSOPHY 7), in the same words as sqlite.Read. Every table is read,
// so that all such reports come at once; a caller compares nothing if
// there are any.
func Read(ctx context.Context, q dialect.Querier) (*schema.Schema, []diag.Diagnostic) {
	r := &reader{Reader: dialect.Reader{Ctx: ctx, Q: q}, columns: map[int64]map[int64]string{}}
	s := &schema.Schema{}
	if err := r.read(s); err != nil {
		return nil, dialect.Bug(err)
	}
	s.Sort()
	return s, r.Diags
}

type reader struct {
	dialect.Reader
	columns map[int64]map[int64]string // column names by attnum, by table oid
}

type readTable struct {
	oid      int64
	t        *schema.Table
	identity map[string]string // IDENTITY kind by column
}

func (r *reader) read(s *schema.Schema) error {
	notSupported := func(kind string) func([]any) {
		return func(v []any) {
			r.Fail(fmt.Sprintf("%s %q is not supported", kind, dialect.Str(v[0])), "remove it from the migrations")
		}
	}
	if err := r.Rows(`SELECT e.extname FROM pg_extension e JOIN pg_namespace n ON n.oid = e.extnamespace
		WHERE n.nspname = current_schema()`, notSupported("extension")); err != nil {
		return err
	}

	var tables []*readTable
	err := r.Rows(`SELECT o.oid, o.relname, o.relkind, o.relpersistence, o.relrowsecurity, o.relispartition,
			EXISTS (SELECT 1 FROM pg_inherits i WHERE i.inhrelid = o.oid),
			EXISTS (SELECT 1 FROM pg_depend d JOIN pg_class t ON t.oid = d.refobjid
				WHERE d.classid = 'pg_class'::regclass AND d.objid = o.oid AND d.deptype IN ('i', 'a') AND t.relnamespace = o.relnamespace)
		FROM pg_class o JOIN pg_namespace n ON n.oid = o.relnamespace
		WHERE n.nspname = current_schema() AND o.relkind IN ('r', 'p', 'v', 'm', 'f', 'S')
			AND `+fmt.Sprintf(notExtension, "pg_class"), func(v []any) {
		name, kind := dialect.Str(v[1]), dialect.Str(v[2])
		switch {
		case v[5] == true || name == "maago_migrations": // a partition is reported with its table
		case kind == "r":
			t := &readTable{oid: dialect.Num(v[0]), t: &schema.Table{Name: name}, identity: map[string]string{}}
			tables = append(tables, t)
			s.Tables = append(s.Tables, t.t)
			if dialect.Str(v[3]) == "u" {
				r.Unsupported(name, "UNLOGGED storage")
			}
			if v[4] == true {
				r.Unsupported(name, "row level security")
			}
			if v[6] == true {
				r.Unsupported(name, "inheritance")
			}
		case kind == "S" && v[7] == true: // IDENTITY or serial
		default:
			what := map[string]string{"p": "partitioned table", "v": "view", "m": "materialized view", "f": "foreign table", "S": "sequence"}[kind]
			r.Fail(fmt.Sprintf("%s %q is not supported", what, name), "remove it from the migrations")
		}
	})
	if err != nil {
		return err
	}
	for _, q := range []struct{ kind, query string }{
		{"function", `SELECT o.proname FROM pg_proc o JOIN pg_namespace n ON n.oid = o.pronamespace
			WHERE n.nspname = current_schema() AND ` + fmt.Sprintf(notExtension, "pg_proc") + `
				AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.classid = 'pg_proc'::regclass AND d.objid = o.oid AND d.deptype = 'i')`}, // a range type's constructors come with it
		{"type", `SELECT o.typname FROM pg_type o JOIN pg_namespace n ON n.oid = o.typnamespace
			WHERE n.nspname = current_schema()
				AND (o.typtype IN ('e', 'd', 'r') OR o.typtype = 'c' AND (SELECT relkind FROM pg_class WHERE oid = o.typrelid) = 'c')
				AND ` + fmt.Sprintf(notExtension, "pg_type")},
		{"rule", `SELECT o.rulename FROM pg_rewrite o JOIN pg_class c ON c.oid = o.ev_class JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = current_schema() AND o.rulename <> '_RETURN' AND ` + fmt.Sprintf(notExtension, "pg_rewrite")},
		{"trigger", `SELECT o.tgname FROM pg_trigger o JOIN pg_class c ON c.oid = o.tgrelid JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = current_schema() AND NOT o.tgisinternal AND ` + fmt.Sprintf(notExtension, "pg_trigger")},
	} {
		if err := r.Rows(q.query, notSupported(q.kind)); err != nil {
			return err
		}
	}

	for _, t := range tables {
		if err := r.readColumns(t); err != nil {
			return err
		}
	}
	for _, t := range tables {
		backing, err := r.readConstraints(t)
		if err == nil {
			err = r.readIndexes(t, backing)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) readColumns(t *readTable) error {
	names := map[int64]string{}
	r.columns[t.oid] = names
	return r.Rows(`SELECT a.attnum, a.attname, upper(format_type(a.atttypid, a.atttypmod)), a.attnotnull,
			pg_get_expr(d.adbin, d.adrelid), a.attidentity, a.attgenerated, a.attcollation <> y.typcollation
		FROM pg_attribute a JOIN pg_type y ON y.oid = a.atttypid
			LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		WHERE a.attrelid = $1 AND a.attnum > 0 AND NOT a.attisdropped ORDER BY a.attnum`, func(v []any) {
		c := schema.Column{Name: dialect.Str(v[1]), Type: dialect.Str(v[2]), NotNull: v[3] == true}
		names[dialect.Num(v[0])] = c.Name
		switch {
		case dialect.Str(v[6]) != "":
			r.Unsupported(t.t.Name, fmt.Sprintf("generated column %q", c.Name))
		case v[4] != nil:
			c.Default = dialect.NormalizeSQL(dialect.Str(v[4]))
		}
		if id := dialect.Str(v[5]); id != "" {
			t.identity[c.Name] = map[string]string{"d": "BY DEFAULT", "a": "ALWAYS"}[id]
		}
		if v[7] == true {
			r.Unsupported(t.t.Name, fmt.Sprintf("COLLATE on column %q", c.Name))
		}
		t.t.Columns = append(t.t.Columns, c)
	}, t.oid)
}

// names turns "1,3" into the names of those columns of table oid.
func (r *reader) names(oid int64, nums string) []string {
	var out []string
	for n := range strings.SplitSeq(nums, ",") {
		if n == "" {
			continue
		}
		i, _ := strconv.ParseInt(n, 10, 64)
		out = append(out, r.columns[oid][i])
	}
	return out
}

var actions = map[string]string{"a": "NO ACTION", "r": "RESTRICT", "c": "CASCADE", "n": "SET NULL", "d": "SET DEFAULT"}

// readConstraints reads keys, uniques, foreign keys and checks, and
// returns the indexes backing them, which readIndexes checks but does not
// list again.
func (r *reader) readConstraints(t *readTable) (map[int64]string, error) {
	backing := map[int64]string{}
	name := t.t.Name
	err := r.Rows(`SELECT c.conname, c.contype, array_to_string(c.conkey, ','),
			c.confrelid, rn.nspname = current_schema(), rc.relname, array_to_string(c.confkey, ','),
			c.confdeltype, c.confupdtype, c.confmatchtype, array_to_string(c.confdelsetcols, ','),
			pg_get_constraintdef(c.oid), c.connoinherit, c.condeferrable, c.convalidated, c.conindid
		FROM pg_constraint c LEFT JOIN pg_class rc ON rc.oid = c.confrelid
			LEFT JOIN pg_namespace rn ON rn.oid = rc.relnamespace
		WHERE c.conrelid = $1 ORDER BY c.conname`, func(v []any) {
		con, kind, cols := dialect.Str(v[0]), dialect.Str(v[1]), r.names(t.oid, dialect.Str(v[2]))
		if kind == "t" {
			return // a constraint trigger, reported as a trigger
		}
		if v[12] == true && (kind == "c" || kind == "n") { // keys and foreign keys are never inherited
			r.Unsupported(name, fmt.Sprintf("NO INHERIT on constraint %q", con))
		}
		if v[13] == true {
			r.Unsupported(name, fmt.Sprintf("DEFERRABLE constraint %q", con))
		}
		if v[14] == false {
			r.Unsupported(name, fmt.Sprintf("NOT VALID constraint %q", con))
		}
		switch kind {
		case "p":
			t.t.PK = cols
			backing[dialect.Num(v[15])] = "primary key"
		case "u":
			t.t.Indexes = append(t.t.Indexes, schema.Index{Name: con, Columns: cols, Unique: true, Constraint: true})
			backing[dialect.Num(v[15])] = fmt.Sprintf("unique constraint %q", con)
		case "f":
			fk := schema.FK{Name: con, Columns: cols, RefTable: dialect.Str(v[5]),
				RefColumns: r.names(dialect.Num(v[3]), dialect.Str(v[6])),
				OnDelete:   actions[dialect.Str(v[7])], OnUpdate: actions[dialect.Str(v[8])]}
			switch {
			case v[4] != true:
				r.Fail(fmt.Sprintf("foreign key of %q references %q outside the practice schema", name, fk.RefTable), "reference a table the migrations create, without a schema name")
			case dialect.Str(v[9]) != "s":
				r.Unsupported(name, fmt.Sprintf("MATCH FULL on foreign key %q", con))
			case dialect.Str(v[10]) != "":
				r.Unsupported(name, fmt.Sprintf("SET NULL or SET DEFAULT with columns on foreign key %q", con))
			default:
				t.t.FKs = append(t.t.FKs, fk)
			}
		case "c":
			c := schema.Check{Name: con, Expr: checkExpr(dialect.Str(v[11]))}
			if len(cols) == 1 {
				c.Column = cols[0] // PostgreSQL cannot tell a column check from a table one
			}
			t.t.Checks = append(t.t.Checks, c)
		case "x":
			r.Unsupported(name, fmt.Sprintf("exclusion constraint %q", con))
			backing[dialect.Num(v[15])] = "" // reported: its index is not checked again
		}
	}, t.oid)
	for col, kind := range t.identity {
		if slices.Contains(t.t.PK, col) {
			t.t.PKAuto = kind
		} else {
			r.Unsupported(name, fmt.Sprintf("IDENTITY on column %q outside the primary key", col))
		}
	}
	return backing, err
}

// checkExpr returns what is inside CHECK ( … ) of a constraint definition,
// normalized; words after it, such as NOT VALID, are left out.
func checkExpr(def string) string {
	stmts, _ := dialect.Split("", def)
	if len(stmts) != 1 {
		return def
	}
	toks := stmts[0].Tokens
	i := slices.IndexFunc(toks, func(tok dialect.Token) bool { return tok.Text == "(" })
	if i < 0 {
		return def
	}
	return dialect.Normalize(toks[i+1 : dialect.Closing(toks, i)])
}

func (r *reader) readIndexes(t *readTable, backing map[int64]string) error {
	name := t.t.Name
	return r.Rows(`SELECT i.relname, x.indexrelid, x.indisunique, x.indpred IS NOT NULL, x.indexprs IS NOT NULL,
			am.amname, x.indnatts > x.indnkeyatts, x.indnullsnotdistinct,
			(SELECT string_agg(concat_ws(':', x.indkey[s], x.indoption[s],
					(x.indcollation[s] <> COALESCE(a.attcollation, 0))::int, (NOT oc.opcdefault)::int), ',' ORDER BY s)
				FROM generate_subscripts(x.indkey, 1) s
					LEFT JOIN pg_attribute a ON a.attrelid = x.indrelid AND a.attnum = x.indkey[s]
					LEFT JOIN pg_opclass oc ON oc.oid = x.indclass[s]
				WHERE s < x.indnkeyatts) -- indoption covers key columns only
		FROM pg_index x JOIN pg_class i ON i.oid = x.indexrelid JOIN pg_am am ON am.oid = i.relam
		WHERE x.indrelid = $1 ORDER BY i.relname`, func(v []any) {
		what, backs := backing[dialect.Num(v[1])]
		if backs && what == "" {
			return
		}
		if !backs {
			what = fmt.Sprintf("index %q", dialect.Str(v[0]))
		}
		ok := true
		bad := func(s string) { // one report per index: dropping it fixes all
			if ok {
				r.Unsupported(name, s)
			}
			ok = false
		}
		switch {
		case v[3] == true:
			bad("partial " + what)
		case v[4] == true:
			bad(what + " on an expression")
		case dialect.Str(v[5]) != "btree":
			bad(what + " using " + dialect.Str(v[5]))
		}
		if v[6] == true {
			bad(what + " with INCLUDE")
		}
		if v[7] == true {
			bad(what + " with NULLS NOT DISTINCT")
		}
		var cols []string
		for key := range strings.SplitSeq(dialect.Str(v[8]), ",") {
			f := strings.Split(key, ":")
			if len(f) != 4 {
				continue
			}
			num, _ := strconv.ParseInt(f[0], 10, 64)
			opt, _ := strconv.Atoi(f[1])
			col := r.columns[t.oid][num]
			cols = append(cols, col)
			switch {
			case num == 0: // an expression, reported above
			case opt&1 != 0:
				bad("descending " + what)
			case opt&2 != 0:
				bad(what + " with NULLS FIRST")
			case f[2] == "1": // a column's own COLLATE is reported with the column
				bad(what + " with COLLATE")
			case f[3] == "1":
				bad(what + " with an operator class that is not the default")
			}
		}
		if ok && !backs {
			t.t.Indexes = append(t.t.Indexes, schema.Index{Name: dialect.Str(v[0]), Columns: cols, Unique: v[2] == true})
		}
	}, t.oid)
}
