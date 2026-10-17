package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/ref"
)

// examples are each topic's example, a whole project that
// TestDocExamples builds and checks (D-152).
//
//go:embed docs
var examples embed.FS

// cmdDoc prints a topic of the reference, or lists them.
func cmdDoc(fs *flag.FlagSet) func(stdout, stderr io.Writer) int {
	return func(stdout, stderr io.Writer) int {
		if fs.NArg() == 0 {
			fmt.Fprint(stdout, "usage: maago doc <topic>\n\ntopics:\n")
			for _, t := range ref.Topics {
				fmt.Fprintf(stdout, "  %-10s %s\n", t.Name, t.Title)
			}
			return 0
		}
		t := ref.Find(fs.Arg(0))
		if t == nil {
			var names []string
			for _, t := range ref.Topics {
				names = append(names, t.Name)
			}
			msg := "topics: " + strings.Join(names, ", ")
			if s := diag.Suggest(fs.Arg(0), names); s != "" {
				msg = fmt.Sprintf("did you mean %q? %s", s, msg)
			}
			fmt.Fprintf(stderr, "maago doc: unknown topic %q\n  %s\n", fs.Arg(0), msg)
			return 2
		}
		ref.Print(stdout, t)
		example, err := examples.ReadFile("docs/" + t.Name + ".txt")
		if err != nil {
			panic(err) // TestDocExamples has one for each topic
		}
		fmt.Fprintf(stdout, "\nExample: module example.com/app, .env with MAAGO_DATABASE_URL=sqlite:app.db; lines starting with $ run first.\nmaago makemigrations, gen and check pass on it, and its tests pass.\n\n%s", example)
		return 0
	}
}
