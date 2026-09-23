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

	_ "github.com/maago-web/maago" // temporary: keeps the require until the CLI uses the runtime
)

type command struct {
	name, summary string
	run           func(stdout, stderr io.Writer) int
}

var commands = []command{
	{"gen", "check the project, then write generated code", notImplemented("gen", 7)},
	{"check", "check the project and that generated code is up to date", notImplemented("check", 8)},
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

func notImplemented(name string, task int) func(stdout, stderr io.Writer) int {
	return func(_, stderr io.Writer) int {
		fmt.Fprintf(stderr, "maago %s: not implemented yet (M0 task %d)\n", name, task)
		return 1
	}
}
