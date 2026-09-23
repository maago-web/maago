package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	const toStdout, toStderr = true, false
	tests := []struct {
		args     []string
		code     int
		toStdout bool
		want     []string // substrings of the non-empty stream
	}{
		{nil, 2, toStderr, []string{"usage:", "gen", "check"}},
		{[]string{"nope"}, 2, toStderr, []string{`unknown command "nope"`, "usage:", "gen", "check"}},
		{[]string{"help"}, 0, toStdout, []string{"usage:", "gen", "check"}},
		{[]string{"-h"}, 0, toStdout, []string{"usage:"}},
		{[]string{"--help"}, 0, toStdout, []string{"usage:"}},
		{[]string{"gen", "-h"}, 0, toStdout, []string{"usage:"}},
		{[]string{"gen", "-x"}, 2, toStderr, []string{"-x", "usage:"}},
		{[]string{"gen", "extra"}, 2, toStderr, []string{`"extra"`, "usage:"}},
		{[]string{"gen"}, 1, toStderr, []string{"maago gen: not implemented"}},
		{[]string{"check"}, 1, toStderr, []string{"maago check: not implemented"}},
		{[]string{"gen", "--"}, 1, toStderr, []string{"maago gen: not implemented"}},
		{[]string{""}, 2, toStderr, []string{`unknown command ""`, "usage:"}},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)
			if code != tt.code {
				t.Errorf("code = %d, want %d", code, tt.code)
			}
			out, other := stderr.String(), stdout.String()
			if tt.toStdout {
				out, other = other, out
			}
			if other != "" {
				t.Errorf("wrong stream got output:\n%s", other)
			}
			if n := strings.Count(out, "usage:"); n > 1 {
				t.Errorf("usage printed %d times:\n%s", n, out)
			}
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("output lacks %q:\n%s", w, out)
				}
			}
		})
	}
}

// The usage text is what agents read first; lock it, including command order.
func TestUsage(t *testing.T) {
	var stdout bytes.Buffer
	run([]string{"help"}, &stdout, &bytes.Buffer{})
	const want = `usage: maago <command>

commands:
  gen    check the project, then write generated code
  check  check the project and that generated code is up to date
`
	if got := stdout.String(); got != want {
		t.Errorf("usage:\n%s\nwant:\n%s", got, want)
	}
}
