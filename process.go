package maago

import (
	"fmt"
	"sync/atomic"
	"time"
)

// process is what the last Handler set for the code that no request
// reaches (D-122): the generated URL functions and the template functions.
type process struct {
	prefix string
	tz     *time.Location
}

var current atomic.Pointer[process]

// Prefix is Config.Prefix of the last Handler made, "" before any: what
// the generated URL functions and {{url}} put before a route's path.
func Prefix() string {
	if p := current.Load(); p != nil {
		return p.prefix
	}
	return ""
}

func timeZone() *time.Location {
	if p := current.Load(); p != nil {
		return p.tz
	}
	return time.UTC
}

// DateFunc is the date function of templates (D-122): {{date "2006-01-02
// 15:04" .CreatedAt}}, or {{.CreatedAt | date "2006-01-02"}}, shows a
// time.Time or *time.Time in Config.TimeZone of the last Handler made (UTC
// before any); a nil or zero time shows nothing. maago gen checks that the layout is a string literal.
func DateFunc(layout string, t any) (string, error) {
	switch v := t.(type) {
	case time.Time:
		if v.IsZero() {
			return "", nil
		}
		return v.In(timeZone()).Format(layout), nil
	case *time.Time:
		if v == nil {
			return "", nil
		}
		return DateFunc(layout, *v)
	}
	return "", fmt.Errorf("date: %T is not a time", t)
}
