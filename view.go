package maago

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// The functions of this file are what the generated adapters call to read
// a request into a view's In and to write its page (D-116). Only
// generated code uses them.

// ParseQuery reads r's query string: a malformed one is a 400, not pairs
// dropped silently.
func ParseQuery(r *http.Request) (url.Values, error) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, Status(http.StatusBadRequest, fmt.Errorf("query: %w", err))
	}
	return q, nil
}

// Query reads the query value key with parse, and whether it was there.
// A value given twice, or one parse rejects, is a 400; so is a missing
// one when required.
func Query[T any](q url.Values, key string, parse func(string) (T, error), required bool) (T, bool, error) {
	var zero T
	vs, ok := q[key]
	switch {
	case !ok && required:
		return zero, false, Status(http.StatusBadRequest, fmt.Errorf("query %q is missing", key))
	case !ok:
		return zero, false, nil
	case len(vs) > 1:
		return zero, false, Status(http.StatusBadRequest, fmt.Errorf("query %q is given %d times", key, len(vs)))
	}
	v, err := parse(vs[0])
	if err != nil {
		return zero, false, Status(http.StatusBadRequest, fmt.Errorf("query %q: %w", key, err))
	}
	return v, true, nil
}

// Path reads the path value name with parse. One parse rejects is a 404:
// no page has that address.
func Path[T any](r *http.Request, name string, parse func(string) (T, error)) (T, error) {
	v, err := parse(r.PathValue(name))
	if err != nil {
		var zero T
		return zero, Status(http.StatusNotFound, fmt.Errorf("path %q: %w", name, err))
	}
	return v, nil
}

// The parsers of Query and Path, one for each type a value may have.

func ParseString(s string) (string, error) { return s, nil }

func ParseInt64(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }

func ParseInt32(s string) (int32, error) {
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(n), nil
}

// ParseBool takes what strconv.ParseBool does; an empty value is an error.
func ParseBool(s string) (bool, error) { return strconv.ParseBool(s) }

// ParseFloat64 rejects NaN and the infinities, which no form means.
func ParseFloat64(s string) (float64, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err == nil && (math.IsNaN(f) || math.IsInf(f, 0)) {
		return 0, fmt.Errorf("%q is not a finite number", s)
	}
	return f, err
}

// PathSegment is s escaped as one segment of a path, for the generated
// URL functions. An empty s matches no {name} wildcard, so it panics.
func PathSegment(s string) string {
	if s == "" {
		panic("maago: empty path value; a {name} wildcard matches one non-empty segment")
	}
	return url.PathEscape(s)
}

// ParsePages parses each page's template set: templates/base.html, if fsys
// has it, and templates/PAGE. A set of its own for each page lets every
// page fill the same blocks of the layout. maago gen parses with it too.
func ParsePages(fsys fs.FS, funcs template.FuncMap, pages ...string) (map[string]*template.Template, error) {
	files := []string{}
	if _, err := fs.Stat(fsys, "templates/base.html"); err == nil {
		files = append(files, "templates/base.html")
	}
	sets := map[string]*template.Template{}
	for _, p := range pages {
		t, err := template.New(p).Funcs(funcs).ParseFS(fsys, append(slices.Clone(files), "templates/"+p)...)
		if err != nil {
			return nil, err
		}
		sets[p] = t
	}
	return sets, nil
}

// Pages is ParsePages for the generated code, which knows the files parse.
// It escapes each page once, as maago gen checked it: html/template
// escapes a template in place when first executed, so a block route
// served first would leave the block escaped for its own context only.
func Pages(fsys fs.FS, funcs template.FuncMap, pages ...string) map[string]*template.Template {
	sets, err := ParsePages(fsys, funcs, pages...)
	if err != nil {
		panic(err)
	}
	for page, t := range sets {
		t.ExecuteTemplate(io.Discard, entry(t, page), nil) // escaping ends before data is read: the error is nil data's
	}
	return sets
}

// entry is the template a page starts at: base.html if the set has it.
func entry(t *template.Template, page string) string {
	if t.Lookup("base.html") != nil {
		return "base.html"
	}
	return page
}

// Render executes page's set with data: the block if block is not "",
// else base.html if the set has it, else the page. It writes only if that
// worked: a failure is a 500 with nothing written (see AppRoute).
func Render(w http.ResponseWriter, pages map[string]*template.Template, page, block string, data any) error {
	t, name := pages[page], block
	if name == "" {
		name = entry(t, page)
	}
	var b bytes.Buffer
	if err := t.ExecuteTemplate(&b, name, data); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := w.Write(b.Bytes())
	return err
}

// URLFunc is the url function of templates: {{url "post_detail" .ID}} is
// the path of the route named post_detail. routes maps each route name of
// the project to its pattern. maago gen has checked the names and the
// arguments; a wrong one is an error here too.
func URLFunc(routes map[string]string) func(name string, args ...any) (string, error) {
	return func(name string, args ...any) (string, error) {
		pattern, ok := routes[name]
		if !ok {
			return "", fmt.Errorf("no route named %q", name)
		}
		var b strings.Builder
		n := 0
		for _, seg := range strings.Split(pattern, "/")[1:] {
			b.WriteByte('/')
			switch {
			case seg == "{$}":
			case strings.HasPrefix(seg, "{"):
				if n++; n > len(args) {
					continue // counted, for the error below
				}
				switch v := args[n-1].(type) {
				case string:
					if v == "" {
						return "", fmt.Errorf("route %s: empty value for %s", name, seg)
					}
					b.WriteString(url.PathEscape(v))
				case int, int32, int64:
					fmt.Fprint(&b, v)
				default:
					return "", fmt.Errorf("route %s: %s cannot be %T", name, seg, v)
				}
			default:
				b.WriteString(url.PathEscape(seg))
			}
		}
		if n != len(args) {
			return "", fmt.Errorf("route %s takes %d values, not %d", name, n, len(args))
		}
		return b.String(), nil
	}
}

// JSON writes v as the JSON answer of an API route (D-44), or fails with
// nothing written.
func JSON(w http.ResponseWriter, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return errors.Join(errors.New("maago: cannot write the JSON answer"), err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(append(b, '\n'))
	return err
}
