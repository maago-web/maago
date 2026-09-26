package dialect

import (
	"fmt"
	"go/token"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/maago-web/maago/internal/diag"
)

// Kind is the kind of a Token.
type Kind int

const (
	Word    Kind = iota // unquoted name, keyword or number piece
	Quoted              // "name" or `name`
	Literal             // 'text', E'text' or $tag$text$tag$
	Punct               // any other single character
)

func (k Kind) String() string {
	return [...]string{"word", "quoted", "literal", "punct"}[k]
}

// Token is one token of SQL. Text is as written, quotes included.
type Token struct {
	Kind Kind
	Text string
	Pos  token.Position
}

// Stmt is one statement of a SQL file.
type Stmt struct {
	Pos token.Position // of the first token, Offset included
	// SQL is the source from the first token to the last, comments
	// inside included, without the closing semicolon.
	SQL       string
	Tokens    []Token
	AllowDrop token.Position // of the -- maago: allow-drop before it; zero if none
}

const marker = "-- maago: allow-drop"

// Split splits src into statements at top-level semicolons, dropping
// statements without tokens. It knows strings, quoted names, comments and
// dollar quoting of both SQLite and PostgreSQL. Where the two differ, it
// never reads as string or comment what a database reads as SQL, so no
// statement can hide from the checks on Tokens. An unterminated string or
// comment gives no statements; a misplaced marker (D-37) still gives them.
func Split(filename, src string) ([]Stmt, []diag.Diagnostic) {
	s := splitter{file: filename, src: src, lines: []int{0}}
	for i := range len(src) {
		if src[i] == '\n' {
			s.lines = append(s.lines, i+1)
		}
	}
	if !s.run() {
		return nil, s.ds
	}
	return s.stmts, s.ds
}

type splitter struct {
	file, src string
	lines     []int // offsets where lines start

	stmts   []Stmt
	toks    []Token // of the open statement
	end     int     // where the last token ends
	allow   token.Position
	pending token.Position // a marker waiting for its statement
	after   int            // where the pending marker's comment ends
	lastRow int            // line of the last token or semicolon
	ds      []diag.Diagnostic
}

func (s *splitter) pos(off int) token.Position {
	line := sort.SearchInts(s.lines, off+1)
	return token.Position{Filename: s.file, Offset: off, Line: line, Column: off - s.lines[line-1] + 1}
}

func (s *splitter) fail(off int, msg, fix string) {
	s.ds = append(s.ds, diag.New(s.pos(off), msg, fix))
}

// run scans src. It returns false at an unterminated string or comment.
func (s *splitter) run() bool {
	src := s.src
	for i := 0; i < len(src); {
		r, size := utf8.DecodeRuneInString(src[i:])
		next := byte(0)
		if i+1 < len(src) {
			next = src[i+1]
		}
		switch {
		case unicode.IsSpace(r):
			i += size
		case r == '-' && next == '-':
			end := strings.IndexByte(src[i:], '\n')
			if end < 0 {
				end = len(src) - i
			}
			s.comment(i, i+end)
			i += end
		case r == '/' && next == '*':
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				s.fail(i, "unterminated comment", "close it with */")
				return false
			}
			i += 2 + end + 2
		case r == ';':
			s.flush()
			s.lastRow = s.pos(i).Line
			i++
		case r == '\'':
			if i = s.quoted(i, i, Literal, false); i < 0 {
				return false
			}
		case (r == 'E' || r == 'e') && next == '\'':
			if i = s.quoted(i, i+1, Literal, true); i < 0 {
				return false
			}
		case r == '"' || r == '`':
			if i = s.quoted(i, i, Quoted, false); i < 0 {
				return false
			}
		case r == '$' && s.tag(i) != "":
			tag := s.tag(i)
			end := strings.Index(src[i+len(tag):], tag)
			if end < 0 {
				s.fail(i, "unterminated dollar-quoted string", "close it with "+tag)
				return false
			}
			i = s.add(Literal, i, i+len(tag)+end+len(tag))
		case isWordStart(r):
			j := i + size
			for j < len(src) {
				r, size := utf8.DecodeRuneInString(src[j:])
				if !isWordStart(r) && !unicode.IsDigit(r) && r != '$' {
					break
				}
				j += size
			}
			i = s.add(Word, i, j)
		default:
			i = s.add(Punct, i, i+size)
		}
	}
	s.flush()
	if s.pending.Line > 0 {
		s.notBefore(s.pending)
	}
	return true
}

func isWordStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// tag returns the dollar-quote tag starting at i, such as $$ or $fn$, or
// "" if there is none.
func (s *splitter) tag(i int) string {
	j := i + 1
	for j < len(s.src) {
		r, size := utf8.DecodeRuneInString(s.src[j:])
		if r == '$' {
			return s.src[i : j+1]
		}
		if r != '_' && !unicode.IsLetter(r) && (j == i+1 || !unicode.IsDigit(r)) {
			return ""
		}
		j += size
	}
	return ""
}

// quoted reads the token starting at start whose quote is at q, and
// returns where it ends, or -1 if it is unterminated. A doubled quote
// stands for itself; with backslash, so does a backslashed character.
func (s *splitter) quoted(start, q int, kind Kind, backslash bool) int {
	c := s.src[q]
	for j := q + 1; j < len(s.src); j++ {
		switch {
		case backslash && s.src[j] == '\\':
			j++
		case s.src[j] == c && j+1 < len(s.src) && s.src[j+1] == c:
			j++
		case s.src[j] == c:
			return s.add(kind, start, j+1)
		}
	}
	what := map[byte]string{'\'': "string", '"': "quoted name", '`': "quoted name"}[c]
	s.fail(start, "unterminated "+what, "close it with "+string(c))
	return -1
}

// add appends the token src[i:j] to the open statement and returns j. The
// first token of a statement takes a marker waiting on the line before.
func (s *splitter) add(kind Kind, i, j int) int {
	p := s.pos(i)
	if len(s.toks) == 0 && s.pending.Line > 0 {
		// Only the line break may stand between the marker and the statement.
		if gap := s.src[s.after:i]; strings.TrimSpace(gap) == "" && strings.Count(gap, "\n") == 1 {
			s.allow = s.pending
		} else {
			s.notBefore(s.pending)
		}
		s.pending = token.Position{}
	}
	s.toks = append(s.toks, Token{kind, s.src[i:j], p})
	s.end, s.lastRow = j, p.Line
	return j
}

func (s *splitter) flush() {
	if len(s.toks) > 0 {
		first := s.toks[0].Pos
		s.stmts = append(s.stmts, Stmt{first, s.src[first.Offset:s.end], s.toks, s.allow})
	}
	s.toks, s.allow = nil, token.Position{}
}

// comment checks the -- comment src[i:end]. Only "maago:" comments are
// markers.
func (s *splitter) comment(i, end int) {
	text := strings.TrimSpace(s.src[i+2 : end])
	if !strings.HasPrefix(text, "maago:") {
		return
	}
	p := s.pos(i)
	switch name := strings.TrimSpace(strings.TrimPrefix(text, "maago:")); {
	case name != "allow-drop":
		s.fail(i, fmt.Sprintf("unknown maago marker %q", name), "the only marker is "+marker)
	case s.lastRow == p.Line:
		s.fail(i, marker+" must be on its own line", "move it to its own line right before the DROP statement")
	case len(s.toks) > 0:
		n := s.toks[0].Pos.Line
		s.fail(i, fmt.Sprintf("%s is inside the statement starting at line %d", marker, n), fmt.Sprintf("end the statement starting at line %d with ;", n))
	default:
		if s.pending.Line > 0 {
			s.notBefore(s.pending)
		}
		s.pending, s.after = p, end
	}
}

func (s *splitter) notBefore(p token.Position) {
	s.fail(p.Offset, marker+" is not on the line right before a statement", "put it on the line right before the DROP statement")
}

// Normalize joins toks with single spaces, upper-casing words, for
// comparing SQL written differently: a(b) and A ( b ) are the same.
// Strings and quoted names are kept as written.
func Normalize(toks []Token) string {
	parts := make([]string, len(toks))
	for i, t := range toks {
		parts[i] = t.Text
		if t.Kind == Word {
			parts[i] = strings.ToUpper(t.Text)
		}
	}
	return strings.Join(parts, " ")
}
