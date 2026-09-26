package migrate

import (
	"slices"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/schema"
)

// post is a table like the ones the readers return; each call is a fresh
// copy to change.
func post() *schema.Table {
	return &schema.Table{
		Name:   "post",
		Strict: true,
		Columns: []schema.Column{
			{Name: "id", Type: "INTEGER"},
			{Name: "title", Type: "TEXT", NotNull: true},
			{Name: "views", Type: "INTEGER", NotNull: true, Default: "0"},
			{Name: "author_id", Type: "INTEGER", NotNull: true},
		},
		PK:      []string{"id"},
		PKAuto:  "AUTOINCREMENT",
		FKs:     []schema.FK{{Name: "post_author_id_fkey", Columns: []string{"author_id"}, RefTable: "people", RefColumns: []string{"id"}, OnDelete: "NO ACTION", OnUpdate: "NO ACTION"}},
		Indexes: []schema.Index{{Name: "post_author_id_title_key", Columns: []string{"author_id", "title"}, Unique: true}},
		Checks:  []schema.Check{{Column: "title", Expr: `LENGTH ( "title" ) < = 200`}},
	}
}

func people() *schema.Table {
	return &schema.Table{Name: "people", Strict: true, Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}, PK: []string{"id"}, PKAuto: "AUTOINCREMENT"}
}

func messages(cs []Change) string {
	var b strings.Builder
	for _, c := range cs {
		b.WriteString(strings.TrimPrefix(c.Message(), "migrations do not match models: ") + "\n")
	}
	return b.String()
}

