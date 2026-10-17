package maago

import "github.com/maago-web/maago/internal/rtcore"

// Prefix is Config.Prefix of the last Handler made, "" before any: what
// the generated URL functions and {{url}} put before a route's path.
func Prefix() string { return rtcore.Prefix() }
