package rt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/internal/rtcore"
)

// FormValid reports whether f was sent and has no error: its fields read,
// Validate passed, and no form error was added. Only generated code calls
// it: maago gen gives each form a Valid that also counts the errors a view
// sets on its fields.
func FormValid(f maago.Form) bool {
	s := rtcore.FormStateOf(&f)
	return s.Bound && s.OK && len(f.Errors) == 0
}

// FinishForm records what binding found. Only generated code calls it.
func FinishForm(f *maago.Form, bound, fieldsOK bool) {
	*rtcore.FormStateOf(f) = rtcore.FormState{Bound: bound, OK: fieldsOK}
}

// Rule is what maago gen read from a field's tags (or its model's), for
// the Bind functions. Only generated code makes one.
type Rule struct {
	Name, Label, Help  string
	Choices            []string
	Optional           bool
	Min, Max           int      // the length of a string, in characters; 0: none
	MinValue, MaxValue string   // the bounds of a number, as the tag wrote them; "": none
	Accept             []string // the kinds of file a file field takes (FileKinds); none: any
}

// FormValues is the form r sent in its body, or nil for a GET or HEAD,
// which sends none: the form is drawn empty. A body that is not
// application/x-www-form-urlencoded, or does not parse, or sends one of
// the form's keys twice, is a 400; other keys are not the form's, and
// ignored. The query is not read: it is the URL's, not the form's.
func FormValues(r *http.Request, keys ...string) (url.Values, error) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return nil, nil
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/x-www-form-urlencoded" {
		return nil, maago.Status(http.StatusBadRequest, fmt.Errorf("form: Content-Type %q, not application/x-www-form-urlencoded", r.Header.Get("Content-Type")))
	}
	if err := r.ParseForm(); err != nil {
		return nil, maago.Status(http.StatusBadRequest, fmt.Errorf("form: %w", err))
	}
	for _, k := range keys {
		if vs := r.PostForm[k]; len(vs) > 1 {
			return nil, maago.Status(http.StatusBadRequest, fmt.Errorf("form key %q is given %d times", k, len(vs)))
		}
	}
	return r.PostForm, nil
}

// bind fills f's description from rule and, when the form was sent, its
// text; it reports whether there is text to read. Not given: an error
// when required, else the zero value.
func bind[T any](ctx context.Context, vals url.Values, f *maago.Field[T], rule Rule) bool {
	f.Name, f.Label, f.Help, f.Choices = rule.Name, rule.Label, rule.Help, rule.Choices
	if vals == nil {
		return false
	}
	f.Raw = vals.Get(rule.Name)
	f.Given = f.Raw != ""
	if !f.Given && !rule.Optional {
		f.Error = rtcore.SettingsFrom(ctx).Msgs.Required
	}
	return f.Given
}

// BindString reads a string field: its length and choices.
func BindString(ctx context.Context, vals url.Values, f *maago.Field[string], rule Rule) {
	if !bind(ctx, vals, f, rule) {
		return
	}
	m, n := rtcore.SettingsFrom(ctx).Msgs, utf8.RuneCountInString(f.Raw)
	switch {
	case rule.Min > 0 && n < rule.Min:
		f.Error = fmt.Sprintf(m.TooShort, rule.Min)
	case rule.Max > 0 && n > rule.Max:
		f.Error = fmt.Sprintf(m.TooLong, rule.Max)
	case rule.Choices != nil && !slices.Contains(rule.Choices, f.Raw):
		f.Error = m.NotChoice
	default:
		f.Value = f.Raw
	}
}

// BindInt64 reads an int64 field.
func BindInt64(ctx context.Context, vals url.Values, f *maago.Field[int64], rule Rule) {
	bindInt(ctx, vals, f, rule, 64)
}

// BindInt32 reads an int32 field.
func BindInt32(ctx context.Context, vals url.Values, f *maago.Field[int32], rule Rule) {
	bindInt(ctx, vals, f, rule, 32)
}

func bindInt[T int32 | int64](ctx context.Context, vals url.Values, f *maago.Field[T], rule Rule, bits int) {
	if !bind(ctx, vals, f, rule) {
		return
	}
	m := rtcore.SettingsFrom(ctx).Msgs
	v, err := strconv.ParseInt(f.Raw, 10, bits)
	if err != nil {
		f.Error = m.NotNumber
		if isRange(err) { // a number, past the type
			lo, hi := int64(-1)<<(bits-1), int64(1)<<(bits-1)-1
			if f.Raw[0] == '-' {
				f.Error = fmt.Sprintf(m.TooSmall, strconv.FormatInt(lo, 10))
			} else {
				f.Error = fmt.Sprintf(m.TooLarge, strconv.FormatInt(hi, 10))
			}
		}
		return
	}
	if lo, err := strconv.ParseInt(rule.MinValue, 10, 64); err == nil && v < lo {
		f.Error = fmt.Sprintf(m.TooSmall, rule.MinValue)
	} else if hi, err := strconv.ParseInt(rule.MaxValue, 10, 64); err == nil && v > hi {
		f.Error = fmt.Sprintf(m.TooLarge, rule.MaxValue)
	} else {
		f.Value = T(v)
	}
}

