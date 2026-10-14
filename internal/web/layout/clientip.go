package layout

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP is the rate limit key: IPv4 or an IPv6 /64. SECURITY: the trustedHops-th X-Forwarded-For entry from the right, else RemoteAddr.
func ClientIP(r *http.Request, trustedHops int) string {
	if trustedHops > 0 {
		entries := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
		if len(entries) >= trustedHops {
			if ip, ok := parseIP(strings.TrimSpace(entries[len(entries)-trustedHops])); ok {
				return ip
			}
		}
	}
	if ip, ok := parseIP(r.RemoteAddr); ok {
		return ip
	}
	return r.RemoteAddr
}

// NOTE: one IPv6 host usually holds a whole /64, so keying on the address alone lets it dodge a per-IP limit.
func parseIP(s string) (string, bool) {
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return "", false
	}
	addr = addr.WithZone("").Unmap()
	if addr.Is4() {
		return addr.String(), true
	}
	return netip.PrefixFrom(addr, 64).Masked().String(), true
}
