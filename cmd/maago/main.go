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

	// The practice database opens drivers by name (D-89, D-90).
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type command struct {
	name, summary string
	run           func(stdout, stderr io.Writer) int
}

var commands = []command{
	{"gen", "check the project, then write generated code", gen},
	{"check", "check the project and that generated code is up to date", check},
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
		return c.run(stdout, stderr)
	}
	fmt.Fprintf(stderr, "maago: unknown command %q\n", args[0])
	usage(stderr)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprint(w, "usage: maago <command>\n\ncommands:\n")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-6s %s\n", c.name, c.summary)
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
