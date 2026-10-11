package auth

import (
	"net"
	"net/netip"
)

// ClientIP is who a request with remoteAddr comes from, for limiting
// attempts (D-128): the IP without the port and zone, an IPv4 address in
// IPv6 as IPv4, and an IPv6 address as its /64, the block one subscriber
// commonly gets, so that changing addresses inside it does not escape the
// limit. A remoteAddr it cannot read is itself.
func ClientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr // no port
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return remoteAddr
	}
	a = a.WithZone("").Unmap()
	if a.Is4() {
		return a.String()
	}
	return netip.PrefixFrom(a, 64).Masked().String()
}
