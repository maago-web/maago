// Package diag holds what Maago reports when something is wrong: where it
// is, what is wrong, and how to fix it (D-61, D-62, D-63). Every
// diagnostic is an error; there are no warnings.
package diag

import (
	"cmp"
	"fmt"
	"go/token"
	"io"
	"slices"
	"strings"
)

// Diagnostic is one error. Its fields are unexported so that New, which
// refuses a diagnostic without a fix, is the only way to fill one in.
type Diagnostic struct {
	pos      token.Position
	msg, fix string
}

// New returns a diagnostic at pos. pos.Filename is relative to the project
// root; an empty Filename means the diagnostic has no position. msg and fix
// are one line of English each. A missing msg or fix is a bug in the
// caller, so New panics.
func New(pos token.Position, msg, fix string) Diagnostic {
	if msg == "" || fix == "" {
		panic(fmt.Sprintf("diag: a diagnostic needs a message and a fix (D-62): %q, %q", msg, fix))
	}
	if strings.ContainsAny(msg, "\r\n") || strings.ContainsAny(fix, "\r\n") {
		panic(fmt.Sprintf("diag: message and fix must be one line each: %q, %q", msg, fix))
	}
	pos.Filename = OneLine(pos.Filename)
	// Clear what is never printed, so it cannot keep duplicates apart.
	pos.Offset = 0
	if pos.Filename == "" {
		pos.Line = 0
	}
	if pos.Line == 0 {
		pos.Column = 0
	}
	return Diagnostic{pos, msg, fix}
}

// Write prints ds sorted by position, then message, with duplicates
// removed. Diagnostics without a position come first. ds is not modified.
func Write(w io.Writer, ds []Diagnostic) error {
	ds = slices.Clone(ds)
	slices.SortFunc(ds, compare)
	ds = slices.Compact(ds)
	var b strings.Builder
	for _, d := range ds {
		if p := d.pos; p.Filename != "" {
			b.WriteString(p.Filename)
			if p.Line > 0 {
				fmt.Fprintf(&b, ":%d", p.Line)
				if p.Column > 0 {
					fmt.Fprintf(&b, ":%d", p.Column)
				}
			}
			b.WriteString(": ")
		}
		fmt.Fprintf(&b, "%s\n  fix: %s\n", d.msg, d.fix)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// OneLine replaces line breaks in s with \r and \n escapes, for putting
// text such as an OS error into a message.
func OneLine(s string) string {
	return strings.NewReplacer("\r", `\r`, "\n", `\n`).Replace(s)
}

// compare orders by every field, so that the unstable sort always puts
// duplicates next to each other.
func compare(a, b Diagnostic) int {
	return cmp.Or(
		cmp.Compare(a.pos.Filename, b.pos.Filename),
		cmp.Compare(a.pos.Line, b.pos.Line),
		cmp.Compare(a.pos.Column, b.pos.Column),
		cmp.Compare(a.msg, b.msg),
		cmp.Compare(a.fix, b.fix),
	)
}
