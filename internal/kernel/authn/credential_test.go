package authn

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/kernel/session"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef")

const testPrefix = "oagent_"

func basic(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func TestFromHeader(t *testing.T) {
	goodCookie := session.CookieName + "=" + session.Sign(testSecret, "sid-1")
	cases := map[string]struct {
		auth    string
		cookie  string
		want    Credential
		present bool
	}{
		"nothing":                   {"", "", Credential{}, false},
		"other cookie only":         {"", "a=b", Credential{}, false},
		"bearer key":                {"Bearer oagent_abc", "", Credential{Kind: KindAPIKey, Token: "oagent_abc"}, true},
		"lowercase bearer":          {"bearer oagent_abc", "", Credential{Kind: KindAPIKey, Token: "oagent_abc"}, true},
		"bearer extra spaces":       {"Bearer   oagent_abc  ", "", Credential{Kind: KindAPIKey, Token: "oagent_abc"}, true},
		"bearer without prefix":     {"Bearer eyJ.abc.def", "", Credential{}, true},
		"bearer empty":              {"Bearer ", "", Credential{}, true},
		"unknown scheme":            {"Digest abc", "", Credential{}, true},
		"basic good":                {basic("admin", "s3:cret"), "", Credential{Kind: KindBasic, User: "admin", Password: "s3:cret"}, true},
		"basic lowercase":           {"basic " + base64.StdEncoding.EncodeToString([]byte("a:b")), "", Credential{Kind: KindBasic, User: "a", Password: "b"}, true},
		"basic bad base64":          {"Basic !!!", "", Credential{}, true},
		"basic no colon":            {"Basic " + base64.StdEncoding.EncodeToString([]byte("admin")), "", Credential{}, true},
		"cookie good":               {"", goodCookie, Credential{Kind: KindSession, Token: "sid-1"}, true},
		"cookie tampered":           {"", goodCookie + "x", Credential{}, true},
		"header beats cookie":       {"Bearer oagent_abc", goodCookie, Credential{Kind: KindAPIKey, Token: "oagent_abc"}, true},
		"bad header ignores cookie": {"Bearer nope", goodCookie, Credential{}, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := http.Header{}
			if tc.auth != "" {
				h.Set("Authorization", tc.auth)
			}
			if tc.cookie != "" {
				h.Set("Cookie", tc.cookie)
			}
			got, present := FromHeader(h, testPrefix, testSecret)
			assert.Equal(t, tc.present, present)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFromHeader_EmptyPrefixUsesNoKeys(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer anything")
	got, present := FromHeader(h, "", testSecret)
	assert.True(t, present)
	assert.Equal(t, KindNone, got.Kind)
}
