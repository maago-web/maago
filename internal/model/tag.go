// Package model reads Maago models: structs that embed maago.Model, with
// their options in maago:"…" struct tags (D-10, D-13, D-17).
package model

import (
	"fmt"
	"go/ast"
	"go/token"
	"regexp"
	"strings"

	"github.com/maago-web/maago/internal/diag"
)

// optionName is an option's name: lowercase words joined by _ (D-81).
var optionName = regexp.MustCompile(`^[a-z]+(_[a-z]+)*$`)

// Kind is the shape of an option's value.
type Kind int

const (
	NoValue Kind = iota // key
	Bare                // key=a, or key=a/b/c: one or more items split by "/"
	Text                // key='text'
	Group               // key=(a,b)
)

// Option is one key[=value] of a maago tag. Values and ValuePos are
// parallel. A Text has one value, each doubled quote in it already
// turned into one, and its position is that of the opening quote.
type Option struct {
	Key      string
	KeyPos   token.Pos
	Kind     Kind
	Values   []string
	ValuePos []token.Pos
}

// ParseTag returns the options in the maago:"…" part of a struct field's
// tag. It checks the syntax only; which keys and values are allowed is the
// caller's to check. It stops at the first error in the tag, since what
// follows it cannot be read reliably.
func ParseTag(fset *token.FileSet, lit *ast.BasicLit) ([]Option, []diag.Diagnostic) {
	fail := func(pos token.Pos, msg, fix string) ([]Option, []diag.Diagnostic) {
		return nil, []diag.Diagnostic{diag.New(fset.Position(pos), msg, fix)}
	}
	if lit == nil {
		return nil, nil
	}
	if lit.Kind != token.STRING || lit.Value[0] != '`' {
		return fail(lit.Pos(), "struct tag is not a raw string", "write the struct tag with backquotes")
	}
	tag := lit.Value[1 : len(lit.Value)-1]
	if strings.ContainsAny(tag, "\r\n") {
		return fail(lit.Pos(), "struct tag spans several lines", "write the struct tag on one line")
	}

	// The outer layer follows reflect.StructTag.Lookup, except that the
	// maago value is not unquoted (so its bytes map to columns) and a
	// second maago key is an error.
	var value string
	var valuePos token.Pos
	found := false
	afterMaago := -1 // just past the maago value's closing quote, if a string in it is left open
	for i := 0; i < len(tag); {
		for i < len(tag) && tag[i] == ' ' {
			i++
		}
		if i == len(tag) {
			break
		}
		start := i
		for i < len(tag) && tag[i] > ' ' && tag[i] != ':' && tag[i] != '"' && tag[i] != 0x7f {
			i++
		}
		if i == start || i+1 >= len(tag) || tag[i] != ':' || tag[i+1] != '"' {
			if start == afterMaago {
				return fail(pos(lit, start-1), `double quote in maago tag`, "remove it; maago tag values cannot hold a double quote")
			}
			if i == len(tag) {
				i = start // point at the stray part, not past its end
			}
			return fail(pos(lit, i), `struct tag is not in the form key:"value"`, `write each part as key:"value", separated by spaces`)
		}
		name := tag[start:i]
		i += 2 // past :"
		vstart := i
		for i < len(tag) && tag[i] != '"' {
			if tag[i] == '\\' {
				i++
			}
			i++
		}
		if i >= len(tag) {
			return fail(pos(lit, vstart-1), `struct tag value has no closing "`, `close the value with "`)
		}
		if name == "maago" {
			if found {
				return fail(pos(lit, start), "struct tag has two maago keys", "merge them into one maago:\"…\"")
			}
			found, value, valuePos = true, tag[vstart:i], pos(lit, vstart)
		}
		i++ // past the closing "
		if name == "maago" && strings.Count(value, "'")%2 == 1 {
			afterMaago = i // the " most likely sat inside a 'string'
		}
	}
	if !found {
		return nil, nil
	}
	if j := strings.IndexByte(value, '\\'); j >= 0 {
		return fail(valuePos+token.Pos(j), `backslash in maago tag`, "remove it; write text as 'text', which cannot hold a double quote")
	}
	p := &tagParser{s: value, base: valuePos}
	opts, err := p.options()
	if err != nil {
		return fail(err.pos, err.msg, err.fix)
	}
	return opts, nil
}

// pos is the position of byte i of the tag's content, just past the backquote.
func pos(lit *ast.BasicLit, i int) token.Pos { return lit.Pos() + 1 + token.Pos(i) }

