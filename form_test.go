package maago

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// The completion criterion of M4: an empty number, a number past its
// range and text that is no number give three different results.
func TestBindNumberThree(t *testing.T) {
	rule := Rule{Name: "n", Label: "N"}
	got := map[string]Field[int64]{}
	for _, raw := range []string{"", "99999999999999999999", "abc", "-99999999999999999999", "42"} {
		var f Field[int64]
		BindInt64(context.Background(), url.Values{"n": {raw}}, &f, rule)
		got[raw] = f
	}
	want := map[string]Field[int64]{
		"":                      {Raw: "", Given: false, Error: "This field is required."},
		"99999999999999999999":  {Raw: "99999999999999999999", Given: true, Error: "Enter 9223372036854775807 or less."},
		"abc":                   {Raw: "abc", Given: true, Error: "Enter a number."},
		"-99999999999999999999": {Raw: "-99999999999999999999", Given: true, Error: "Enter -9223372036854775808 or more."},
		"42":                    {Value: 42, Raw: "42", Given: true},
	}
	for raw, w := range want {
		w.Name, w.Label = "n", "N"
		if g := got[raw]; fmt.Sprint(g) != fmt.Sprint(w) {
			t.Errorf("%q: got %+v, want %+v", raw, g, w)
		}
	}
}

func TestBind(t *testing.T) {
	ctx := context.Background()
	str := func(raw string, rule Rule) Field[string] {
		var f Field[string]
		BindString(ctx, url.Values{"s": {raw}}, &f, rule)
		return f
	}
	for _, c := range []struct {
		name  string
		f     Field[string]
		value string
		err   string
	}{
		{"absent is empty", func() Field[string] {
			var f Field[string]
			BindString(ctx, url.Values{}, &f, Rule{Name: "s"})
			return f
		}(), "", "This field is required."},
		{"optional empty", str("", Rule{Name: "s", Optional: true}), "", ""},
		{"spaces are text", str("  ", Rule{Name: "s"}), "  ", ""},
		{"too short", str("ab", Rule{Name: "s", Min: 3}), "", "Enter at least 3 characters."},
		{"too long in characters", str("가나다라", Rule{Name: "s", Max: 3}), "", "Enter at most 3 characters."},
		{"three characters", str("가나다", Rule{Name: "s", Max: 3}), "가나다", ""},
		{"not a choice", str("x", Rule{Name: "s", Choices: []string{"draft", "published"}}), "", "Select one of the choices."},
		{"a choice", str("draft", Rule{Name: "s", Choices: []string{"draft", "published"}}), "draft", ""},
	} {
		if c.f.Value != c.value || c.f.Error != c.err {
			t.Errorf("%s: got %q %q, want %q %q", c.name, c.f.Value, c.f.Error, c.value, c.err)
		}
	}

	var n32 Field[int32]
	BindInt32(ctx, url.Values{"n": {"2147483648"}}, &n32, Rule{Name: "n"})
	if n32.Error != "Enter 2147483647 or less." {
		t.Errorf("int32 past its range: %q", n32.Error)
	}
	for raw, want := range map[string]string{"5": "", "4": "Enter 5 or more.", "11": "Enter 10 or less."} {
		var f Field[int64]
		BindInt64(ctx, url.Values{"n": {raw}}, &f, Rule{Name: "n", MinValue: "5", MaxValue: "10"})
		if f.Error != want {
			t.Errorf("int64 %s in 5..10: %q, want %q", raw, f.Error, want)
		}
	}
	for raw, want := range map[string]string{"1.5": "", "abc": "Enter a number.", "NaN": "Enter a number.", "Inf": "Enter a number.",
		"1e999": "Enter 1.7976931348623157e+308 or less.", "0.5": "Enter 1 or more."} {
		var f Field[float64]
		BindFloat64(ctx, url.Values{"x": {raw}}, &f, Rule{Name: "x", MinValue: "1"})
		if f.Error != want {
			t.Errorf("float64 %s: %q, want %q", raw, f.Error, want)
		}
	}
	for raw, want := range map[string]string{"": "false", "on": "true", "true": "true", "1": "true", "off": "false", "0": "false", "yes": "error"} {
		var f Field[bool]
		BindBool(ctx, url.Values{"b": {raw}}, &f, Rule{Name: "b"})
		got := fmt.Sprint(f.Value)
		if f.Error != "" {
			got = "error"
		}
		if got != want {
			t.Errorf("checkbox %q: %s, want %s", raw, got, want)
		}
	}

	// GET draws the form: described, not read.
	var empty Field[string]
	BindString(ctx, nil, &empty, Rule{Name: "s", Label: "S", Help: "h", Choices: []string{"a"}})
	if empty.Error != "" || empty.Given || empty.Name != "s" || empty.Label != "S" || empty.Help != "h" || len(empty.Choices) != 1 {
		t.Errorf("unbound field: %+v", empty)
	}
}

