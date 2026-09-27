package dialect

import (
	"fmt"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/golden"
)

var splitCases = []struct{ name, src string }{
	{"semicolons in strings, names and comments", "SELECT 'a;b', \"c;d\", `e;f` -- g;h\n/* i;j */ FROM t;SELECT 2"},
	{"doubled quotes", "SELECT 'it''s; x', \"a\"\"b;\";SELECT 2;"},
	{"escape string", `SELECT E'\'; x', e'\\';SELECT 2`},
	{"dollar quoting", "CREATE FUNCTION f() RETURNS int AS $$ SELECT 1; $$ LANGUAGE sql;\nDO $fn$ BEGIN PERFORM '$$;'; END $fn$;"},
	{"nested tags", "SELECT $a$ x $b$ ; $b$ y $a$;SELECT 2"},
	{"unicode tag and names", "SELECT $태그$;$태그$, 제목, a$b, $1;"},
	{"lines after multi-line text", "SELECT 'a\nb', $$\n;\n$$;\n\n  SELECT /* c\n */ 2;"},
	{"empty statements", ";; -- only a comment\n;/* and this */;SELECT 1;;"},
	{"no final semicolon, comment before semicolon", "SELECT 1 /* gone */ -- also gone\n;SELECT 2"},
	{"CRLF", "SELECT 1;\r\nSELECT\r\n  2;\r\n"},
	{"column after Korean", "SELECT '가나'; SELECT 3;"},
	{"numbers and operators", "CHECK (x::int >= 1.5e3 AND y <> 0x1F)"},

	{"unterminated string", "SELECT 1;\nSELECT 'abc;\nSELECT 2;"},
	{"unterminated escape string", `SELECT E'abc\';`},
	{"unterminated name", "SELECT \"abc;"},
	{"unterminated comment", "SELECT 1; /* abc;"},
	{"unterminated dollar quote", "SELECT $x$ abc $y$;"},

	{"marker attaches to the next statement only", "-- maago: allow-drop\nDROP TABLE a;\nDROP TABLE b;"},
	{"one of two drops", "DROP TABLE a;\n  -- maago: allow-drop\n  DROP TABLE b;"},
	{"no space", "--maago:allow-drop\nDROP TABLE a;"},
	{"marker in text is not a marker", "SELECT '\n-- maago: allow-drop\n', $$\n-- maago: allow-drop\n$$;\n/* maago: allow-drop */\nDROP TABLE a;"},
	{"blank line between", "-- maago: allow-drop\n\nDROP TABLE a;"},
	{"comment between", "-- maago: allow-drop\n-- drop the old table\nDROP TABLE a;"},
	{"after a statement on the line", "SELECT 1; -- maago: allow-drop\nDROP TABLE a;"},
	{"missing semicolon before", "SELECT 1\n-- maago: allow-drop\nDROP TABLE a;"},
	{"after a lone semicolon on the line", "SELECT 1\n; -- maago: allow-drop\nDROP TABLE a;"},
	{"comment mentioning a marker", "-- see maago: allow-drop in the docs\nDROP TABLE a;"},
	{"comment before the statement on the next line", "-- maago: allow-drop\n/* gap */ DROP TABLE a;"},
	{"CRLF after marker", "-- maago: allow-drop\r\nDROP TABLE a;"},
	{"after a token, inside an open statement", "SELECT 1 -- maago: allow-drop\nDROP TABLE a;"},
	{"two markers", "-- maago: allow-drop\n-- maago: allow-drop\nDROP TABLE a;"},
	{"end of file", "DROP TABLE a;\n-- maago: allow-drop\n"},
	{"unknown marker", "-- maago: allow-dorp\nDROP TABLE a;"},
	{"unknown marker after a statement", "SELECT 1; -- maago: allow-dorp\nDROP TABLE a;"},
}

func TestSplit(t *testing.T) {
	var b strings.Builder
	for _, c := range splitCases {
		stmts, ds := Split("m.sql", c.src)
		fmt.Fprintf(&b, "== %s\n%q\n", c.name, c.src)
		for _, st := range stmts {
			allow := "-"
			if st.AllowDrop.Line > 0 {
				allow = fmt.Sprintf("%d:%d", st.AllowDrop.Line, st.AllowDrop.Column)
			}
			if got := c.src[st.Pos.Offset:][:len(st.SQL)]; got != st.SQL {
				t.Errorf("%s: Offset %d does not point at %q", c.name, st.Pos.Offset, st.SQL)
			}
			fmt.Fprintf(&b, "stmt %d:%d allow=%s %q\n ", st.Pos.Line, st.Pos.Column, allow, st.SQL)
			for _, tok := range st.Tokens {
				fmt.Fprintf(&b, " %s:%s", tok.Kind, diag.OneLine(tok.Text))
			}
			b.WriteString("\n")
		}
		diag.Write(&b, ds)
	}
	golden.Check(t, "testdata/split.golden", []byte(b.String()))
}

func TestNormalize(t *testing.T) {
	norm := func(src string) string {
		stmts, ds := Split("", src)
		if len(ds) > 0 || len(stmts) != 1 {
			t.Fatalf("Split(%q) = %d statements, %d diagnostics", src, len(stmts), len(ds))
		}
		return Normalize(stmts[0].Tokens)
	}
	same := [][2]string{
		{"check (length(title) <= 200)", "CHECK(LENGTH ( title )<=200 )"},
		{"a IN ('x', 'y')", "a  in\n('x',/* c */'y') -- z"},
	}
	for _, p := range same {
		if a, b := norm(p[0]), norm(p[1]); a != b {
			t.Errorf("%q and %q differ: %q, %q", p[0], p[1], a, b)
		}
	}
	differ := [][2]string{
		{"a = 'x'", "a = 'X'"},
		{`"Title"`, `"title"`},
		{"a = 1", "a = 2"},
	}
	for _, p := range differ {
		if norm(p[0]) == norm(p[1]) {
			t.Errorf("%q and %q are the same", p[0], p[1])
		}
	}
	if got, want := norm("check (a in ('x'))"), "CHECK ( A IN ( 'x' ) )"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// SplitComments reads no markers and returns only real comments.
func TestSplitComments(t *testing.T) {
	src := "-- a\r\nSELECT '-- b', $$ -- c $$ /* -- d */, 1 -- e\n-- maago: nope\nFROM t"
	stmts, comments, ds := SplitComments("q.sql", src)
	var got []string
	for _, c := range comments {
		got = append(got, fmt.Sprintf("%d:%d %q", c.Pos.Line, c.Pos.Column, c.Text))
	}
	want := `1:1 "-- a" 2:41 "-- e" 3:1 "-- maago: nope"`
	if strings.Join(got, " ") != want || len(stmts) != 1 || ds != nil {
		t.Errorf("got %s, %d statements, %v\nwant %s, 1 statement, no errors", strings.Join(got, " "), len(stmts), ds, want)
	}
	for _, tok := range stmts[0].Tokens {
		if tok.Kind == Comment {
			t.Errorf("comment %q among the tokens", tok.Text)
		}
	}
}
