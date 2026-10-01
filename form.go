package maago

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Form is embedded in every form struct (D-119): the errors of the form
// as a whole, and whether the request filled it in. A form's fields are
// Field values; maago gen writes the code that reads them.
type Form struct {
	Errors []string // the form's own errors, beside its fields'
	bound  bool
	ok     bool
}

// AddError adds an error of the form as a whole, such as one Validate or
// the view finds: "the period overlaps another notice".
func (f *Form) AddError(msg string) { f.Errors = append(f.Errors, msg) }

// Bound reports whether the request sent the form: not a GET or HEAD.
func (f Form) Bound() bool { return f.bound }

// Valid reports whether the form was sent and has no error: its fields
// read, Validate passed, and no form error was added.
func (f Form) Valid() bool { return f.bound && f.ok && len(f.Errors) == 0 }

// FinishForm records what binding found. Only generated code calls it.
func FinishForm(f *Form, bound, fieldsOK bool) { f.bound, f.ok = bound, fieldsOK }

// Field is one field of a form: what was sent, what it means, and what is
// wrong with it, with what a template needs to draw it (D-119).
type Field[T any] struct {
	Value T      // the value read; the zero value when not given or wrong
	Raw   string // the text sent, to draw back into the input
	Given bool   // the text sent is not empty
	Error string // why the text is not a valid value; "" when it is

	Name    string // the form key: the field's name in snake_case
	Label   string
	Help    string
	Choices []string
}

// Rule is what maago gen read from a field's tags (or its model's), for
// the Bind functions. Only generated code makes one.
type Rule struct {
	Name, Label, Help  string
	Choices            []string
	Optional           bool
	Min, Max           int    // the length of a string, in characters; 0: none
	MinValue, MaxValue string // the bounds of a number, as the tag wrote them; "": none
}

// Messages are the errors forms show to the people using the site. A
// project replaces them in Config.Messages; an empty one keeps the
// default. Each %s or %d is filled as its default shows.
type Messages struct {
	Required  string // the field is empty
	NotNumber string // the text is not a number
	TooSmall  string // %s: the smallest number allowed
	TooLarge  string // %s: the largest number allowed
	TooShort  string // %d: the fewest characters allowed
	TooLong   string // %d: the most characters allowed
	NotChoice string // the text is not one of the choices
	BadBool   string // a checkbox sent what it cannot send
	BadTime   string // the text is not a date and time
}

// DefaultMessages are the messages a project has not replaced.
var DefaultMessages = Messages{
	Required:  "This field is required.",
	NotNumber: "Enter a number.",
	TooSmall:  "Enter %s or more.",
	TooLarge:  "Enter %s or less.",
	TooShort:  "Enter at least %d characters.",
	TooLong:   "Enter at most %d characters.",
	NotChoice: "Select one of the choices.",
	BadBool:   "Check the box or leave it.",
	BadTime:   "Enter a date and time.",
}

// formSettings are the Config values forms read, put in each request's
// context by Handler.
type formSettings struct {
	msgs Messages
	tz   *time.Location
}

type formKey struct{}

func newFormSettings(m Messages, tz *time.Location) *formSettings {
	d := DefaultMessages
	m.Required, m.NotNumber, m.TooSmall = cmp.Or(m.Required, d.Required), cmp.Or(m.NotNumber, d.NotNumber), cmp.Or(m.TooSmall, d.TooSmall)
	m.TooLarge, m.TooShort, m.TooLong = cmp.Or(m.TooLarge, d.TooLarge), cmp.Or(m.TooShort, d.TooShort), cmp.Or(m.TooLong, d.TooLong)
	m.NotChoice, m.BadBool, m.BadTime = cmp.Or(m.NotChoice, d.NotChoice), cmp.Or(m.BadBool, d.BadBool), cmp.Or(m.BadTime, d.BadTime)
	if tz == nil {
		tz = time.UTC
	}
	return &formSettings{m, tz}
}

var defaultSettings = newFormSettings(Messages{}, nil)

// settings are ctx's, or the defaults: a view tested with WithDB alone
// reads its forms in English and UTC.
func settings(ctx context.Context) *formSettings {
	if s, ok := ctx.Value(formKey{}).(*formSettings); ok {
		return s
	}
	return defaultSettings
}

