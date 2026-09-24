package model

import (
	"strings"
	"unicode"
)

// snake returns a Go name in snake_case, for table and column names
// (D-76): AuthorID → author_id, HTTPStatus → http_status. A "_" goes
// before an upper-case letter that follows a lower-case letter or a digit,
// and before the last letter of an upper-case run that a lower-case letter
// follows, unless that letter is the s of a plural acronym. Digits never
// start a word.
func snake(name string) string {
	r := []rune(name)
	var b strings.Builder
	for i, c := range r {
		if i > 0 && unicode.IsUpper(c) {
			prev := r[i-1]
			// A plural acronym keeps its s: URLs → urls, not ur_ls.
			plural := i+1 < len(r) && r[i+1] == 's' && (i+2 == len(r) || !unicode.IsLower(r[i+2]))
			nextLower := i+1 < len(r) && unicode.IsLower(r[i+1]) && !plural
			if unicode.IsLower(prev) || unicode.IsDigit(prev) || unicode.IsUpper(prev) && nextLower {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToLower(c))
	}
	return b.String()
}
