package tokens_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/tokens"
)

func TestNewResourceInsertsWellKnownBeforeThePath(t *testing.T) {
	for resource, want := range map[string][2]string{
		"https://oagent.example.com/mcp":         {"/.well-known/oauth-protected-resource/mcp", "https://oagent.example.com/.well-known/oauth-protected-resource/mcp"},
		"https://oagent.example.com/oagent/mcp":  {"/.well-known/oauth-protected-resource/oagent/mcp", "https://oagent.example.com/.well-known/oauth-protected-resource/oagent/mcp"},
		"https://oagent.example.com/oagent/mcp/": {"/.well-known/oauth-protected-resource/oagent/mcp", "https://oagent.example.com/.well-known/oauth-protected-resource/oagent/mcp"},
		"http://localhost:5150/mcp":              {"/.well-known/oauth-protected-resource/mcp", "http://localhost:5150/.well-known/oauth-protected-resource/mcp"},
	} {
		t.Run(resource, func(t *testing.T) {
			r, err := tokens.NewResource(resource)
			require.NoError(t, err)
			assert.Equal(t, resource, r.Identifier)
			assert.Equal(t, want[0], r.MetadataPath)
			assert.Equal(t, want[1], r.MetadataURL)
		})
	}
}

func TestNewResourceRejectsAnUnusableIdentifier(t *testing.T) {
	for _, raw := range []string{"", "/mcp", "https://", "https://oagent.example.com/mcp#f", "urn:oagent:mcp"} {
		_, err := tokens.NewResource(raw)
		assert.ErrorAs(t, err, new(*tokens.ResourceInvalidError), "%q", raw)
	}
}

func TestResourceMetadataAndChallengesAgree(t *testing.T) {
	r, err := tokens.NewResource("https://oagent.example.com/mcp")
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	r.MetadataHandler("https://idp.example.com", []string{"agent:read"}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, r.MetadataURL, nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var doc struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
		ScopesSupported      []string `json:"scopes_supported"`
		BearerMethods        []string `json:"bearer_methods_supported"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &doc))
	assert.Equal(t, r.Identifier, doc.Resource)
	assert.Equal(t, []string{"https://idp.example.com"}, doc.AuthorizationServers)
	assert.Equal(t, []string{"agent:read"}, doc.ScopesSupported)
	assert.Equal(t, []string{"header"}, doc.BearerMethods)

	assert.Equal(t, `Bearer resource_metadata="`+r.MetadataURL+`"`, r.Challenge())
	assert.Equal(t, `Bearer resource_metadata="`+r.MetadataURL+`", error="invalid_token"`, r.InvalidToken())
	assert.Equal(t, `Bearer resource_metadata="`+r.MetadataURL+`", error="insufficient_scope", scope="agent:write"`, r.InsufficientScope("agent:write"))
}

func TestResource_ZeroIsAPlainBearerChallenge(t *testing.T) {
	var r tokens.Resource
	assert.Equal(t, "Bearer", r.Challenge())
	assert.Equal(t, "Bearer", r.InvalidToken())
	assert.Equal(t, "Bearer", r.InsufficientScope("agent:write"))
}