// FormValues is the form r sent in its body, or nil for a GET or HEAD,
// which sends none: the form is drawn empty. A body that is not
// application/x-www-form-urlencoded, or does not parse, or sends one of
// the form's keys twice, is a 400; other keys are not the form's, and
// ignored (D-116). The query is not read: it is the URL's, not the form's.
func FormValues(r *http.Request, keys ...string) (url.Values, error) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return nil, nil
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/x-www-form-urlencoded" {
		return nil, Status(http.StatusBadRequest, fmt.Errorf("form: Content-Type %q, not application/x-www-form-urlencoded", r.Header.Get("Content-Type")))
	}
	if err := r.ParseForm(); err != nil {
		return nil, Status(http.StatusBadRequest, fmt.Errorf("form: %w", err))
	}
	for _, k := range keys {
		if vs := r.PostForm[k]; len(vs) > 1 {
			return nil, Status(http.StatusBadRequest, fmt.Errorf("form key %q is given %d times", k, len(vs)))
		}
	}
	return r.PostForm, nil
}

// bind fills f's description from rule and, when the form was sent, its
// text; it reports whether there is text to read. Not given: an error
// when required, else the zero value.
func bind[T any](ctx context.Context, vals url.Values, f *Field[T], rule Rule) bool {
	f.Name, f.Label, f.Help, f.Choices = rule.Name, rule.Label, rule.Help, rule.Choices
	if vals == nil {
		return false
	}
	f.Raw = vals.Get(rule.Name)
	f.Given = f.Raw != ""
	if !f.Given && !rule.Optional {
		f.Error = settings(ctx).msgs.Required
	}
	return f.Given
}

// BindString reads a string field: its length and choices.
func BindString(ctx context.Context, vals url.Values, f *Field[string], rule Rule) {
	if !bind(ctx, vals, f, rule) {
		return
	}
	m, n := settings(ctx).msgs, utf8.RuneCountInString(f.Raw)
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
func BindInt64(ctx context.Context, vals url.Values, f *Field[int64], rule Rule) {
	bindInt(ctx, vals, f, rule, 64)
}

// BindInt32 reads an int32 field.
func BindInt32(ctx context.Context, vals url.Values, f *Field[int32], rule Rule) {
	bindInt(ctx, vals, f, rule, 32)
}

func bindInt[T int32 | int64](ctx context.Context, vals url.Values, f *Field[T], rule Rule, bits int) {
	if !bind(ctx, vals, f, rule) {
		return
	}
	m := settings(ctx).msgs
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
func BindFloat64(ctx context.Context, vals url.Values, f *Field[float64], rule Rule) {
	if !bind(ctx, vals, f, rule) {
		return
	}
	m := settings(ctx).msgs
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
func BindBool(ctx context.Context, vals url.Values, f *Field[bool], rule Rule) {
	rule.Optional = true // an unchecked box sends nothing
	if !bind(ctx, vals, f, rule) {
		return
	}
	switch f.Raw {
	case "on", "true", "1":
		f.Value = true
	case "false", "off", "0":
	default:
		f.Error = settings(ctx).msgs.BadBool
	}
}

// BindTime reads a time field as an <input type="datetime-local"> sends
// it, 2006-01-02T15:04 or with seconds, in Config.TimeZone.
func BindTime(ctx context.Context, vals url.Values, f *Field[time.Time], rule Rule) {
	if !bind(ctx, vals, f, rule) {
		return
	}
	s := settings(ctx)
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, f.Raw, s.tz); err == nil {
			f.Value = t
			return
		}
	}
	f.Error = s.msgs.BadTime
}

// check reports a message whose verbs do not fit what fills it, which
// would show people %!s(int=10).
func (m Messages) check() error {
	for _, c := range []struct {
		name, msg string
		args      []any
	}{
		{"Required", m.Required, nil}, {"NotNumber", m.NotNumber, nil}, {"NotChoice", m.NotChoice, nil},
		{"BadBool", m.BadBool, nil}, {"BadTime", m.BadTime, nil},
		{"TooSmall", m.TooSmall, []any{"1"}}, {"TooLarge", m.TooLarge, []any{"1"}},
		{"TooShort", m.TooShort, []any{1}}, {"TooLong", m.TooLong, []any{1}},
	} {
		if strings.Contains(fmt.Sprintf(c.msg, c.args...), "%!") {
			return fmt.Errorf("maago: Config.Messages.%s %q does not fit its value: use the verbs of DefaultMessages.%s", c.name, c.msg, c.name)
		}
	}
	return nil
}
