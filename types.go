package maago

import (
	"crypto/rand"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// UUID is a model field's type for a UUID: uuid in PostgreSQL, its
// lowercase 8-4-4-4-12 text in SQLite. It reads and writes that text in
// JSON, forms, paths and queries.
type UUID [16]byte

// NewUUID is a new version 7 UUID: the time in milliseconds, then random
// bits, so that UUIDs made later sort later.
func NewUUID() UUID {
	var u UUID
	rand.Read(u[6:]) // never fails
	ms := uint64(time.Now().UnixMilli())
	for i := range 6 {
		u[i] = byte(ms >> (40 - 8*i))
	}
	u[6] = u[6]&0x0f | 0x70 // version 7
	u[8] = u[8]&0x3f | 0x80 // variant 10
	return u
}

// ParseUUID reads s, 8-4-4-4-12 hexadecimal digits in either case.
func ParseUUID(s string) (UUID, error) {
	var u UUID
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return u, fmt.Errorf("maago: %q is not a UUID", s)
	}
	digits := s[:8] + s[9:13] + s[14:18] + s[19:23] + s[24:]
	if _, err := hex.Decode(u[:], []byte(digits)); err != nil {
		return UUID{}, fmt.Errorf("maago: %q is not a UUID", s)
	}
	return u, nil
}

// String is u as 8-4-4-4-12 lowercase hexadecimal digits.
func (u UUID) String() string {
	h := hex.EncodeToString(u[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func (u UUID) MarshalText() ([]byte, error) { return []byte(u.String()), nil }

func (u *UUID) UnmarshalText(b []byte) (err error) {
	*u, err = ParseUUID(string(b))
	return err
}

// Value is u as the database keeps it: its text.
func (u UUID) Value() (driver.Value, error) { return u.String(), nil }

// Scan reads the text of a uuid column.
func (u *UUID) Scan(src any) (err error) {
	switch src := src.(type) {
	case string:
		*u, err = ParseUUID(src)
	case []byte:
		*u, err = ParseUUID(string(src))
	default:
		err = fmt.Errorf("maago: cannot read a UUID from %T", src)
	}
	return err
}

// JSON is a model field's type for a value kept as JSON: jsonb in
// PostgreSQL, text that json_valid accepts in SQLite. V is the value; in
// JSON, a JSON field is V itself. T is a type of the model's package, or
// built from such types and Go's own.
type JSON[T any] struct{ V T }

// Value is j.V as JSON text.
func (j JSON[T]) Value() (driver.Value, error) {
	b, err := json.Marshal(j.V)
	return string(b), err
}

// Scan reads JSON text into j.V.
func (j *JSON[T]) Scan(src any) error {
	switch src := src.(type) {
	case string:
		return json.Unmarshal([]byte(src), &j.V)
	case []byte:
		return json.Unmarshal(src, &j.V)
	}
	return fmt.Errorf("maago: cannot read JSON from %T", src)
}

func (j JSON[T]) MarshalJSON() ([]byte, error) { return json.Marshal(j.V) }

func (j *JSON[T]) UnmarshalJSON(b []byte) error { return json.Unmarshal(b, &j.V) }

// String is j.V as JSON text, the same whichever database read it; "" for
// a nil j.
func (j *JSON[T]) String() string {
	if j == nil {
		return ""
	}
	b, err := json.Marshal(j.V)
	if err != nil {
		return err.Error()
	}
	return string(b)
}
