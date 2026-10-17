package admin

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/rt"
)

// The functions of this file are what admin_gen.go calls (D-131): what
// the generated forms need beside maago's Bind functions. Only generated
// code uses them.

// In is the input of f, an fk field's when fk is the model it refers to.
func In[T any](f *maago.Field[T], required bool, fk string) Input {
	in := Input{Name: f.Name, Label: f.Label, Help: f.Help, Raw: f.Raw, Error: f.Error, Choices: f.Choices, Required: required, FK: fk}
	switch any(f).(type) {
	case *maago.Field[bool]:
		in.Kind = "checkbox"
	case *maago.Field[time.Time]:
		in.Kind = "datetime" // text: <input type="datetime-local"> holds only milliseconds
	case *maago.Field[int64], *maago.Field[int32]:
		in.Kind = "integer"
	case *maago.Field[float64]:
		in.Kind = "number"
	default:
		in.Kind = "text"
		if f.Choices != nil {
			in.Kind = "select"
		}
	}
	return in
}

// Show is the input of a field the form does not change: its value as
// the list shows it.
func Show(label string, v any) Input {
	text, err := cell(v)
	if err != nil {
		panic(err) // the generated code passes a column's value
	}
	return Input{Label: label, Kind: "readonly", Raw: text}
}

// Raw is v as its input draws it: a time to the microsecond in
// Config.TimeZone, as the database keeps it (D-105), so that a field left
// alone is saved as it was; a bool as a checkbox sends it; a *bool as
// its select's choice; nil as an empty input.
func Raw(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case int64:
		return strconv.FormatInt(v, 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "on"
		}
		return ""
	case time.Time:
		s, _ := rt.DateFunc("2006-01-02T15:04:05.999999", v)
		return s
	case *string:
		return raw(v)
	case *int64:
		return raw(v)
	case *int32:
		return raw(v)
	case *float64:
		return raw(v)
	case *time.Time:
		return raw(v)
	case *bool:
		switch {
		case v == nil:
			return ""
		case *v:
			return "yes"
		}
		return "no"
	}
	panic(fmt.Sprintf("admin: Raw of a %T; this is a bug in maago", v))
}

func raw[T any](p *T) string {
	if p == nil {
		return ""
	}
	return Raw(*p)
}

// Ptr is f's value for a pointer field: nil when nothing was given.
func Ptr[T any](f maago.Field[T]) *T {
	if !f.Given {
		return nil
	}
	v := f.Value
	return &v
}

// Bool is a *bool field's value, read from its select of yes and no.
func Bool(f maago.Field[string]) *bool {
	if !f.Given {
		return nil
	}
	v := f.Value == "yes"
	return &v
}

// Exists reports whether query, SELECT 1 … WHERE "id" = ?, finds the row
// of id: an fk field's check.
func Exists(ctx context.Context, db maago.DB, query string, id int64) (bool, error) {
	var one int
	err := db.QueryRowContext(ctx, query, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// CountRefs is the Refs of the row of id: each of refs with its count,
// or sql.ErrNoRows when row (SELECT 1 … WHERE "id" = ?) finds no such row.
func CountRefs(ctx context.Context, db maago.DB, id int64, row string, refs []Ref) ([]Ref, error) {
	if found, err := Exists(ctx, db, row, id); err != nil || !found {
		return nil, cmp.Or(err, sql.ErrNoRows)
	}
	out := slices.Clone(refs)
	for i := range out {
		args := []any{id}
		if out[i].Self {
			args = append(args, id)
		}
		if err := db.QueryRowContext(ctx, out[i].SQL, args...).Scan(&out[i].N); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Like is the pattern of a search: text, trimmed, between %s with its
// \, % and _ escaped by \, or nil for no search (D-134).
func Like(text string) *string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	p := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(text) + "%"
	return &p
}

// Choice is a choice filter's value, or nil for none; a value that is
// not one of choices is a 400.
func Choice(v string, choices ...string) (*string, error) {
	switch {
	case v == "":
		return nil, nil
	case !slices.Contains(choices, v):
		return nil, maago.Status(http.StatusBadRequest, fmt.Errorf("admin: filter value %q is not one of %q", v, choices))
	}
	return &v, nil
}

// YesNo is a bool filter's value, yes or no, or nil for none.
func YesNo(v string) (*bool, error) {
	switch v {
	case "":
		return nil, nil
	case "yes", "no":
		b := v == "yes"
		return &b, nil
	}
	return nil, maago.Status(http.StatusBadRequest, fmt.Errorf("admin: filter value %q is not yes or no", v))
}

// ID is an fk filter's value, the id of a row, or nil for none.
func ID(v string) (*int64, error) {
	if v == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil, maago.Status(http.StatusBadRequest, fmt.Errorf("admin: filter value %q is not an id", v))
	}
	return &n, nil
}

// NoAdding is the error of the form of a new row of a model that has no
// adding (D-132): the add page is not there.
func NoAdding(model string) error {
	return maago.Status(http.StatusNotFound, fmt.Errorf("admin: %s has no adding", model))
}

// Updated is the error of an UPDATE of one row that changed n rows:
// sql.ErrNoRows when there was none.
func Updated(n int64, err error) error {
	if err == nil && n == 0 {
		return sql.ErrNoRows
	}
	return err
}

// Version is the version the form of a row carries, as _version (D-150):
// read, the row's, to draw; to save, the one the form was opened with,
// which a POST must send.
func Version(vals url.Values, read time.Time) (time.Time, error) {
	if vals == nil {
		return read, nil
	}
	n, err := strconv.ParseInt(vals.Get("_version"), 10, 64)
	if err != nil {
		return time.Time{}, maago.Status(http.StatusBadRequest, errors.New("admin: the form has no _version, the version of the row it was opened with"))
	}
	return time.UnixMicro(n), nil
}

// Changed is err of an UPDATE that saved only at the version the form
// was opened with: maago.ErrConflict when it changed no row and the row is
// there, query (SELECT 1 … WHERE "id" = ?) finding it.
func Changed(ctx context.Context, db maago.DB, query string, id int64, err error) error {
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if found, err := Exists(ctx, db, query, id); err != nil || !found {
		return cmp.Or(err, sql.ErrNoRows)
	}
	return maago.ErrConflict
}
