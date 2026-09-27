package sqlite

import (
	"testing"

	"github.com/maago-web/maago/internal/golden"
	"github.com/maago-web/maago/internal/model/modeltest"
)

// The golden file is checked by hand and applied to a real SQLite by
// cmd/maago's example_test.go; this test keeps it from changing unnoticed.
func TestDDL(t *testing.T) {
	apps, fset := modeltest.Example(t, "../../testdata/example")
	ddl, ds := DDL(fset, apps)
	if len(ds) > 0 {
		t.Fatal(ds)
	}
	golden.Check(t, "testdata/example.sql", []byte(ddl))
}
