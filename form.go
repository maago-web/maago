package maago

import (
	"fmt"
	"mime/multipart"
	"strings"

	"github.com/maago-web/maago/internal/rtcore"
)

// Form is embedded in every form struct: the errors of the form
// as a whole, and whether the request filled it in. A form's fields are
// Field values; maago gen writes the code that reads them, and the
// form's Valid method.
type Form struct {
	Errors []string // the form's own errors, beside its fields'
	state  rtcore.FormState
}

// AddError adds an error of the form as a whole, such as one Validate or
// the view finds: "the period overlaps another notice".
func (f *Form) AddError(msg string) { f.Errors = append(f.Errors, msg) }

// Bound reports whether the request sent the form: not a GET or HEAD.
func (f Form) Bound() bool { return f.state.Bound }

// Field is one field of a form: what was sent, what it means, and what is
// wrong with it, with what a template needs to draw it.
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
	NoRow     string // the number is of no row: an fk field in the admin
	NotKind   string // the file is not of a kind the field accepts
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
	NoRow:     "Select a row that exists.",
	NotKind:   "This kind of file is not allowed.",
}

// check reports a message whose verbs do not fit what fills it, which
// would show people %!s(int=10).
func (m Messages) check() error {
	for _, c := range []struct {
		name, msg string
		args      []any
	}{
		{"Required", m.Required, nil}, {"NotNumber", m.NotNumber, nil}, {"NotChoice", m.NotChoice, nil},
		{"BadBool", m.BadBool, nil}, {"BadTime", m.BadTime, nil}, {"NoRow", m.NoRow, nil}, {"NotKind", m.NotKind, nil},
		{"TooSmall", m.TooSmall, []any{"1"}}, {"TooLarge", m.TooLarge, []any{"1"}},
		{"TooShort", m.TooShort, []any{1}}, {"TooLong", m.TooLong, []any{1}},
	} {
		if strings.Contains(fmt.Sprintf(c.msg, c.args...), "%!") {
			return fmt.Errorf("maago: Config.Messages.%s %q does not fit its value: use the verbs of DefaultMessages.%s", c.name, c.msg, c.name)
		}
	}
	return nil
}

// Invalid is the error an API view returns for what it finds wrong with
// a field, such as a title the database already has: a 422 whose errors
// name the field. Join several with errors.Join.
func Invalid[T any](f *Field[T], msg string) error {
	return rtcore.Invalid(rtcore.FieldError{Field: f.Name, Detail: msg})
}

// InvalidForm is Invalid for the form as a whole.
func InvalidForm(msg string) error {
	return rtcore.Invalid(rtcore.FieldError{Detail: msg})
}

func init() {
	rtcore.Default = rtcore.NewSettings(rtcore.Messages{}, rtcore.Messages(DefaultMessages), nil)
	rtcore.FormStateOf = func(f any) *rtcore.FormState { return &f.(*Form).state }
	rtcore.NewFile = func(h *multipart.FileHeader, typ string) any {
		return File{Name: h.Filename, Size: h.Size, Type: typ, header: h}
	}
}
