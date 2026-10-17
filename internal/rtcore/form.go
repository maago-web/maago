package rtcore

import (
	"cmp"
	"context"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// Messages has maago.Messages's fields, in order, so each converts to the
// other.
type Messages struct {
	Required, NotNumber, TooSmall, TooLarge, TooShort, TooLong string
	NotChoice, BadBool, BadTime, NoRow, NotKind                string
}

// Default are the settings of a context with none: maago.DefaultMessages
// and UTC. maago sets them as it starts.
var Default *Settings

// Settings are the Config values forms read, put in each request's
// context by Handler.
type Settings struct {
	Msgs Messages
	TZ   *time.Location
}

type settingsKey struct{}

// NewSettings fills what m leaves empty from d.
func NewSettings(m, d Messages, tz *time.Location) *Settings {
	m.Required, m.NotNumber, m.TooSmall = cmp.Or(m.Required, d.Required), cmp.Or(m.NotNumber, d.NotNumber), cmp.Or(m.TooSmall, d.TooSmall)
	m.TooLarge, m.TooShort, m.TooLong = cmp.Or(m.TooLarge, d.TooLarge), cmp.Or(m.TooShort, d.TooShort), cmp.Or(m.TooLong, d.TooLong)
	m.NotChoice, m.BadBool, m.BadTime = cmp.Or(m.NotChoice, d.NotChoice), cmp.Or(m.BadBool, d.BadBool), cmp.Or(m.BadTime, d.BadTime)
	m.NoRow, m.NotKind = cmp.Or(m.NoRow, d.NoRow), cmp.Or(m.NotKind, d.NotKind)
	return &Settings{m, cmp.Or(tz, time.UTC)}
}

// WithSettings is ctx with s, for the forms of its request.
func WithSettings(ctx context.Context, s *Settings) context.Context {
	return context.WithValue(ctx, settingsKey{}, s)
}

// SettingsFrom are ctx's, or the defaults: a view tested with WithDB alone
// reads its forms in English and UTC.
func SettingsFrom(ctx context.Context) *Settings {
	if s, ok := ctx.Value(settingsKey{}).(*Settings); ok {
		return s
	}
	return Default
}

// FormState is what binding found of a maago.Form, which holds it.
type FormState struct{ Bound, OK bool }

// FormStateOf is the state inside the *maago.Form form; maago sets it as
// it starts, since rtcore cannot name maago's types.
var FormStateOf func(form any) *FormState

// NewFile is the maago.File of a file a form sent; maago sets it as it
// starts.
var NewFile func(h *multipart.FileHeader, typ string) any

// FieldError is rt.FieldError.
type FieldError struct {
	Field  string `json:"field,omitempty"`
	Detail string `json:"detail"`
}

// invalidError is the errors of a form a 422 answers with.
type invalidError struct{ errs []FieldError }

func (e *invalidError) Error() string {
	var b strings.Builder
	for i, fe := range e.errs {
		if i > 0 {
			b.WriteString("; ")
		}
		if fe.Field != "" {
			b.WriteString(fe.Field + ": ")
		}
		b.WriteString(fe.Detail)
	}
	return b.String()
}

// Invalid is the 422 of errs.
func Invalid(errs ...FieldError) error {
	return Status(http.StatusUnprocessableEntity, &invalidError{errs})
}

// FieldErrors are all the form errors in err's tree, in order.
func FieldErrors(err error) []FieldError {
	switch e := err.(type) {
	case *invalidError:
		return e.errs
	case interface{ Unwrap() []error }:
		var errs []FieldError
		for _, x := range e.Unwrap() {
			errs = append(errs, FieldErrors(x)...)
		}
		return errs
	case interface{ Unwrap() error }:
		return FieldErrors(e.Unwrap())
	}
	return nil
}
