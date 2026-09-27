package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/golden"
)

// tempProject writes files, by slash path, under a new root.
func tempProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for p, src := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func text(ds []diag.Diagnostic) string {
	var b strings.Builder
	diag.Write(&b, ds)
	return b.String()
}

func TestReadFilesNone(t *testing.T) {
	files, ds := ReadFiles(t.TempDir())
	if len(files) > 0 || len(ds) > 0 {
		t.Errorf("got %v\n%s", files, text(ds))
	}
}

func TestReadFilesOrder(t *testing.T) {
	root := tempProject(t, map[string]string{
		"migrations/0002_add_body.sql": "ALTER TABLE t ADD COLUMN b TEXT;",
		"migrations/0001_initial.sql":  "CREATE TABLE t (a TEXT);\nCREATE INDEX i ON t (a);",
		"migrations/README.md":         "not a migration",
		"migrations/old/0009_x.sql":    "ignored: a subdirectory",
	})
	files, ds := ReadFiles(root)
	var got []string
	for _, f := range files {
		got = append(got, f.Path)
	}
	if len(ds) > 0 || strings.Join(got, " ") != "migrations/0001_initial.sql migrations/0002_add_body.sql" || len(files[0].Stmts) != 2 {
		t.Errorf("got %v\n%s", got, text(ds))
	}
}

// Names and numbers, each case its own project.
func TestReadFilesNames(t *testing.T) {
	var out strings.Builder
	for _, tc := range []struct {
		name  string
		files []string
	}{
		{"gap", []string{"0001_initial", "0002_a", "0004_c"}},
		{"not from 1", []string{"0002_a"}},
		// Two branches that each made 0003.
		{"same number", []string{"0001_initial", "0002_a", "0003_add_x", "0003_add_y"}},
		// Independent: the repeat does not hide the gap.
		{"same number after a gap", []string{"0001_initial", "0003_a", "0003_b"}},
		{"bad names", []string{"0001_initial", "2_short", "0002_Upper", "0002-dash"}},
	} {
		files := map[string]string{}
		for _, f := range tc.files {
			files["migrations/"+f+".sql"] = "SELECT 1;"
		}
		_, ds := ReadFiles(tempProject(t, files))
		out.WriteString("-- " + tc.name + "\n" + text(ds))
	}
	out.WriteString("-- unterminated string\n")
	_, ds := ReadFiles(tempProject(t, map[string]string{"migrations/0001_initial.sql": "SELECT 1;\nSELECT 'x;\n"}))
	out.WriteString(text(ds))
	golden.Check(t, "testdata/names.golden", []byte(out.String()))
}

// Each statement a migration must not hold gives one diagnostic, and
// what looks like one but is not gives none.
func TestCheckFiles(t *testing.T) {
	const refused = `BEGIN;
begin transaction;
START TRANSACTION;
COMMIT;
COMMIT PREPARED 'x';
END;
ROLLBACK;
ABORT;
SAVEPOINT s;
RELEASE s;
PREPARE TRANSACTION 'x';
SET search_path TO public;
SET LOCAL statement_timeout = 0;
RESET ALL;
PRAGMA foreign_keys = OFF;
SELECT set_config('search_path', 'public', false);
DO $$ BEGIN EXECUTE 'CREATE TABLE public.x (a int)'; END $$;
ATTACH DATABASE 'x.db' AS x;
DETACH DATABASE x;
CREATE INDEX CONCURRENTLY i ON t (a);
DROP INDEX CONCURRENTLY i;
REINDEX TABLE CONCURRENTLY t;
CREATE EXTENSION citext;
CREATE SCHEMA s;
CREATE TEMP TABLE x (a int);
CREATE GLOBAL TEMPORARY TABLE x (a int);
CREATE VIEW v AS SELECT 1;
CREATE OR REPLACE MATERIALIZED VIEW v AS SELECT 1;
CREATE TABLE public.x (a int);
CREATE TABLE IF NOT EXISTS "temp".x (a int);
CREATE UNIQUE INDEX s.i ON t (a);
CREATE INDEX i ON ONLY public.t (a);
ALTER TABLE public.t ADD COLUMN b int;
INSERT INTO public.t VALUES (1);
CREATE TRIGGER tr AFTER INSERT ON t BEGIN
  UPDATE t SET a = 1;
END;
CREATE TRIGGER tr3 AFTER DELETE ON t BEGIN
  UPDATE t SET a = 1;
  DELETE FROM t WHERE a = 2;
END;
CREATE OR REPLACE TRIGGER tr2 AFTER INSERT ON t FOR EACH ROW EXECUTE FUNCTION f();
`
	const passed = `CREATE TABLE t (a int CHECK (lower(a) = pg_catalog.lower(a)), "concurrently" int);
CREATE INDEX ON t (a);
CREATE INDEX i ON t ("concurrently");
INSERT INTO t (a) SELECT t.a FROM t;
ALTER TABLE t DROP CONSTRAINT c;
ALTER TABLE t ALTER COLUMN a DROP DEFAULT, ALTER COLUMN a DROP NOT NULL;
ALTER TABLE t ADD CONSTRAINT c CHECK (a > 0);
-- maago: allow-drop
DROP TABLE old;
-- maago: allow-drop
ALTER TABLE t DROP COLUMN b;
-- maago: allow-drop
ALTER TABLE t ADD COLUMN c int, DROP d;
CREATE TABLE "maago_new_t" (a int);
INSERT INTO "maago_new_t" ("a") SELECT "a" FROM "t";
DROP TABLE "t";
ALTER TABLE "maago_new_t" RENAME TO "t";
`
	const drops = `DROP TABLE old;
ALTER TABLE t DROP COLUMN b;
ALTER TABLE t DROP b;
alter table t add column c int, drop if exists d;
-- maago: allow-drop
CREATE INDEX i ON t (a);
DROP TABLE x;
-- maago: allow-drop
DROP TABLE y;
DROP TABLE z;
CREATE TABLE u2 (a int);
INSERT INTO u2 (a) SELECT a FROM u WHERE a > 0;
DROP TABLE u;
ALTER TABLE u2 RENAME TO u;
CREATE TABLE v2 (a int);
INSERT INTO v2 (a) SELECT a FROM v;
DROP TABLE v;
`
	root := tempProject(t, map[string]string{
		"migrations/0001_refused.sql": refused,
		"migrations/0002_passed.sql":  passed,
		"migrations/0003_drops.sql":   drops,
	})
	files, ds := ReadFiles(root)
	if len(ds) > 0 {
		t.Fatal(text(ds))
	}
	golden.Check(t, "testdata/check.golden", []byte(text(CheckFiles(files))))
}
