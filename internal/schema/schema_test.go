package schema

import (
	"slices"
	"testing"
)

// Foreign keys on one column to one table, differing only in action,
// sort the same whatever order they come in.
func TestSortForeignKeys(t *testing.T) {
	fk := func(del, upd, name string) FK {
		return FK{Name: name, Columns: []string{"a"}, RefTable: "p", RefColumns: []string{"id"}, OnDelete: del, OnUpdate: upd}
	}
	want := []FK{fk("CASCADE", "NO ACTION", "x"), fk("NO ACTION", "CASCADE", "x"), fk("NO ACTION", "NO ACTION", "x"), fk("NO ACTION", "NO ACTION", "y")}
	reversed := slices.Clone(want)
	slices.Reverse(reversed)
	for _, fks := range [][]FK{slices.Clone(want), reversed} {
		s := &Schema{Tables: []*Table{{FKs: fks}}}
		s.Sort()
		if !slices.EqualFunc(fks, want, func(a, b FK) bool { return a.Name == b.Name && a.OnDelete == b.OnDelete && a.OnUpdate == b.OnUpdate }) {
			t.Fatalf("got %v, want %v", fks, want)
		}
	}
}