func TestBindSettings(t *testing.T) {
	seoul := time.FixedZone("KST", 9*60*60)
	ctx := context.WithValue(context.Background(), formKey{}, newFormSettings(Messages{Required: "필수 항목입니다."}, seoul))
	var s Field[string]
	BindString(ctx, url.Values{}, &s, Rule{Name: "s"})
	var n Field[int64]
	BindInt64(ctx, url.Values{"n": {"x"}}, &n, Rule{Name: "n"})
	if s.Error != "필수 항목입니다." || n.Error != "Enter a number." {
		t.Errorf("replaced message %q, kept default %q", s.Error, n.Error)
	}
	for raw, want := range map[string]string{"2026-10-01T09:30": "2026-10-01T00:30:00Z", "2026-10-01T09:30:15": "2026-10-01T00:30:15Z",
		"2026-10-01T09:30:15.123456": "2026-10-01T00:30:15.123456Z", "2026-10-01": ""} {
		var f Field[time.Time]
		BindTime(ctx, url.Values{"t": {raw}}, &f, Rule{Name: "t"})
		got := ""
		if f.Error == "" {
			got = f.Value.UTC().Format(time.RFC3339Nano)
		}
		if got != want {
			t.Errorf("time %q in Seoul: %q (%q), want %q", raw, got, f.Error, want)
		}
	}
	var fk Field[int64]
	NoRow(ctx, &fk)
	NoRow(context.WithValue(context.Background(), formKey{}, newFormSettings(Messages{NoRow: "없는 행입니다."}, nil)), &n)
	if fk.Error != "Select a row that exists." || n.Error != "없는 행입니다." {
		t.Errorf("NoRow: %q, replaced %q", fk.Error, n.Error)
	}
	var utc Field[time.Time] // no settings: UTC
	BindTime(context.Background(), url.Values{"t": {"2026-10-01T09:30"}}, &utc, Rule{Name: "t"})
	if utc.Value.Location() != time.UTC || utc.Value.Hour() != 9 {
		t.Errorf("default time zone: %v", utc.Value)
	}
}

func TestFormValues(t *testing.T) {
	post := func(ct, body string) *http.Request {
		r := httptest.NewRequest("POST", "/x?name=fromquery", strings.NewReader(body))
		if ct != "" {
			r.Header.Set("Content-Type", ct)
		}
		return r
	}
	if v, err := FormValues(httptest.NewRequest("GET", "/x?name=a", nil)); v != nil || err != nil {
		t.Errorf("GET: %v %v", v, err)
	}
	v, err := FormValues(post("application/x-www-form-urlencoded; charset=utf-8", "title=a&tag=1&tag=2"), "title")
	if err != nil || v.Get("title") != "a" || v.Has("name") {
		t.Errorf("body only, not the query; another key twice is not the form's: %v %v", v, err)
	}
	for name, r := range map[string]*http.Request{
		"no Content-Type": post("", "title=a"),
		"text/plain":      post("text/plain", "title=a"),
		"multipart":       post("multipart/form-data; boundary=x", "--x--"),
		"twice":           post("application/x-www-form-urlencoded", "title=a&title=b"),
		"malformed":       post("application/x-www-form-urlencoded", "title=%zz"),
	} {
		var se *statusError
		if _, err := FormValues(r, "title"); !errors.As(err, &se) || se.code != 400 {
			t.Errorf("%s: %v, want a 400", name, err)
		}
	}
}

func TestFormValid(t *testing.T) {
	var f Form
	if FormValid(f) || f.Bound() {
		t.Error("an unbound form is not valid")
	}
	FinishForm(&f, true, true)
	if !FormValid(f) {
		t.Error("bound, fields fine: valid")
	}
	f.AddError("overlaps")
	if FormValid(f) || len(f.Errors) != 1 {
		t.Error("a form error makes it invalid")
	}
	FinishForm(&f, true, false)
	f.Errors = nil
	if FormValid(f) {
		t.Error("a field error makes it invalid")
	}
}