func TestDiff(t *testing.T) {
	tests := []struct {
		name string
		from func(*schema.Table) // changes post in the migrations
		to   func(*schema.Table) // changes post in the models
		want string
	}{
		{name: "same"},
		{
			name: "column order",
			from: func(t *schema.Table) { slices.Reverse(t.Columns) },
		},
		{
			name: "index name and constraint",
			from: func(t *schema.Table) { t.Indexes[0].Name, t.Indexes[0].Constraint = "other", true },
		},
		{
			name: "unique column order",
			from: func(t *schema.Table) { t.Indexes[0].Columns = []string{"title", "author_id"} },
		},
		{
			name: "index column order",
			from: func(t *schema.Table) {
				t.Indexes = []schema.Index{{Name: "x", Columns: []string{"title", "author_id"}}}
			},
			to: func(t *schema.Table) {
				t.Indexes = []schema.Index{{Name: "x", Columns: []string{"author_id", "title"}}}
			},
			want: `index ("title", "author_id") of "post" is not in the models
index ("author_id", "title") of "post" is missing
`,
		},
		{
			name: "add and drop column",
			from: func(t *schema.Table) { t.Columns = append(t.Columns, schema.Column{Name: "summary", Type: "TEXT"}) },
			to:   func(t *schema.Table) { t.Columns = append(t.Columns, schema.Column{Name: "body", Type: "BLOB"}) },
			want: `column "summary" of "post" is not in the models
column "body" of "post" is missing
`,
		},
		{
			name: "type and default of one column",
			from: func(t *schema.Table) { t.Columns[2].Type, t.Columns[2].Default = "REAL", "1 . 5" },
			want: `column "views" of "post" is REAL, the models have INTEGER
column "views" of "post" has DEFAULT 1.5, the models have DEFAULT 0
`,
		},
		{
			name: "not null and no default",
			from: func(t *schema.Table) { t.Columns[1].NotNull = false },
			to:   func(t *schema.Table) { t.Columns[2].Default = "" },
			want: `column "title" of "post" is NULL, the models have NOT NULL
column "views" of "post" has DEFAULT 0, the models have no default
`,
		},
		{
			name: "strict",
			from: func(t *schema.Table) { t.Strict = false },
			want: `table "post" is not STRICT, the models have STRICT
`,
		},
		{
			name: "primary key autoincrement",
			from: func(t *schema.Table) { t.PKAuto = "" },
			want: `primary key ("id") of "post" is not in the models
primary key ("id") AUTOINCREMENT of "post" is missing
`,
		},
		{
			name: "primary key only in the models",
			from: func(t *schema.Table) { t.PK, t.PKAuto = nil, "" },
			want: `primary key ("id") AUTOINCREMENT of "post" is missing
`,
		},
		{
			name: "unique and index on one column",
			from: func(t *schema.Table) {
				t.Indexes = []schema.Index{{Name: "a", Columns: []string{"title"}, Unique: true}, {Name: "b", Columns: []string{"title"}}}
			},
			to: func(t *schema.Table) {
				t.Indexes = []schema.Index{{Name: "c", Columns: []string{"title"}, Unique: true}}
			},
			want: `index ("title") of "post" is not in the models
`,
		},
		{
			name: "foreign key on update",
			from: func(t *schema.Table) { t.FKs[0].OnUpdate = "CASCADE" },
			want: `foreign key ("author_id") -> "people" ("id") ON DELETE NO ACTION ON UPDATE CASCADE of "post" is not in the models
foreign key ("author_id") -> "people" ("id") ON DELETE NO ACTION of "post" is missing
`,
		},
		{
			name: "foreign key target column",
			from: func(t *schema.Table) { t.FKs[0].RefColumns = []string{"code"} },
			want: `foreign key ("author_id") -> "people" ("code") ON DELETE NO ACTION of "post" is not in the models
foreign key ("author_id") -> "people" ("id") ON DELETE NO ACTION of "post" is missing
`,
		},
		{
			name: "foreign key target table",
			from: func(t *schema.Table) { t.FKs[0].RefTable = "users" },
			want: `foreign key ("author_id") -> "users" ("id") ON DELETE NO ACTION of "post" is not in the models
foreign key ("author_id") -> "people" ("id") ON DELETE NO ACTION of "post" is missing
`,
		},
		{
			name: "two equal checks against one",
			from: func(t *schema.Table) { t.Checks = append(t.Checks, t.Checks[0]) },
			want: `check (LENGTH ("title") <= 200) of "post" is not in the models
`,
		},
		{
			name: "changed check",
			to:   func(t *schema.Table) { t.Checks[0].Expr = `LENGTH ( "title" ) < = 100` },
			want: `check (LENGTH ("title") <= 200) of "post" is not in the models
check (LENGTH ("title") <= 100) of "post" is missing
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from, to := post(), post()
			if tt.from != nil {
				tt.from(from)
			}
			if tt.to != nil {
				tt.to(to)
			}
			got := messages(Diff(&schema.Schema{Tables: []*schema.Table{from, people()}}, &schema.Schema{Tables: []*schema.Table{to, people()}}))
			if got != tt.want {
				t.Errorf("got\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

// Every kind once, over tables given out of name order: the changes come
// in kind order, then table name order.
func TestDiffKinds(t *testing.T) {
	from, to := post(), post()
	from.FKs[0].OnDelete = "CASCADE"
	from.Indexes[0].Columns = []string{"title"}
	from.Checks[0].Expr = "1"
	from.PKAuto = ""
	from.Columns = append(from.Columns, schema.Column{Name: "old", Type: "TEXT"})
	to.Columns = append(to.Columns, schema.Column{Name: "body", Type: "BLOB"})
	from.Columns[1].Type = "BLOB"
	from.Columns[2].NotNull, from.Columns[2].Default = false, "1"
	from.Strict = false
	fromPeople, toPeople := people(), people()
	toPeople.Columns = append(toPeople.Columns, schema.Column{Name: "name", Type: "TEXT"})
	gone := &schema.Table{Name: "gone", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}}
	abandoned := &schema.Table{Name: "abandoned", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}}
	fresh := &schema.Table{Name: "fresh", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}}
	cs := Diff(&schema.Schema{Tables: []*schema.Table{from, gone, fromPeople, abandoned}}, &schema.Schema{Tables: []*schema.Table{to, fresh, toPeople}})
	want := `foreign key ("author_id") -> "people" ("id") ON DELETE CASCADE of "post" is not in the models
unique index ("title") of "post" is not in the models
check (1) of "post" is not in the models
primary key ("id") of "post" is not in the models
table "abandoned" is not in the models
table "gone" is not in the models
column "old" of "post" is not in the models
table "fresh" is missing
column "name" of "people" is missing
column "body" of "post" is missing
column "title" of "post" is BLOB, the models have TEXT
column "views" of "post" is NULL, the models have NOT NULL
column "views" of "post" has DEFAULT 1, the models have DEFAULT 0
table "post" is not STRICT, the models have STRICT
primary key ("id") AUTOINCREMENT of "post" is missing
unique index ("author_id", "title") of "post" is missing
check (LENGTH ("title") <= 200) of "post" is missing
foreign key ("author_id") -> "people" ("id") ON DELETE NO ACTION of "post" is missing
`
	if got := messages(cs); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// A drop takes what it drops from the migrations, whose names the draft
// writes; an add takes it from the models.
func TestDiffSides(t *testing.T) {
	from, to := post(), post()
	from.Indexes[0].Columns = []string{"title"}
	from.FKs[0].Name, to.FKs[0].Name = "from_fk", "to_fk"
	from.FKs[0].OnDelete = "CASCADE"
	from.Checks[0].Name, to.Checks[0].Name = "from_check", "to_check"
	to.Checks[0].Expr = "1"
	cs := Diff(&schema.Schema{Tables: []*schema.Table{from}}, &schema.Schema{Tables: []*schema.Table{to}})
	for _, c := range cs {
		var ok bool
		switch c.Kind {
		case DropIndex:
			ok = c.Index == &from.Indexes[0]
		case DropFK:
			ok = c.FK == &from.FKs[0]
		case DropCheck:
			ok = c.Check == &from.Checks[0]
		case AddIndex:
			ok = c.Index == &to.Indexes[0]
		case AddFK:
			ok = c.FK == &to.FKs[0]
		case AddCheck:
			ok = c.Check == &to.Checks[0]
		}
		if !ok || c.From != from || c.To != to {
			t.Errorf("%s: from the wrong side", c.Message())
		}
	}
	if len(cs) != 6 {
		t.Errorf("got %d changes, want 6:\n%s", len(cs), messages(cs))
	}
}

func TestDiffTables(t *testing.T) {
	named := func(name string, refs ...string) *schema.Table {
		t := &schema.Table{Name: name, Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}}
		for _, r := range refs {
			t.FKs = append(t.FKs, schema.FK{Columns: []string{"id"}, RefTable: r, RefColumns: []string{"id"}, OnDelete: "NO ACTION", OnUpdate: "NO ACTION"})
		}
		return t
	}
	// a points at c and c at b, so the drops go a, c, b: neither name
	// order nor its reverse. e points at itself and at d; its own foreign
	// key must not hold it back. kept points at b, so its foreign key
	// goes first.
	from := &schema.Schema{Tables: []*schema.Table{named("kept", "b"), named("b"), named("c", "b"), named("a", "c"), named("d"), named("e", "e", "d")}}
	to := &schema.Schema{Tables: []*schema.Table{named("new", "kept"), named("kept")}}
	cs := Diff(from, to)
	want := `foreign key ("id") -> "b" ("id") ON DELETE NO ACTION of "kept" is not in the models
table "a" is not in the models
table "c" is not in the models
table "b" is not in the models
table "e" is not in the models
table "d" is not in the models
table "new" is missing
`
	if got := messages(cs); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestDisplay(t *testing.T) {
	tests := []struct{ in, want string }{
		{"1 . 5", "1.5"},
		{`LENGTH ( "slug" ) < = 100`, `LENGTH ("slug") <= 100`},
		{`"public" IN ( 0 , 1 )`, `"public" IN (0, 1)`},
		{"'draft' : : TEXT", "'draft'::TEXT"},
		{"( STATUS = ANY ( ARRAY [ 'draft' : : TEXT , 'published' : : TEXT ] ) )", "STATUS = ANY (ARRAY['draft'::TEXT, 'published'::TEXT])"},
		{"( ( A > 0 ) )", "A > 0"},
		{"( A ) OR ( B )", "(A) OR (B)"},
		{"A < > B OR C ! = D OR E | | F", "A <> B OR C != D OR E || F"},
		{"A < - 1", "A < - 1"},
		{"'a ( b'", "'a ( b'"},
	}
	for _, tt := range tests {
		if got := display(tt.in); got != tt.want {
			t.Errorf("display(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
