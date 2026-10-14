package layout

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientIP(t *testing.T) {
	cases := []struct {
		name   string
		remote string
		xff    []string
		hops   int
		want   string
	}{
		{"remote ipv4", "192.0.2.10:5150", nil, 0, "192.0.2.10"},
		{"xff ignored with no trusted hops", "192.0.2.10:5150", []string{"203.0.113.9"}, 0, "192.0.2.10"},
		{"negative hops read as none", "192.0.2.10:5150", []string{"203.0.113.9"}, -1, "192.0.2.10"},
		{"one hop: the last entry", "10.0.0.1:5150", []string{"203.0.113.9"}, 1, "203.0.113.9"},
		{"one hop: a spoofed entry before the proxy's is ignored", "10.0.0.1:5150", []string{"6.6.6.6, 203.0.113.9"}, 1, "203.0.113.9"},
		{"one hop: the last header when several", "10.0.0.1:5150", []string{"6.6.6.6", "203.0.113.9"}, 1, "203.0.113.9"},
		{"two hops: cdn then nginx", "10.0.0.1:5150", []string{"203.0.113.9, 198.51.100.7"}, 2, "203.0.113.9"},
		{"two hops: a spoofed entry is ignored", "10.0.0.1:5150", []string{"6.6.6.6, 203.0.113.9, 198.51.100.7"}, 2, "203.0.113.9"},
		{"two hops across headers", "10.0.0.1:5150", []string{"6.6.6.6, 203.0.113.9", "198.51.100.7"}, 2, "203.0.113.9"},
		{"fewer entries than hops falls back to remote, never the left-most", "10.0.0.1:5150", []string{"6.6.6.6"}, 2, "10.0.0.1"},
		{"spaces around the entry", "10.0.0.1:5150", []string{"6.6.6.6 ,  203.0.113.9 "}, 1, "203.0.113.9"},
		{"junk entry falls back, never to an earlier one", "10.0.0.1:5150", []string{"6.6.6.6, junk"}, 1, "10.0.0.1"},
		{"trusted but no xff", "10.0.0.1:5150", nil, 1, "10.0.0.1"},
		{"trusted but junk xff", "10.0.0.1:5150", []string{"not-an-ip"}, 1, "10.0.0.1"},
		{"xff with a port", "10.0.0.1:5150", []string{"203.0.113.9:4711"}, 1, "203.0.113.9"},
		{"ipv6 to its /64", "[2001:db8:0:1:aaaa::1]:5150", nil, 0, "2001:db8:0:1::/64"},
		{"ipv6 long form to the same /64", "[2001:0db8:0000:0001:bbbb:0000:0000:0002]:443", nil, 0, "2001:db8:0:1::/64"},
		{"ipv6 with zone", "[fe80::1%eth0]:5150", nil, 0, "fe80::/64"},
		{"ipv4-mapped ipv6", "[::ffff:192.0.2.10]:5150", nil, 0, "192.0.2.10"},
		{"xff ipv6 in brackets", "10.0.0.1:5150", []string{"[2001:db8::7]:80"}, 1, "2001:db8::/64"},
		{"xff ipv6 two hops", "10.0.0.1:5150", []string{"2001:db8:0:2::9, 198.51.100.7"}, 2, "2001:db8:0:2::/64"},
		{"remote without port", "192.0.2.10", nil, 0, "192.0.2.10"},
		{"unparsable remote", "pipe", nil, 0, "pipe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remote
			for _, v := range tc.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			assert.Equal(t, tc.want, ClientIP(r, tc.hops))
		})
	}
}
