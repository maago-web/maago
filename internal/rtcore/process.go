package rtcore

import (
	"sync/atomic"
	"time"
)

// Process is what the last Handler set for the code that no request
// reaches (D-122): the generated URL functions and the template functions.
type Process struct {
	Prefix string
	TZ     *time.Location
	Static map[string]*StaticFiles // by package name, for {{static}}
}

// Current is the last Handler's.
var Current atomic.Pointer[Process]

// Prefix is maago.Prefix.
func Prefix() string {
	if p := Current.Load(); p != nil {
		return p.Prefix
	}
	return ""
}

// TimeZone is Config.TimeZone of the last Handler, UTC before any.
func TimeZone() *time.Location {
	if p := Current.Load(); p != nil {
		return p.TZ
	}
	return time.UTC
}