type parseError struct {
	pos      token.Pos
	msg, fix string
}

type tagParser struct {
	s    string
	i    int
	base token.Pos
}

func (p *tagParser) at(i int) token.Pos { return p.base + token.Pos(i) }

func (p *tagParser) fail(i int, msg, fix string) *parseError {
	return &parseError{p.at(i), msg, fix}
}

// unexpected reports the byte at p.i, or a missing byte at the end.
func (p *tagParser) unexpected(want, fix string) *parseError {
	switch {
	case p.i == len(p.s):
		return p.fail(p.i, "maago tag ends where "+want+" should be", fix)
	case p.s[p.i] == ' ':
		return p.fail(p.i, "space in maago tag", "remove the space")
	}
	return p.fail(p.i, fmt.Sprintf("unexpected %q in maago tag, want %s", p.s[p.i], want), fix)
}

func (p *tagParser) options() ([]Option, *parseError) {
	var opts []Option
	for {
		opt, err := p.option()
		if err != nil {
			return nil, err
		}
		opts = append(opts, opt)
		if p.i == len(p.s) {
			return opts, nil
		}
		if p.s[p.i] != ',' {
			if opt.Kind == Bare {
				return nil, p.unexpected("a comma", "write a value holding = or ' as 'text'")
			}
			return nil, p.unexpected("a comma", "separate options with commas")
		}
		p.i++
	}
}

func (p *tagParser) option() (Option, *parseError) {
	start := p.i
	for p.i < len(p.s) && isAlnum(p.s[p.i]) {
		p.i++
	}
	if p.i == start {
		return Option{}, p.unexpected("an option name", "write options as key or key=value")
	}
	key := p.s[start:p.i]
	if !optionName.MatchString(key) {
		return Option{}, p.fail(start, fmt.Sprintf("option name %q is not lowercase words joined by _", key), "write the option name in lowercase letters, like max or min_value")
	}
	opt := Option{Key: key, KeyPos: p.at(start)}
	if p.i == len(p.s) || p.s[p.i] != '=' {
		return opt, nil
	}
	p.i++
	switch {
	case p.i < len(p.s) && p.s[p.i] == '\'':
		opt.Kind = Text
		return opt, p.str(&opt)
	case p.i < len(p.s) && p.s[p.i] == '(':
		opt.Kind = Group
		return opt, p.group(&opt)
	}
	opt.Kind = Bare
	for {
		if err := p.bare(&opt); err != nil {
			return opt, err
		}
		if p.i == len(p.s) || p.s[p.i] != '/' {
			return opt, nil
		}
		p.i++
	}
}

func (p *tagParser) str(opt *Option) *parseError {
	open := p.i
	p.i++
	var b strings.Builder
	for {
		j := strings.IndexByte(p.s[p.i:], '\'')
		if j < 0 {
			return p.fail(open, "string in maago tag has no closing '", "close the string with '")
		}
		b.WriteString(p.s[p.i : p.i+j])
		p.i += j + 1
		if p.i < len(p.s) && p.s[p.i] == '\'' { // '' is a quote
			b.WriteByte('\'')
			p.i++
			continue
		}
		opt.Values = append(opt.Values, b.String())
		opt.ValuePos = append(opt.ValuePos, p.at(open))
		return nil
	}
}

func (p *tagParser) group(opt *Option) *parseError {
	open := p.i
	p.i++
	for {
		if err := p.bare(opt); err != nil {
			return err
		}
		if p.i == len(p.s) {
			return p.fail(open, "group in maago tag has no closing )", "close the group with )")
		}
		switch p.s[p.i] {
		case ')':
			p.i++
			return nil
		case ',':
			p.i++
		default:
			return p.unexpected("a comma or )", "write a group as (a,b)")
		}
	}
}

func isAlnum(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '_'
}

// bare reads one item: a value that is not a string or a group.
func (p *tagParser) bare(opt *Option) *parseError {
	start := p.i
	for p.i < len(p.s) && !strings.ContainsRune(",()'/= \"\\", rune(p.s[p.i])) {
		p.i++
	}
	if p.i == start {
		return p.unexpected("a value", "write a value, or 'text' for text with spaces or commas")
	}
	opt.Values = append(opt.Values, p.s[start:p.i])
	opt.ValuePos = append(opt.ValuePos, p.at(start))
	return nil
}
