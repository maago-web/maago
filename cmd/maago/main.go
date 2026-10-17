// Command maago is the Maago CLI.
//
// Exit codes: 0 success or help, 1 checks failed, 2 usage error.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/maago-web/maago/internal/build"
	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/pipeline"
	"github.com/maago-web/maago/internal/practicedb"
	"github.com/maago-web/maago/internal/project"
	"github.com/maago-web/maago/internal/scaffold"

	// The practice database opens drivers by name (D-89, D-90).
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"modernc.org/sqlite"
)

func init() {
	practicedb.ErrorPosition = func(err error) int {
		var pe *pgconn.PgError
		if errors.As(err, &pe) {
			return int(pe.Position)
		}
		return 0
	}
	practicedb.DescribeQuery = describe
}

// describe prepares query without running it. For PostgreSQL the
// statement is the unnamed one, which the server does not keep.
func describe(ctx context.Context, conn *sql.Conn, query string) (desc practicedb.Description, err error) {
	err = conn.Raw(func(dc any) error {
		switch c := dc.(type) {
		case *stdlib.Conn:
			desc, err = describePostgres(ctx, c.Conn().PgConn(), query)
			return err
		case interface {
			ColumnInfo(string) ([]sqlite.ColumnInfo, error)
		}:
			info, err := c.ColumnInfo(query)
			for _, ci := range info {
				desc.Columns = append(desc.Columns, practicedb.Column{Name: ci.Name, Table: ci.TableName, Origin: ci.OriginName})
			}
			return err
		}
		return fmt.Errorf("cannot describe queries with driver %T", dc)
	})
	return desc, err
}

func describePostgres(ctx context.Context, pg *pgconn.PgConn, query string) (practicedb.Description, error) {
	sd, err := pg.Prepare(ctx, "", query, nil)
	if err != nil {
		return practicedb.Description{}, err
	}
	desc := practicedb.Description{Params: sd.ParamOIDs}
	var tables []string
	for _, f := range sd.Fields {
		desc.Columns = append(desc.Columns, practicedb.Column{Name: f.Name, Type: f.DataTypeOID})
		if f.TableOID != 0 {
			tables = append(tables, strconv.FormatUint(uint64(f.TableOID), 10))
		}
	}
	if len(tables) == 0 {
		return desc, nil
	}
	// The sources' names, in one round trip; only tables of the practice
	// schema count.
	rr := pg.ExecParams(ctx, `SELECT a.attrelid, a.attnum, c.relname, a.attname
		FROM pg_attribute a JOIN pg_class c ON c.oid = a.attrelid
		WHERE a.attrelid = ANY($1::oid[]) AND a.attnum > 0 AND c.relnamespace = current_schema()::regnamespace`,
		[][]byte{[]byte("{" + strings.Join(tables, ",") + "}")}, nil, nil, nil)
	names := map[string][2]string{} // by "table OID.attribute number"
	for rr.NextRow() {
		v := rr.Values()
		names[string(v[0])+"."+string(v[1])] = [2]string{string(v[2]), string(v[3])}
	}
	if _, err := rr.Close(); err != nil {
		return practicedb.Description{}, err
	}
	for i, f := range sd.Fields {
		if n, ok := names[fmt.Sprint(f.TableOID, ".", f.TableAttributeNumber)]; ok {
			desc.Columns[i].Table, desc.Columns[i].Origin = n[0], n[1]
		}
	}
	return desc, nil
}

// command is one command. setup ties its flags to fs and returns what to
// run once they are parsed.
type command struct {
	name, summary string
	args          int // the arguments after the flags, exactly; -1 for none or one
	setup         func(fs *flag.FlagSet) func(stdout, stderr io.Writer) int
}

// most is how many arguments c takes at most.
func (c command) most() int {
	if c.args < 0 {
		return 1
	}
	return c.args
}

// commands, in the order a project uses them.
var commands = []command{
	{"gen", "check the project, then write generated code", 0, noFlags(gen)},
	{"check", "check the project and that generated code is up to date", 0, noFlags(check)},
	{"makemigrations", "write the next migration from the models; --name NAME, --rename TABLE.OLD:NEW (repeatable)", 0, cmdMakeMigrations},
	{"showmigrations", "list the migrations and which the database has applied", 0, noFlags(cmdShowMigrations)},
	{"migrate", "apply the migrations the database has not applied", 0, noFlags(cmdMigrate)},
	{"add", "copy contrib app APP into the project, yours from then on (apps: " + strings.Join(scaffold.Apps(), ", ") + ")", 1, cmdAdd},
	{"doc", "show the reference of a topic; with no topic, list them", -1, cmdDoc},
	{"tailwind", "write each app's static/tailwind.css from its tailwind.css, with the standalone Tailwind CSS " + tailwindVersion, 0, noFlags(cmdTailwind)},
}

