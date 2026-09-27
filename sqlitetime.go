package maago

import (
	"database/sql"
	"fmt"
	"time"
)

// sqliteLayout is how SQLite stores a time.Time (D-105): UTC, a space, six
// digits of microseconds, no zone. Its text order is time order, and it
// compares right with datetime('now').
const sqliteLayout = "2006-01-02 15:04:05.000000"

// SQLiteTime is t as SQLite stores it (D-105).
func SQLiteTime(t time.Time) string { return t.UTC().Format(sqliteLayout) }

// ScanSQLiteTime reads a SQLite time into dst. NULL is an error; use
// ScanNullSQLiteTime for a column that may be NULL.
func ScanSQLiteTime(dst *time.Time) sql.Scanner {
	return scanner(func(src any) error {
		if src == nil {
			return fmt.Errorf("maago: NULL in a time column that is not a pointer")
		}
		t, err := parseSQLiteTime(src)
		*dst = t
		return err
	})
}

// ScanNullSQLiteTime reads a SQLite time into dst, nil for NULL.
func ScanNullSQLiteTime(dst **time.Time) sql.Scanner {
	return scanner(func(src any) error {
		if src == nil {
			*dst = nil
			return nil
		}
		t, err := parseSQLiteTime(src)
		*dst = &t
		return err
	})
}

type scanner func(any) error

func (s scanner) Scan(src any) error { return s(src) }

// parseSQLiteTime reads what SQLite gives: maago's own form, with any
// number of fraction digits, read as UTC, or RFC 3339 for times written
// by hand. The time is in UTC.
func parseSQLiteTime(src any) (time.Time, error) {
	var s string
	switch v := src.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		return time.Time{}, fmt.Errorf("maago: %T in a time column, want text (D-105)", src)
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999999", time.RFC3339Nano} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("maago: %q in a time column is not YYYY-MM-DD HH:MM:SS.ffffff or RFC 3339 (D-105)", s)
}