func TestBindOptionalNumber(t *testing.T) {
	var f Field[int64]
	BindInt64(context.Background(), url.Values{"n": {""}}, &f, Rule{Name: "n", Optional: true, MinValue: "5"})
	if f.Error != "" || f.Given || f.Value != 0 {
		t.Errorf("optional empty number: %+v", f)
	}
}

func TestMessagesCheck(t *testing.T) {
	if err := newFormSettings(Messages{TooLong: "%d자 이하로"}, nil).msgs.check(); err != nil {
		t.Errorf("a fitting message: %v", err)
	}
	for _, m := range []Messages{{TooLong: "at most %s"}, {Required: "%d required"}, {NoRow: "%s"}, {TooSmall: "%s and %s"}} {
		if err := newFormSettings(m, nil).msgs.check(); err == nil {
			t.Errorf("%+v: no error", m)
		}
	}
}

func TestJSONValues(t *testing.T) {
	keys := []string{"title", "count", "agree", "note"}
	read := func(ct, body string) (url.Values, error) {
		r := httptest.NewRequest("POST", "/x?title=fromquery", strings.NewReader(body))
		r.Header.Set("Content-Type", ct)
		return JSONValues(r, keys...)
	}
	const js = "application/json"
	v, err := read(js+"; charset=utf-8", `{"title":"a \"q\" é","count":9223372036854775807,"agree":true,"note":null,"other":[1,{"x":2}],"other":3}`)
	want := url.Values{"title": {`a "q" é`}, "count": {"9223372036854775807"}, "agree": {"true"}}
	if err != nil || fmt.Sprint(v) != fmt.Sprint(want) {
		t.Errorf("values: %v %v, want %v", v, err, want)
	}
	v, _ = read(js, `{"count":0}`)
	var zero Field[int64]
	BindInt64(context.Background(), v, &zero, Rule{Name: "count"})
	if !zero.Given || zero.Error != "" || zero.Value != 0 {
		t.Errorf("0 is given: %+v", zero)
	}
	for _, body := range []string{`{"count":12}`, `{"count":"12"}`} {
		v, _ := read(js, body)
		var f Field[int64]
		BindInt64(context.Background(), v, &f, Rule{Name: "count"})
		if f.Value != 12 || f.Error != "" {
			t.Errorf("%s: %+v", body, f)
		}
	}
	v, _ = read(js, `{"count":1e3,"title":""}`)
	var n Field[int64]
	BindInt64(context.Background(), v, &n, Rule{Name: "count"})
	var s Field[string]
	BindString(context.Background(), v, &s, Rule{Name: "title"})
	if n.Error != "Enter a number." || n.Raw != "1e3" || s.Error != "This field is required." {
		t.Errorf("1e3 is no int64, \"\" is not given: %+v %+v", n, s)
	}
	if v, err := JSONValues(httptest.NewRequest("GET", "/x", nil), keys...); v != nil || err != nil {
		t.Errorf("GET: %v %v", v, err)
	}
	for name, c := range map[string][2]string{
		"empty":            {js, ""},
		"spaces":           {js, "  "},
		"null":             {js, "null"},
		"array":            {js, "[]"},
		"string":           {js, `"x"`},
		"number":           {js, "1"},
		"broken":           {js, `{"title":`},
		"after the object": {js, `{}{}`},
		"words after":      {js, `{} x`},
		"array value":      {js, `{"title":["a"]}`},
		"object value":     {js, `{"title":{}}`},
		"a form key twice": {js, `{"title":"a","title":"b"}`},
		"form urlencoded":  {"application/x-www-form-urlencoded", "title=a"},
		"no Content-Type":  {"", `{}`},
	} {
		var se *statusError
		if _, err := read(c[0], c[1]); !errors.As(err, &se) || se.code != 400 {
			t.Errorf("%s: %v, want a 400", name, err)
		}
	}
}

func TestBindTimeRFC3339(t *testing.T) {
	var f Field[time.Time]
	BindTime(context.Background(), url.Values{"t": {"2026-10-01T09:30:00.123+09:00"}}, &f, Rule{Name: "t"})
	if f.Error != "" || !f.Value.Equal(time.Date(2026, 10, 1, 0, 30, 0, 123e6, time.UTC)) {
		t.Errorf("RFC 3339 with an offset: %+v", f)
	}
}
