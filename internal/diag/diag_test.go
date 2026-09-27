package diag

import (
	"bytes"
	"go/token"
	"slices"
	"testing"

	"github.com/maago-web/maago/internal/golden"
)

func TestWrite(t *testing.T) {
	models := func(line, col, offset int) token.Position {
		return token.Position{Filename: "blog/models.go", Line: line, Column: col, Offset: offset}
	}
	// Shuffled; holds an exact duplicate, a duplicate differing only in
	// Offset, same line with different columns, same position with
	// different messages and with different fixes (message and fix order
	// disagree), and every position shape: none, file only, file:line,
	// file:line:col.
	ds := []Diagnostic{
		New(token.Position{Filename: "blog/urls.go", Line: 5, Column: 9}, `unknown route name "post_detial"`, `did you mean "post_detail"?`),
		New(models(14, 2, 0), "field Post.Body has no type", "add a type after the field name"),
		New(models(12, 18, 0), `unknown tag key "maxlen" in Post.Title`, "remove the key"),
		New(token.Position{}, "cannot connect to the practice database", "start PostgreSQL, or set MAAGO_DATABASE_URL in .env"),
		New(models(12, 18, 311), `unknown tag key "maxlen" in Post.Title`, `did you mean "max"?`),
		New(models(3, 0, 0), "package clause is missing", `add "package blog" as the first line`),
		New(models(12, 18, 0), `unknown tag key "maxlen" in Post.Title`, `did you mean "max"?`),
		New(models(0, 0, 0), "no models found", "embed maago.Model in a struct"),
		New(models(14, 2, 0), "field Post.Body has no type", "add a type after the field name"),
		New(models(12, 18, 0), `duplicate key "max" in Post.Title`, "remove the second key"),
		New(models(12, 4, 0), `empty value for key "max"`, "give max a number, like max=200"),
		// Unprinted position parts must not keep these apart from the ones above.
		New(token.Position{Line: 7, Column: 3}, "cannot connect to the practice database", "start PostgreSQL, or set MAAGO_DATABASE_URL in .env"),
		New(models(0, 4, 0), "no models found", "embed maago.Model in a struct"),
	}
	before := slices.Clone(ds)

	var out bytes.Buffer
	if err := Write(&out, ds); err != nil {
		t.Fatal(err)
	}
	golden.Check(t, "testdata/write.golden", out.Bytes())

	if !slices.Equal(ds, before) {
		t.Error("Write reordered the caller's slice")
	}
}

func TestNewRejects(t *testing.T) {
	tests := []struct{ name, msg, fix string }{
		{"empty msg", "", "fix it"},
		{"empty fix", "broken", ""},
		{"newline in msg", "broken\nreally", "fix it"},
		{"newline in fix", "broken", "fix\nit"},
		{"carriage return in msg", "broken\rreally", "fix it"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("New did not panic")
				}
			}()
			New(token.Position{}, tt.msg, tt.fix)
		})
	}
}

func TestSuggest(t *testing.T) {
	allowed := []string{"max", "maxima", "table", "On"}
	for _, c := range []struct{ got, want string }{
		{"maxlen", "max"},  // got starts with an allowed name
		{"ma", "max"},      // an allowed name starts with got; ties go to the earlier
		{"MAX", "max"},     // case does not matter
		{"tabel", "table"}, // distance 2, within ceil(5/3)
		{"tbl", ""},        // distance 2, beyond ceil(3/3)
		{"Foo", ""},        // distance 2 from On, beyond 1
		{"xyz", ""},
	} {
		if got := Suggest(c.got, allowed); got != c.want {
			t.Errorf("Suggest(%q) = %q, want %q", c.got, got, c.want)
		}
	}
}
