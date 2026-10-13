package admin

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/maago-web/maago"
)

// The helpers of the generated WHERE (D-134): the search's pattern, and
// each filter's value, nil for none.
func TestFilterValues(t *testing.T) {
	for in, want := range map[string]string{"": "nil", "  ": "nil", " a ": "%a%", `a%b_c\d`: `%a\%b\_c\\d%`} {
		got := "nil"
		if p := Like(in); p != nil {
			got = *p
		}
		if got != want {
			t.Errorf("Like(%q) = %s, want %s", in, got, want)
		}
	}
	if v, err := Choice("draft", "draft", "published"); err != nil || *v != "draft" {
		t.Errorf("Choice: %v %v", v, err)
	}
	if v, err := YesNo("no"); err != nil || *v {
		t.Errorf("YesNo: %v %v", v, err)
	}
	if v, err := ID("7"); err != nil || *v != 7 {
		t.Errorf("ID: %v %v", v, err)
	}
	if c, b, i := must(Choice("", "draft")), must(YesNo("")), must(ID("")); c != nil || b != nil || i != nil {
		t.Errorf("empty: %v %v %v", c, b, i)
	}
	for _, err := range []error{second(Choice("nope", "draft")), second(YesNo("maybe")), second(ID("x")), second(ID("1.5"))} {
		if err == nil || !strings.Contains(err.Error(), "filter value") {
			t.Errorf("bad value: %v", err)
		}
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func second[T any](_ T, err error) error { return err }

// The list reads the search and the filters from the URL, draws the box
// and the links, and keeps the conditions on the pages' links but not on
// a filter's (D-134).
func TestSearch(t *testing.T) {
	var got []Query
	m := rows("blog", "Post", 120, []Field{{Name: "Title", Label: "제목"}, {Name: "Status", Label: "Status"}, {Name: "Public", Label: "Public"}, {Name: "AuthorID", Label: "Author"}},
		func(i int) []any { return []any{"t", "draft", true, int64(1)} }, nil)
	rowsOf := m.Rows
	m.Rows = func(ctx context.Context, db maago.DB, q Query) ([]Row, error) {
		got = append(got, q)
		return rowsOf(ctx, db, q)
	}
	m.Search = []string{"Title"}
	m.Filters = []Filter{{Key: "status", Field: "Status", Kind: "choice", Choices: []string{"draft", "published"}}, {Key: "public", Field: "Public", Kind: "bool"}, {Key: "author_id", Field: "AuthorID", Kind: "fk"}}
	h := handler(t, Config{Models: [][]Model{{m, plain("blog", "Tag")}}}, "", map[string][]string{"ed": {"*"}})

	w := get(h, "ed", "/admin/blog/Post/?q=a%26b&status=draft&author_id=3&page=2&foo=x")
	body := w.Body.String()
	if len(got) != 1 || got[0].Search != "a&b" || len(got[0].Filters) != 3 || got[0].Filters["status"] != "draft" || got[0].Filters["public"] != "" || got[0].Filters["author_id"] != "3" || got[0].Offset != 50 {
		t.Errorf("Rows got %+v", got)
	}
	for _, want := range []string{
		`<input type="search" name="q" value="a&amp;b" placeholder="Search 제목">`,
		`<label>Author <input type="number" name="author_id" value="3" step="1"></label>`,
		`<input type="hidden" name="status" value="draft">`,
		`Status: <a href="/admin/blog/Post/?author_id=3&amp;q=a%26b">All</a> <b>draft</b> <a href="/admin/blog/Post/?author_id=3&amp;q=a%26b&amp;status=published">published</a>`,
		`Public: <b>All</b> <a href="/admin/blog/Post/?author_id=3&amp;public=yes&amp;q=a%26b&amp;status=draft">yes</a>`,
		`<a href="/admin/blog/Post/?author_id=3&amp;page=3&amp;q=a%26b&amp;status=draft">Next</a>`,
		`<a href="/admin/blog/Post/?author_id=3&amp;page=1&amp;q=a%26b&amp;status=draft">Previous</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("list lacks %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, `name="public"`) || strings.Contains(body, `name="page"`) {
		t.Errorf("a filter with no value is kept in the search form:\n%s", body)
	}
	if w := get(h, "ed", "/admin/blog/Post/?status=draft&status=published"); w.Code != http.StatusBadRequest {
		t.Errorf("a filter twice: %d", w.Code)
	}
	if body := get(h, "ed", "/admin/blog/Tag/").Body.String(); strings.Contains(body, `class="search"`) {
		t.Errorf("a model without search or filters has the form:\n%s", body)
	}
	// Choices alone are links: no form with nothing to type in.
	m.Search, m.Filters = nil, m.Filters[:2]
	h = handler(t, Config{Models: [][]Model{{m}}}, "", map[string][]string{"ed": {"*"}})
	if body := get(h, "ed", "/admin/blog/Post/").Body.String(); strings.Contains(body, `class="search"`) || !strings.Contains(body, `class="filter"`) {
		t.Errorf("a model with choice filters only:\n%s", body)
	}
}
