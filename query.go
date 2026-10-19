package maago

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maago-web/maago/internal/rtcore"
)

// ErrManyRows is what a :one query returns when it finds more than one
// row: it does not pick the first silently.
var ErrManyRows = errors.New("maago: :one query returned more than one row; make it :many, or add LIMIT 1")

// ErrConflict is what a generated Update gives when the row's version
// (its auto=update field) is not the one the model holds: another
// request changed it since it was read. It answers 409.
var ErrConflict = rtcore.ErrConflict

// ErrEmptySlice is what a generated query returns for an empty slice
// parameter where the SQL needs one element or more: IN () is not SQL.
// The generated code wraps it with the query's and the parameter's names.
var ErrEmptySlice = errors.New("maago: empty slice, which IN (…) cannot take; check it before calling the query, or put its line under {name}")

// UniqueError is what a generated Insert or Update gives when a unique
// constraint refused its row: a value, or values together, that another
// row has. Fields are the model's fields of that constraint, nil when
// maago cannot tell which. A form's Fail shows it on the form; else it
// answers 500.
type UniqueError struct {
	Fields []string
	Err    error // the driver's
}

func (e *UniqueError) Error() string {
	over := "a constraint maago does not know"
	if e.Fields != nil {
		over = "unique constraint over " + strings.Join(e.Fields, ", ")
	}
	return fmt.Sprintf("maago: %s: %v; show it on the form with its Fail", over, e.Err)
}

func (e *UniqueError) Unwrap() error { return e.Err }