// BindFloat64 reads a float64 field: a finite number.
func BindFloat64(ctx context.Context, vals url.Values, f *maago.Field[float64], rule Rule) {
	if !bind(ctx, vals, f, rule) {
		return
	}
	m := rtcore.SettingsFrom(ctx).Msgs
	v, err := strconv.ParseFloat(f.Raw, 64)
	switch {
	case err != nil && !isRange(err), math.IsNaN(v), math.IsInf(v, 0) && err == nil: // "Inf" and "NaN" parse
		f.Error = m.NotNumber
	case math.IsInf(v, -1):
		f.Error = fmt.Sprintf(m.TooSmall, strconv.FormatFloat(-math.MaxFloat64, 'g', -1, 64))
	case math.IsInf(v, 1):
		f.Error = fmt.Sprintf(m.TooLarge, strconv.FormatFloat(math.MaxFloat64, 'g', -1, 64))
	default:
		if lo, err := strconv.ParseFloat(rule.MinValue, 64); err == nil && v < lo {
			f.Error = fmt.Sprintf(m.TooSmall, rule.MinValue)
		} else if hi, err := strconv.ParseFloat(rule.MaxValue, 64); err == nil && v > hi {
			f.Error = fmt.Sprintf(m.TooLarge, rule.MaxValue)
		} else {
			f.Value = v
		}
	}
}

func isRange(err error) bool {
	ne, ok := err.(*strconv.NumError)
	return ok && ne.Err == strconv.ErrRange
}

// BindBool reads a checkbox: not sent, "", "false", "off" or "0" is
// false; "on" (what a browser sends), "true" or "1" is true.
func BindBool(ctx context.Context, vals url.Values, f *maago.Field[bool], rule Rule) {
	rule.Optional = true // an unchecked box sends nothing
	if !bind(ctx, vals, f, rule) {
		return
	}
	switch f.Raw {
	case "on", "true", "1":
		f.Value = true
	case "false", "off", "0":
	default:
		f.Error = rtcore.SettingsFrom(ctx).Msgs.BadBool
	}
}

// BindTime reads a time field as an <input type="datetime-local"> sends
// it, 2006-01-02T15:04 or with seconds, which may have a fraction (time.Parse
// takes one after the seconds), in Config.TimeZone, or as RFC 3339 with its offset.
func BindTime(ctx context.Context, vals url.Values, f *maago.Field[time.Time], rule Rule) {
	if !bind(ctx, vals, f, rule) {
		return
	}
	s := rtcore.SettingsFrom(ctx)
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, f.Raw, s.TZ); err == nil {
			f.Value = t
			return
		}
	}
	if t, err := time.Parse(time.RFC3339Nano, f.Raw); err == nil { // what an API sends: its offset says where
		f.Value = t
		return
	}
	f.Error = s.Msgs.BadTime
}

// NoRow marks f, whose number is of no row, with Messages.NoRow. Only
// generated code calls it.
func NoRow[T any](ctx context.Context, f *maago.Field[T]) {
	f.Error = rtcore.SettingsFrom(ctx).Msgs.NoRow
}

// JSONValues is the form an API route's request sent: one JSON
// object, read as the text of each value. A string is its text, a number
// its digits as written, true and false themselves, null not given. A GET
// or HEAD sends none: nil. The body must be application/json: one object
// and nothing after it; a 400 otherwise. A body past Config.MaxBody is a
// 413. Of keys, the
// form's, none may come twice or hold an array or object; other keys are
// ignored.
func JSONValues(r *http.Request, keys ...string) (url.Values, error) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return nil, nil
	}
	bad := func(format string, args ...any) error {
		return maago.Status(http.StatusBadRequest, fmt.Errorf("json body: "+format, args...))
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		return nil, bad("Content-Type %q, not application/json", r.Header.Get("Content-Type"))
	}
	dec := json.NewDecoder(r.Body)
	if t, err := dec.Token(); err != nil {
		return nil, bad("not a JSON object: %w", err)
	} else if t != json.Delim('{') {
		return nil, bad("not a JSON object")
	}
	vals := url.Values{}
	seen := map[string]bool{}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, bad("%w", err)
		}
		key := t.(string) // a key, once the object is open
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, bad("%w", err)
		}
		if !slices.Contains(keys, key) {
			continue
		}
		if seen[key] {
			return nil, bad("key %q is given twice", key)
		}
		seen[key] = true
		switch raw[0] {
		case '{', '[':
			return nil, bad("key %q holds an array or object, not a value", key)
		case 'n': // null: not given
		case '"':
			var s string
			json.Unmarshal(raw, &s)
			vals.Set(key, s)
		default: // a number as written, or true or false
			vals.Set(key, string(raw))
		}
	}
	if _, err := dec.Token(); err != nil {
		return nil, bad("%w", err)
	}
	switch _, err := dec.Token(); {
	case err == io.EOF:
	case err != nil:
		return nil, bad("%w", err)
	default:
		return nil, bad("more after the object")
	}
	return vals, nil
}

// FieldError is one error of a 422 problem's errors member. Field
// is the form key, "" for the form as a whole.
type FieldError = rtcore.FieldError

// Unprocessable is the 422 of a form that did not pass: its fields'
// errors in order, the form's own after them.
func Unprocessable(fields []FieldError, form []string) error {
	errs := slices.DeleteFunc(fields, func(e FieldError) bool { return e.Detail == "" })
	for _, msg := range form {
		errs = append(errs, FieldError{Detail: msg})
	}
	return rtcore.Invalid(errs...)
}
