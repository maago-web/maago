package model_test

import (
	"fmt"
	"go/token"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/golden"
	"github.com/maago-web/maago/internal/model"
	"github.com/maago-web/maago/internal/model/modeltest"
)

// The example project in internal/testdata is shared with the dialect
// tests, so the DDL goldens are of the same models.
func TestCheckValid(t *testing.T) {
	apps, fset := modeltest.Example(t, "../testdata/example")
	golden.Check(t, "testdata/valid.golden", []byte(dump(fset, apps)))
}

// dump prints checked apps for the golden file.
func dump(fset *token.FileSet, apps []*model.App) string {
	var b strings.Builder
	for _, app := range apps {
		fmt.Fprintf(&b, "app %s (%s)\n", app.Name, app.Dir)
		for _, m := range app.Models {
			fmt.Fprintf(&b, "  model %s @%s table=%s ordering=%v uniques=%v indexes=%v\n",
				m.Name, fset.Position(m.Pos), m.Table, m.Ordering, m.Uniques, m.Indexes)
			for _, f := range m.Fields {
				fmt.Fprintf(&b, "    %s(%s) %s", f.Name, f.Column, f.Type)
				if f.Pointer {
					b.WriteString(" null")
				}
				if f.Max > 0 {
					fmt.Fprintf(&b, " max=%d", f.Max)
				}
				if f.Default != nil {
					fmt.Fprintf(&b, " default=%q", *f.Default)
				}
				if f.Choices != nil {
					fmt.Fprintf(&b, " choices=%q", f.Choices)
				}
				if f.Label != "" {
					fmt.Fprintf(&b, " label=%q", f.Label)
				}
				if f.Help != "" {
					fmt.Fprintf(&b, " help=%q", f.Help)
				}
				if f.FK != "" {
					fmt.Fprintf(&b, " fk=%s", f.FK)
				}
				if f.Auto != "" {
					fmt.Fprintf(&b, " auto=%s", f.Auto)
				}
				b.WriteString("\n")
			}
			for _, u := range m.Unsupported {
				fmt.Fprintf(&b, "    (not a column) %s: %s\n", u.Name, u.Reason)
			}
		}
	}
	return b.String()
}
