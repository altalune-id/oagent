package layout

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveReturnTo(t *testing.T) {
	cases := []struct {
		base, raw, want string
	}{
		{"", "", "/console/"},
		{"", "/console/o/acme", "/console/o/acme"},
		{"", "/console/x?tab=keys#top", "/console/x?tab=keys#top"},
		{"", "//evil.com", "/console/"},
		{"", "///evil.com", "/console/"},
		{"", `/\evil.com`, "/console/"},
		{"", `\\evil.com`, "/console/"},
		{"", "https://evil.com", "/console/"},
		{"", "https://evil.com/console/", "/console/"},
		{"", "javascript:alert(1)", "/console/"},
		{"", "JavaScript:alert(1)", "/console/"},
		{"", "data:text/html,hi", "/console/"},
		{"", "console/x", "/console/"},
		{"", "/\t/evil.com", "/console/"},
		{"", "/\n/evil.com", "/console/"},
		{"", "http:/evil.com", "/console/"},
		{"/app", "/app/console/x", "/app/console/x"},
		{"/app", "/app", "/app"},
		{"/app", "/console/x", "/app/console/"},
		{"/app", "/appx/console", "/app/console/"},
		{"/app", "//evil.com/app/", "/app/console/"},
		{"/app/", "/app/console/", "/app/console/"},
	}
	for _, tc := range cases {
		t.Run(tc.base+" "+tc.raw, func(t *testing.T) {
			assert.Equal(t, tc.want, ResolveReturnTo(tc.base, tc.raw))
		})
	}
}