func noFlags(run func(stdout, stderr io.Writer) int) func(*flag.FlagSet) func(stdout, stderr io.Writer) int {
	return func(*flag.FlagSet) func(stdout, stderr io.Writer) int { return run }
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help": // extra words after help are harmless and ignored
		usage(stdout)
		return 0
	}
	for _, c := range commands {
		if c.name != args[0] {
			continue
		}
		fs := flag.NewFlagSet(c.name, flag.ContinueOnError)
		fs.SetOutput(stderr) // parse errors like "flag provided but not defined: -x"
		fs.Usage = func() {}
		run := c.setup(fs)
		err := fs.Parse(args[1:])
		switch {
		case errors.Is(err, flag.ErrHelp):
			usage(stdout)
			return 0
		case err != nil:
			usage(stderr)
			return 2
		case fs.NArg() > c.most():
			fmt.Fprintf(stderr, "maago %s: unexpected argument %q\n", c.name, fs.Arg(c.most()))
			usage(stderr)
			return 2
		case fs.NArg() < c.args:
			fmt.Fprintf(stderr, "maago %s: missing argument\n", c.name)
			usage(stderr)
			return 2
		}
		return run(stdout, stderr)
	}
	fmt.Fprintf(stderr, "maago: unknown command %q\n", args[0])
	usage(stderr)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprint(w, "usage: maago <command> [flags]\n\ncommands:\n")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-14s %s\n", c.name, c.summary)
	}
}

// gen and check work on the project holding the current directory.
// Diagnostic paths are relative to the project root (D-88).

func gen(stdout, stderr io.Writer) int {
	removed, ds := build.Gen(".")
	for _, p := range removed { // even if a later removal failed
		fmt.Fprintf(stdout, "removed %s\n", p)
	}
	if len(ds) > 0 {
		diag.Write(stderr, ds)
		return 1
	}
	// A line that says it worked: an agent does not have to look (M10 task 3).
	if root, ds := project.Root("."); ds == nil {
		if files, ds := pipeline.Owned(root); ds == nil {
			fmt.Fprintf(stdout, "generated %d file%s\n", len(files), plural(len(files)))
		}
	}
	return 0
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func check(_, stderr io.Writer) int {
	if ds := build.Check("."); len(ds) > 0 {
		diag.Write(stderr, ds)
		return 1
	}
	return 0
}

// cmdMakeMigrations writes the next migration (D-32). A malformed --name or
// --rename is a diagnostic with its fix, not a usage error (D-68).
func cmdMakeMigrations(fs *flag.FlagSet) func(stdout, stderr io.Writer) int {
	name := fs.String("name", "", "name the migration instead of naming it after its changes")
	var renames []string
	fs.Func("rename", "turn a dropped and an added column into a rename: TABLE.OLD:NEW", func(s string) error {
		renames = append(renames, s)
		return nil
	})
	return func(stdout, stderr io.Writer) int {
		written, ds := build.MakeMigrations(".", *name, renames)
		switch {
		case len(ds) > 0:
			diag.Write(stderr, ds)
			return 1
		case written == "":
			fmt.Fprintln(stdout, "no changes")
		default:
			fmt.Fprintf(stdout, "created %s\n", written)
			warn(stderr, written)
		}
		return 0
	}
}

// warn repeats the draft's ⚠ comments, which only a reader of the file
// would see otherwise. They are no diagnostics: the draft is written.
func warn(stderr io.Writer, written string) {
	root, ds := project.Root(".")
	if ds != nil {
		return
	}
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(written)))
	if err != nil {
		return
	}
	for i, line := range strings.Split(string(b), "\n") {
		if w, ok := strings.CutPrefix(line, "-- ⚠ "); ok {
			fmt.Fprintf(stderr, "%s:%d: ⚠ %s\n", written, i+1, w)
		}
	}
}

func cmdShowMigrations(stdout, stderr io.Writer) int {
	list, ds := build.ShowMigrations(".")
	if len(ds) > 0 {
		diag.Write(stderr, ds)
		return 1
	}
	fmt.Fprint(stdout, list)
	return 0
}

func cmdMigrate(stdout, stderr io.Writer) int {
	applied, ds := build.Migrate(".")
	for _, name := range applied { // even if a later file failed
		fmt.Fprintf(stdout, "applied %s\n", name)
	}
	if len(ds) > 0 {
		diag.Write(stderr, ds)
		return 1
	}
	if len(applied) == 0 {
		fmt.Fprintln(stdout, "nothing to apply")
	}
	return 0
}

// cmdAdd copies a contrib app into the project (D-123): the files it
// wrote on stdout, or why it wrote none.
func cmdAdd(fs *flag.FlagSet) func(stdout, stderr io.Writer) int {
	return func(stdout, stderr io.Writer) int {
		written, ds := scaffold.Add(".", fs.Arg(0))
		if len(ds) > 0 {
			diag.Write(stderr, ds)
			return 1
		}
		for _, p := range written {
			fmt.Fprintf(stdout, "wrote %s\n", p)
		}
		return 0
	}
}
