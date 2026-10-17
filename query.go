package maago

import (
	"errors"

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
