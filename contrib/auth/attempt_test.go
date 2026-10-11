package auth

import "testing"

func TestClientIP(t *testing.T) {
	for addr, want := range map[string]string{
		"192.0.2.1:1234":             "192.0.2.1",
		"192.0.2.1":                  "192.0.2.1",
		"[::ffff:192.0.2.1]:80":      "192.0.2.1",
		"[2001:db8:1:2:3:4:5:6]:443": "2001:db8:1:2::/64",
		"[2001:db8:1:2:ffff::9]:443": "2001:db8:1:2::/64",
		"[2001:db8:1:3::1]:443":      "2001:db8:1:3::/64",
		"[fe80::1%en0]:80":           "fe80::/64",
		"2001:db8::1":                "2001:db8::/64",
		"not an address":             "not an address",
		"":                           "",
	} {
		if got := ClientIP(addr); got != want {
			t.Errorf("ClientIP(%q) = %q, want %q", addr, got, want)
		}
	}
}
