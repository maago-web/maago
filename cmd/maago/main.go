// Command maago is the Maago CLI.
//
// Exit codes: 0 success or help, 1 checks failed, 2 usage error.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/maago-web/maago/internal/build"
	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/practicedb"

	// The practice database opens drivers by name (D-89, D-90).
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

func init() {
	practicedb.ErrorPosition = func(err error) int {
		var pe *pgconn.PgError
		if errors.As(err, &pe) {
			return int(pe.Position)
		}
		return 0
	}
}

// command is one command. setup ties its flags to fs and returns what to
// run once they are parsed.
type command struct {
	name, summary string
	setup         func(fs *flag.FlagSet) func(stdout, stderr io.Writer) int
}

// commands, in the order a project uses them.
var commands = []command{
	{"gen", "check the project, then write generated code", noFlags(gen)},
	{"check", "check the project and that generated code is up to date", noFlags(check)},
	{"makemigrations", "write the next migration from the models; --name NAME, --rename TABLE.OLD:NEW (repeatable)", cmdMakeMigrations},
	{"showmigrations", "list the migrations and which the database has applied", noFlags(cmdShowMigrations)},
	{"migrate", "apply the migrations the database has not applied", noFlags(cmdMigrate)},
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
		case fs.NArg() > 0:
			fmt.Fprintf(stderr, "maago %s: unexpected argument %q\n", c.name, fs.Arg(0))
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
	return 0
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
		}
		return 0
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
