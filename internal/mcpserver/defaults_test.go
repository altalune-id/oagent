package mcpserver

import (
	"testing"
	"time"

	"buf.build/go/protovalidate"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	mcpserverv1 "altalune.id/oagent/gen/go/mcpserver/v1"
)

func TestDefaults_Published(t *testing.T) {
	assert.Equal(t, 10000, RecommendedTimeoutMS)
	assert.Equal(t, 1000, MinTimeoutMS)
	assert.Equal(t, 20000, MaxTimeoutMS)
}

func TestNew_ZeroTimeoutIsRecommendedAndTheRangeHolds(t *testing.T) {
	in := validInput()
	in.TimeoutMS = 0
	s, err := New(uuid.New(), uuid.New(), in, time.Now())
	require.NoError(t, err)
	assert.Equal(t, int32(RecommendedTimeoutMS), s.TimeoutMS, "the stored value is explicit")

	for _, ms := range []int32{MinTimeoutMS - 1, MaxTimeoutMS + 1, -1} {
		in.TimeoutMS = ms
		_, err := New(uuid.New(), uuid.New(), in, time.Now())
		ae, ok := apperror.AsAppError(err)
		require.True(t, ok, "%d: %v", ms, err)
		assert.Equal(t, "timeout_ms", ae.Meta()["field"], "%d", ms)
	}
	for _, ms := range []int32{MinTimeoutMS, MaxTimeoutMS} {
		in.TimeoutMS = ms
		_, err := New(uuid.New(), uuid.New(), in, time.Now())
		assert.NoError(t, err, "%d", ms)
	}

	in.TimeoutMS = 7000
	_, err = s.Update(in, time.Now())
	require.NoError(t, err)
	in.TimeoutMS = 0
	_, err = s.Update(in, time.Now())
	require.NoError(t, err)
	assert.Equal(t, int32(7000), s.TimeoutMS, "0 on update keeps the stored timeout, never silently resets it")
	in.TimeoutMS = MaxTimeoutMS + 1
	_, err = s.Update(in, time.Now())
	assert.Error(t, err, "update keeps the range")
}

// SECURITY: the proto rules and the domain share the timeout range, so the API and a stored server agree.
func TestTimeout_ProtoRulesMatchTheRange(t *testing.T) {
	v, err := protovalidate.New()
	require.NoError(t, err)
	for _, ms := range []int32{0, MinTimeoutMS, MaxTimeoutMS} {
		assert.NoError(t, v.Validate(&mcpserverv1.UpdateMcpServerRequest{Id: "MsYasakuQcaz23456789a", Url: "https://e.com/mcp",
			AuthKind: mcpserverv1.McpServerAuthKind_MCP_SERVER_AUTH_KIND_NONE, TimeoutMs: ms}), "%d", ms)
	}
	for _, ms := range []int32{-1, MinTimeoutMS - 1, MaxTimeoutMS + 1} {
		err := v.Validate(&mcpserverv1.UpdateMcpServerRequest{Id: "MsYasakuQcaz23456789a", Url: "https://e.com/mcp",
			AuthKind: mcpserverv1.McpServerAuthKind_MCP_SERVER_AUTH_KIND_NONE, TimeoutMs: ms})
		assert.Error(t, err, "%d", ms)
		createErr := v.Validate(&mcpserverv1.CreateMcpServerRequest{TimeoutMs: ms})
		assert.ErrorContains(t, createErr, "timeout_ms", "%d", ms)
	}
}
