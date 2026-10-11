//go:build integration

package boot

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/apikey"
)

func (f *apiFixture) mcpCall(t *testing.T, bearer, body string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, f.mcpURL, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, resp.Header, out
}

func TestMCP_KeyScopesTheToolList(t *testing.T) {
	f := newAPIFixture(t)
	key := f.mint(t, apikey.KindOrg, "agent:read")

	status, _, body := f.mcpCall(t, key, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	require.Equal(t, http.StatusOK, status, string(body))
	var list struct {
		Result struct {
			Tools []struct{ Name string } `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &list))
	var names []string
	for _, tool := range list.Result.Tools {
		names = append(names, tool.Name)
	}
	assert.ElementsMatch(t, []string{"agent_get", "agent_list", "auth_whoami"}, names)
}

func TestMCP_WhoamiWithAKey(t *testing.T) {
	f := newAPIFixture(t)
	key := f.mint(t, apikey.KindOrg, "agent:read")

	status, _, body := f.mcpCall(t, key, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"auth_whoami","arguments":{}}}`)
	require.Equal(t, http.StatusOK, status, string(body))
	var call struct {
		Result struct {
			IsError           bool           `json:"isError"`
			StructuredContent map[string]any `json:"structuredContent"`
			Content           []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &call))
	require.False(t, call.Result.IsError, string(body))
	assert.Contains(t, string(body), `\"source\":\"apikey\"`)
}

func TestMCP_BadKeyIs401(t *testing.T) {
	f := newAPIFixture(t)
	status, header, body := f.mcpCall(t, "oagent_nope", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.Equal(t, "Bearer", header.Get("WWW-Authenticate"))
	assert.Contains(t, string(body), `"code":"GEN002"`)
}
