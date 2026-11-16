package webpolicy_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/webpolicy"
)

func TestHeaders_MatchSharedJSON(t *testing.T) {
	raw, err := os.ReadFile("testdata/headers.json")
	require.NoError(t, err)
	var shared struct {
		Reserved     []string `json:"reserved"`
		ModelRefused []string `json:"modelRefused"`
	}
	require.NoError(t, json.Unmarshal(raw, &shared))
	assert.Equal(t, shared.Reserved, webpolicy.ReservedHeaders)
	assert.Equal(t, shared.ModelRefused, webpolicy.ModelRefusedHeaders)
}

func TestReserved_IsCaseInsensitiveAndExact(t *testing.T) {
	for name, want := range map[string]bool{
		"Host": true, "host": true, "X-FORWARDED-FOR": true, "cookie": true,
		"Authorization": false, "X-Forwarded": false, "Content-Type": false, "Hostname": false,
	} {
		assert.Equal(t, want, webpolicy.Reserved(name), name)
	}
}

func TestModelRefused_IsCaseInsensitive(t *testing.T) {
	assert.True(t, webpolicy.ModelRefused("authorization"))
	assert.True(t, webpolicy.ModelRefused("AUTHORIZATION"))
	assert.False(t, webpolicy.ModelRefused("X-Api-Key"))
	assert.False(t, webpolicy.ModelRefused("Host"))
}

func TestValidHost_Vectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/hosts.json")
	require.NoError(t, err)
	var vectors []struct {
		Host  string `json:"host"`
		Valid bool   `json:"valid"`
	}
	require.NoError(t, json.Unmarshal(raw, &vectors))
	require.NotEmpty(t, vectors)
	for _, v := range vectors {
		assert.Equal(t, v.Valid, webpolicy.ValidHost(v.Host), "%q", v.Host)
	}
}

func TestValidHost_HexLastLabelAndLength(t *testing.T) {
	assert.False(t, webpolicy.ValidHost("a.0x7f"), "a hex last label parses as IPv4")
	assert.True(t, webpolicy.ValidHost("a.0xzz"))
	long := strings.Repeat(strings.Repeat("a", 63)+".", 4) + "com"
	assert.False(t, webpolicy.ValidHost(long), "over 253 characters")
}

func TestValidHeaderName(t *testing.T) {
	for name, want := range map[string]bool{
		"X-Api-Key": true, "x_custom.v2": true, "Authorization": true,
		"": false, "X Api": false, "X:Api": false, "é": false, strings.Repeat("a", 65): false, strings.Repeat("a", 64): true,
	} {
		assert.Equal(t, want, webpolicy.ValidHeaderName(name), "%q", name)
	}
}
