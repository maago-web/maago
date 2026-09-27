package migrate

import (
	"testing"

	"github.com/maago-web/maago/internal/schema"
)

func TestName(t *testing.T) {
	col := func(name string) *schema.Column { return &schema.Column{Name: name, Type: "TEXT"} }
	post := &schema.Table{Name: "blog_post"}
	tag := &schema.Table{Name: "blog_tag"}
	odd := &schema.Table{Name: "Blog-Post"}
	idx := &schema.Index{Columns: []string{"body"}}
	tests := []struct {
		name    string
		cs      []Change
		renames []Rename
		want    string
	}{
		{"nothing", nil, nil, ""},
		{"create", []Change{{Kind: CreateTable, To: tag}}, nil, "create_blog_tag"},
		{"drop table", []Change{{Kind: DropTable, From: tag}}, nil, "drop_blog_tag"},
		{"add", []Change{{Kind: AddColumn, From: post, To: post, New: col("body")}}, nil, "add_blog_post_body"},
		// Its index follows from the column and is not counted.
		{"add with index", []Change{
			{Kind: AddColumn, From: post, To: post, New: col("body")},
			{Kind: AddIndex, From: post, To: post, Index: idx},
		}, nil, "add_blog_post_body"},
		{"drop", []Change{{Kind: DropColumn, From: post, To: post, Old: col("summary")}}, nil, "drop_blog_post_summary"},
		{"alter", []Change{{Kind: AlterNotNull, From: post, To: post, Old: col("body"), New: col("body")}}, nil, "alter_blog_post_body"},
		{"index", []Change{{Kind: AddIndex, From: post, To: post, Index: &schema.Index{Columns: []string{"title"}}}}, nil, "alter_blog_post"},
		{"one table", []Change{
			{Kind: AddColumn, From: post, To: post, New: col("body")},
			{Kind: DropColumn, From: post, To: post, Old: col("summary")},
		}, nil, "blog_post"},
		{"two tables", []Change{
			{Kind: DropColumn, From: post, To: post, Old: col("summary")},
			{Kind: CreateTable, To: tag},
		}, nil, "drop_blog_post_summary_and_more"},
		{"rename", nil, []Rename{{"blog_post", "text", "body"}}, "rename_blog_post_text"},
		{"rename and alter", []Change{{Kind: AlterNotNull, From: post, To: post, Old: col("body"), New: col("body")}},
			[]Rename{{"blog_post", "text", "body"}}, "blog_post"},
		{"rename and another table", []Change{{Kind: CreateTable, To: tag}},
			[]Rename{{"blog_post", "text", "body"}}, "rename_blog_post_text_and_more"},
		{"odd letters", []Change{{Kind: CreateTable, To: odd}}, nil, "create_blog_post"},
	}
	for _, tt := range tests {
		if got := Name(tt.cs, tt.renames); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestParseRename(t *testing.T) {
	if r, ok := ParseRename("blog_post.text:body"); !ok || r != (Rename{"blog_post", "text", "body"}) {
		t.Errorf("got %v %v", r, ok)
	}
	for _, s := range []string{"", "blog_post", "blog_post.text", "text:body", ".text:body", "blog_post.:body", "blog_post.text:", "a.b:c:d", "a.b:c.d", "a.b.c:d", "a..c:d"} {
		if _, ok := ParseRename(s); ok {
			t.Errorf("%q: accepted", s)
		}
	}
}
