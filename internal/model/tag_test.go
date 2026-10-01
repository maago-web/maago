package model

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/maago-web/maago/internal/diag"
	"github.com/maago-web/maago/internal/golden"
)

// src holds one field per case. Its tags are written as they would be in
// a models.go file; the golden file shows what ParseTag makes of each.
const src = "package blog\n\ntype T struct {\n" +
	// Valid: every form in the symbol table of DECISIONS.
	"	A int `maago:\"unique\"`\n" +
	"	B int `maago:\"max=200,unique,index\"`\n" +
	"	C int `maago:\"label='제목',help='a, b, and c'\"`\n" +
	"	D int `maago:\"default='',label='''',help='it''s'\"`\n" +
	"	E int `maago:\"choices=draft/published/archived\"`\n" +
	"	F int `maago:\"choices=draft\"`\n" +
	"	G int `maago:\"unique=(AuthorID,Title),unique=(Slug)\"`\n" +
	"	H int `maago:\"ordering=-PublishedAt,default=-1,default=1.5\"`\n" +
	"	I int `json:\"i,omitempty\" maago:\"fk=Post\"`\n" +
	"	J int `maago:\"label='한글',max=10\"`\n" +
	"	K int `json:\"k\"`\n" +
	// Invalid: one error each, at its exact column.
	"	L int `maago:\"label='unclosed\"`\n" +
	"	M int `maago:\"max=1,,unique\"`\n" +
	"	N int `maago:\"max=1,\"`\n" +
	"	O int `maago:\"unique=(A,B\"`\n" +
	"	P int `maago:\"unique=(A,,B)\"`\n" +
	"	Q int `maago:\"max=\"`\n" +
	"	R int `maago:\"Max=1\"`\n" +
	"	S int `maago:\"max=1, unique\"`\n" +
	"	T int `maago:\"label=\\\\n\"`\n" +
	"	U int `maago:\"label='say \"hi\"'\"`\n" +
	"	V int \"maago:\\\"max=1\\\"\"\n" +
	"	W int `maago:\"max=1\"\n        json:\"w\"`\n" +
	"	X int `maago:\"max=1\" maago:\"unique\"`\n" +
	"	Y int `maago:max=1`\n" +
	"	Z int `maago:\"\"`\n" +
	"	AB int `maago:\"maxLen=1\"`\n" +
	"	AC int `maago:\"max1=1\"`\n" +
	"	AD int `maago:\"default=a=b\"`\n" +
	"	AE int `maago:\"label=a'b'\"`\n" +
	"	AF int `maago:\"max=1`\n" +
	"	AG int `maago:\"max=1\"foo`\n" +
	// Two errors in one tag: only the first is reported.
	"	AA int `maago:\"max=,,\"`\n" +
	// Option names are lowercase words joined by _ (D-81).
	"	AH int `maago:\"min_value=1\"`\n" +
	"	AI int `maago:\"min__value=1\"`\n" +
	"	AJ int `maago:\"_min=1\"`\n" +
	"	AK int `maago:\"min_=1\"`\n" +
	"}\n"

func TestParseTag(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "blog/models.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	var all []diag.Diagnostic
	if opts, ds := ParseTag(fset, nil); opts != nil || ds != nil {
		t.Errorf("ParseTag(nil) = %v, %v; want nothing", opts, ds)
	}
	for _, field := range f.Decls[0].(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Type.(*ast.StructType).Fields.List {
		opts, ds := ParseTag(fset, field.Tag)
		fmt.Fprintf(&out, "%s %s\n", field.Names[0].Name, field.Tag.Value)
		for _, o := range opts {
			fmt.Fprintf(&out, "  %s@%s %s", o.Key, col(fset, o.KeyPos), [...]string{"none", "bare", "string", "group"}[o.Kind])
			for i, v := range o.Values {
				fmt.Fprintf(&out, " %q@%s", v, col(fset, o.ValuePos[i]))
			}
			out.WriteString("\n")
		}
		for _, d := range ds {
			var b strings.Builder
			diag.Write(&b, []diag.Diagnostic{d})
			out.WriteString("  " + strings.ReplaceAll(strings.TrimSuffix(b.String(), "\n"), "\n", "\n  ") + "\n")
		}
		all = append(all, ds...)
	}
	golden.Check(t, "testdata/tag.golden", []byte(out.String()))

	if len(all) != 25 {
		t.Errorf("want 25 diagnostics, one per invalid field (L to AG, AI to AK), got %d", len(all))
	}
}

func col(fset *token.FileSet, p token.Pos) string {
	pos := fset.Position(p)
	return fmt.Sprintf("%d:%d", pos.Line, pos.Column)
}
