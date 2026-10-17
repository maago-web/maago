// Package rtcore is the state the runtime package maago and the package
// rt, which generated code calls, share. Neither imports the other's
// unexported names; both read and write these.
package rtcore

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
)

// ErrConflict is maago.ErrConflict.
var ErrConflict = errors.New("maago: the row changed since it was read; read it again and apply the change, or change one column with a query that sets the version too")

// Status is maago.Status.
func Status(code int, err error) error {
	if code < 400 || code > 599 {
		panic(fmt.Sprintf("maago: Status(%d): the code must be from 400 to 599", code))
	}
	if err == nil {
		panic("maago: Status with a nil error")
	}
	return &statusError{code, err}
}

type statusError struct {
	code int
	err  error
}

func (e *statusError) Error() string { return e.err.Error() }
func (e *statusError) Unwrap() error { return e.err }

// Code is the response's code for err: a body past Config.MaxBody first,
// even inside a Status (a form's 400 that could not read it); then a
// Status, then sql.ErrNoRows, which is a row that is not there; else 500.
func Code(err error) int {
	var s *statusError
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.As(err, &s):
		return s.code
	case errors.Is(err, sql.ErrNoRows):
		return http.StatusNotFound
	case errors.Is(err, ErrConflict):
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}
