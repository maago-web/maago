package rt

import (
	"testing"
	"time"

	"github.com/maago-web/maago/internal/rtcore"
)

// {{url}} and {{date}} read the last Handler's Config.Prefix and
// Config.TimeZone.
func TestTemplateFuncs(t *testing.T) {
	t.Cleanup(func() { rtcore.Current.Store(nil) })
	seoul := time.FixedZone("KST", 9*60*60)
	rtcore.Current.Store(&rtcore.Process{Prefix: "/app", TZ: seoul})
	u, err := URLFunc(map[string]string{"post": "/posts/{id}", "home": "/{$}"})("post", 7)
	if home, _ := URLFunc(map[string]string{"home": "/{$}"})("home"); err != nil || u != "/app/posts/7" || home != "/app/" {
		t.Errorf("url: %q %q %v", u, home, err)
	}
	at := time.Date(2026, 10, 1, 0, 30, 0, 0, time.UTC)
	for _, c := range []struct {
		t    any
		want string
	}{{at, "2026-10-01 09:30"}, {&at, "2026-10-01 09:30"}, {(*time.Time)(nil), ""}, {time.Time{}, ""}} {
		if got, err := DateFunc("2006-01-02 15:04", c.t); err != nil || got != c.want {
			t.Errorf("date %v: %q %v", c.t, got, err)
		}
	}
	if _, err := DateFunc("2006", 3); err == nil {
		t.Error("date of an int")
	}

	rtcore.Current.Store(nil)
	if got, _ := DateFunc("15:04", at); got != "00:30" {
		t.Errorf("before any Handler, UTC: %q", got)
	}
}
