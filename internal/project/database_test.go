package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDatabase(t *testing.T) {
	const pg = "postgres://maago:s3cret@localhost/app"
	const unset = "\x00" // env value meaning the variable is not set
	for _, tc := range []struct {
		name, env, dotenv string // dotenv unset: no .env file
		dialect           Dialect
		url, want         string // want: diagnostics, if any
	}{
		{name: "comments, blank lines, CRLF", env: unset,
			dotenv:  "# db\r\n\r\n  # indented\r\nOTHER=1\r\nMAAGO_DATABASE_URL=sqlite:app.db\r\n",
			dialect: SQLite, url: "sqlite:app.db"},
		{name: "BOM", env: unset, dotenv: "\uFEFFMAAGO_DATABASE_URL=sqlite:app.db", dialect: SQLite, url: "sqlite:app.db"},
		{name: "double quotes", env: unset, dotenv: `MAAGO_DATABASE_URL="` + pg + `"`, dialect: Postgres, url: pg},
		{name: "single quotes", env: unset, dotenv: "MAAGO_DATABASE_URL='sqlite:a b.db'", dialect: SQLite, url: "sqlite:a b.db"},
		{name: "hash in value", env: unset, dotenv: "MAAGO_DATABASE_URL=postgres://u:p#1@h/db", dialect: Postgres, url: "postgres://u:p#1@h/db"},
		{name: "spaces around =", env: unset, dotenv: "MAAGO_DATABASE_URL = postgresql://h/db", dialect: Postgres, url: "postgresql://h/db"},
		{name: "env wins", env: "sqlite:x.db", dotenv: "MAAGO_DATABASE_URL=mysql://h", dialect: SQLite, url: "sqlite:x.db"},
		{name: "env without .env", env: pg, dotenv: unset, dialect: Postgres, url: pg},

		{name: "unclosed quote", env: unset, dotenv: "A=1\nMAAGO_DATABASE_URL=\"" + pg,
			want: ".env:2: unclosed quote in the value of MAAGO_DATABASE_URL\n  fix: close the quote\n"},
		{name: "lone quote", env: unset, dotenv: `A="`,
			want: "MAAGO_DATABASE_URL is not set\n  fix: add " + example + " to .env\n" +
				".env:1: unclosed quote in the value of A\n  fix: close the quote\n"},
		{name: "comment after quotes", env: unset, dotenv: `MAAGO_DATABASE_URL="sqlite:app.db" # dev`,
			want: ".env:1: text after the closing quote in the value of MAAGO_DATABASE_URL\n  fix: remove the trailing comment\n"},
		{name: "trailing comment", env: unset, dotenv: "MAAGO_DATABASE_URL=sqlite:app.db # dev",
			want: ".env:1: space in the unquoted value of MAAGO_DATABASE_URL\n  fix: quote the value or remove the trailing comment\n"},
		{name: "export, no cascade", env: unset, dotenv: "export MAAGO_DATABASE_URL=" + pg,
			want: ".env:1: .env lines cannot start with export\n  fix: remove \"export\"\n"},
		{name: "no =", env: unset, dotenv: "\nMAAGO_DATABASE_URL",
			want: ".env:2: line without =\n  fix: write KEY=VALUE\n"},
		{name: "bad key", env: unset, dotenv: "1A=x\nA-B=y\n=z",
			want: ".env:1: invalid key\n  fix: use letters, digits and _ in keys, not starting with a digit\n" +
				".env:2: invalid key\n  fix: use letters, digits and _ in keys, not starting with a digit\n" +
				".env:3: invalid key\n  fix: use letters, digits and _ in keys, not starting with a digit\n"},
		{name: "pasted URL without key", env: unset, dotenv: "A=1\n" + pg + "?sslmode=require",
			want: ".env:2: invalid key\n  fix: use letters, digits and _ in keys, not starting with a digit\n"},
		{name: "same key twice", env: unset, dotenv: "A=1\nMAAGO_DATABASE_URL=sqlite:a.db\nA=2\nMAAGO_DATABASE_URL=" + pg,
			want: ".env:3: A is set twice; first on line 1\n  fix: remove one of the lines\n" +
				".env:4: MAAGO_DATABASE_URL is set twice; first on line 2\n  fix: remove one of the lines\n"},
		{name: "line errors even when env wins", env: pg, dotenv: "export A=1",
			want: ".env:1: .env lines cannot start with export\n  fix: remove \"export\"\n"},

		{name: "unknown scheme", env: unset, dotenv: "A=1\nMAAGO_DATABASE_URL=mysql://maago:s3cret@h/db",
			want: ".env:2: MAAGO_DATABASE_URL starts with \"mysql:\", not sqlite:, postgres:// or postgresql://\n  fix: " + formats + "\n"},
		{name: "no scheme", env: unset, dotenv: "MAAGO_DATABASE_URL=s3cret",
			want: ".env:1: MAAGO_DATABASE_URL has no scheme\n  fix: write sqlite:FILE or postgres://HOST:PORT/DB\n"},
		{name: "not a scheme before :", env: unset, dotenv: "MAAGO_DATABASE_URL=s3cret@h:5432/db",
			want: ".env:1: MAAGO_DATABASE_URL has no scheme\n  fix: " + formats + "\n"},
		{name: "sqlite://", env: unset, dotenv: "MAAGO_DATABASE_URL=sqlite://app.db",
			want: ".env:1: MAAGO_DATABASE_URL starts with \"sqlite://\"\n  fix: write sqlite:FILE, as in sqlite:app.db\n"},
		{name: "sqlite without path", env: unset, dotenv: "MAAGO_DATABASE_URL=sqlite:",
			want: ".env:1: MAAGO_DATABASE_URL has no file name after sqlite:\n  fix: " + formats + "\n"},
		{name: "postgres without //", env: unset, dotenv: "MAAGO_DATABASE_URL=postgres:h/db",
			want: ".env:1: MAAGO_DATABASE_URL starts with \"postgres:\", not sqlite:, postgres:// or postgresql://\n  fix: " + formats + "\n"},
		{name: "upper-case scheme", env: unset, dotenv: "MAAGO_DATABASE_URL=POSTGRES://maago:s3cret@h/db",
			want: ".env:1: MAAGO_DATABASE_URL starts with \"POSTGRES:\", not sqlite:, postgres:// or postgresql://\n  fix: " + formats + "\n"},
		{name: "env unknown scheme", env: "mysql://maago:s3cret@h/db", dotenv: "MAAGO_DATABASE_URL=" + pg,
			want: "environment variable MAAGO_DATABASE_URL starts with \"mysql:\", not sqlite:, postgres:// or postgresql://\n  fix: " + formats + "\n"},
		{name: "empty in .env", env: unset, dotenv: "MAAGO_DATABASE_URL=",
			want: ".env:1: MAAGO_DATABASE_URL is empty\n  fix: " + formats + "\n"},
		{name: "empty quoted in .env", env: unset, dotenv: `MAAGO_DATABASE_URL=""`,
			want: ".env:1: MAAGO_DATABASE_URL is empty\n  fix: " + formats + "\n"},
		{name: "empty env", env: "", dotenv: "MAAGO_DATABASE_URL=" + pg,
			want: "environment variable MAAGO_DATABASE_URL is empty\n  fix: set it to sqlite:FILE or postgres://HOST:PORT/DB, or unset it to use .env\n"},
		{name: "not set", env: unset, dotenv: "A=1",
			want: "MAAGO_DATABASE_URL is not set\n  fix: add MAAGO_DATABASE_URL=sqlite:app.db or MAAGO_DATABASE_URL=postgres://localhost:5432/myapp to .env\n"},
		{name: "not set, other line broken", env: unset, dotenv: "DEBUG=true # dev",
			want: "MAAGO_DATABASE_URL is not set\n  fix: add MAAGO_DATABASE_URL=sqlite:app.db or MAAGO_DATABASE_URL=postgres://localhost:5432/myapp to .env\n" +
				".env:1: space in the unquoted value of DEBUG\n  fix: quote the value or remove the trailing comment\n"},
		{name: "not set, URL line broken", env: unset, dotenv: "MAAGO_DATABASE_URL='sqlite:app.db",
			want: ".env:1: unclosed quote in the value of MAAGO_DATABASE_URL\n  fix: close the quote\n"},
		{name: "not set, no .env", env: unset, dotenv: unset,
			want: "MAAGO_DATABASE_URL is not set\n  fix: add " + example + " to .env\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.dotenv != unset {
				write(t, filepath.Join(root, ".env"), tc.dotenv)
			}
			lookup := func(key string) (string, bool) {
				if key != "MAAGO_DATABASE_URL" {
					t.Errorf("looked up %s", key)
				}
				return tc.env, tc.env != unset
			}

			d, url, ds := Database(root, lookup)

			out := text(ds)
			if n := strings.Count(tc.want, "\n  fix: "); len(ds) != n { // text drops duplicates
				t.Errorf("got %d diagnostics, want %d", len(ds), n)
			}
			if d != tc.dialect || url != tc.url || out != tc.want {
				t.Errorf("got %q, %q\n%s\nwant %q, %q\n%s", d, url, out, tc.dialect, tc.url, tc.want)
			}
			if strings.Contains(out, "s3cret") {
				t.Errorf("diagnostics show the password:\n%s", out)
			}
		})
	}
}

func TestDatabaseUnreadable(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".env"), 0o755); err != nil {
		t.Fatal(err)
	}
	d, url, ds := Database(root, func(string) (string, bool) { return "sqlite:a.db", true })
	if out := text(ds); d != "" || url != "" || len(ds) != 1 || !strings.HasPrefix(out, "cannot read .env: ") {
		t.Errorf("got %q, %q, want one cannot-read error:\n%s", d, url, out)
	}
}
