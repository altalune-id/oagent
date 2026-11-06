package weburl_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/weburl"
)

func TestCheck(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		allowHTTP bool
		want      string
		reason    weburl.Reason
	}{
		{name: "https", raw: "https://e.com/v1", want: "https://e.com/v1"},
		{name: "trimmed", raw: "  https://e.com/hook \n", want: "https://e.com/hook"},
		{name: "http allowed", raw: "http://127.0.0.1:8080/x", allowHTTP: true, want: "http://127.0.0.1:8080/x"},
		{name: "longest allowed", raw: "https://e.com/" + strings.Repeat("a", weburl.MaxLen-len("https://e.com/")), want: "https://e.com/" + strings.Repeat("a", weburl.MaxLen-len("https://e.com/"))},
		{name: "empty", raw: "", reason: weburl.Required},
		{name: "blank", raw: " \t ", reason: weburl.Required},
		{name: "too long", raw: "https://e.com/" + strings.Repeat("a", weburl.MaxLen), reason: weburl.TooLong},
		{name: "not utf8", raw: "https://e.com/\xff", reason: weburl.NotUTF8},
		{name: "unparsable", raw: "https://e.com/%zz", reason: weburl.Unparsable},
		{name: "http refused", raw: "http://e.com", reason: weburl.HTTPSRequired},
		{name: "ftp refused even when http allowed", raw: "ftp://e.com", allowHTTP: true, reason: weburl.HTTPSRequired},
		{name: "no scheme", raw: "e.com/x", reason: weburl.HTTPSRequired},
		{name: "user info", raw: "https://user:pass@e.com", reason: weburl.HasCredentials},
		{name: "user only", raw: "https://user@e.com", reason: weburl.HasCredentials},
		{name: "no host", raw: "https:///x", reason: weburl.NoHost},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := weburl.Check(tt.raw, tt.allowHTTP)
			assert.Equal(t, tt.reason, reason)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReason_Values(t *testing.T) {
	assert.Equal(t, []weburl.Reason{"required", "too_long", "not_utf8", "unparsable", "https_required", "has_credentials", "no_host"},
		[]weburl.Reason{weburl.Required, weburl.TooLong, weburl.NotUTF8, weburl.Unparsable, weburl.HTTPSRequired, weburl.HasCredentials, weburl.NoHost})
	assert.Equal(t, 2048, weburl.MaxLen)
}

func TestReason_Text(t *testing.T) {
	for reason, want := range map[weburl.Reason]string{
		weburl.Required:       "is required",
		weburl.TooLong:        "is longer than 2048 characters",
		weburl.NotUTF8:        "is not valid UTF-8",
		weburl.Unparsable:     "cannot be parsed",
		weburl.HTTPSRequired:  "must use https",
		weburl.HasCredentials: "must not carry credentials",
		weburl.NoHost:         "has no host",
		"other":               "is invalid",
	} {
		assert.Equal(t, want, reason.Text(), reason)
	}
}
