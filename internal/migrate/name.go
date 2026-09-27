package migrate

import (
	"strings"
)

// Rename is a column rename given with --rename table.old:new, in
// database names (D-36).
type Rename struct {
	Table, Old, New string
}

// ParseRename reads table.old:new.
func ParseRename(s string) (Rename, bool) {
	table, cols, ok1 := strings.Cut(s, ".")
	old, new, ok2 := strings.Cut(cols, ":")
	r := Rename{table, old, new}
	return r, ok1 && ok2 && table != "" && old != "" && new != "" && !strings.Contains(old, ".") && !strings.ContainsAny(new, ".:")
}

// Name names a migration after what it does (D-94): add_blog_post_body
// for one change, the table for several of one table, and the first
// change's name with _and_more for several tables. Renames come first,
// then the changes in their order; a change that follows from an added or
// dropped column is not counted, as Diagnose does not report it.
func Name(cs []Change, renames []Rename) string {
	type named struct{ table, name string }
	var items []named
	for _, r := range renames {
		items = append(items, named{r.Table, "rename_" + r.Table + "_" + r.Old})
	}
	for _, c := range cs {
		if !follows(c, columns(c), cs) {
			items = append(items, named{c.Table(), changeName(c)})
		}
	}
	switch {
	case len(items) == 0:
		return ""
	case len(items) == 1:
		return clean(items[0].name)
	}
	for _, it := range items {
		if it.table != items[0].table {
			return clean(items[0].name + "_and_more")
		}
	}
	return clean(items[0].table)
}

func changeName(c Change) string {
	t := c.Table()
	switch c.Kind {
	case CreateTable:
		return "create_" + t
	case DropTable:
		return "drop_" + t
	case AddColumn:
		return "add_" + t + "_" + c.New.Name
	case DropColumn:
		return "drop_" + t + "_" + c.Old.Name
	case AlterType, AlterNotNull, AlterDefault:
		return "alter_" + t + "_" + c.New.Name
	}
	return "alter_" + t
}

// clean keeps a name to what a migration file name holds: [a-z0-9_].
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z':
			return r - 'A' + 'a'
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			return r
		}
		return '_'
	}, s)
}
