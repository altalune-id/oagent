package events_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/events"
)

func TestApprovalPayloads_MatchTheirGoldens(t *testing.T) {
	base := events.ApprovalV1{
		ID: "Vb3Kq8Rt2Wy5Zp9Hm7Nc4", ProjectID: "Pj4kW9sQ2mN7vB3xC8zKr", AgentID: "AgTn6Hs3Jd4Kf7Mp5Qw3E",
		ThreadID: "ThQ2wE4rT6yU8jP9pA3sD", RunID: "RnZ3xC5vB7nM9qW2eR2tY", Tools: []string{"record_expense"}, Channel: "whatsapp",
	}
	requested, decided := base, base
	requested.Status, decided.Status = "pending", "approved"
	for golden, payload := range map[string]any{
		"approval_requested_v1": events.ApprovalRequestedV1(requested),
		"approval_decided_v1":   events.ApprovalDecidedV1(decided),
	} {
		t.Run(golden, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join("testdata", golden+".golden.json"))
			require.NoError(t, err)
			got, err := json.Marshal(payload)
			require.NoError(t, err)
			assert.JSONEq(t, string(want), string(got))
		})
	}
}

// SECURITY: an approval event names the call, never its arguments, the decision text or the requester's phone.
func TestApprovalPayloads_PublicIDsToolsStatusAndChannelOnly(t *testing.T) {
	want := []string{"agent_id", "channel", "id", "project_id", "run_id", "status", "thread_id", "tools"}
	for _, payload := range []any{events.ApprovalRequestedV1{}, events.ApprovalDecidedV1{}} {
		assert.Equal(t, want, jsonKeys(t, payload))
	}
}
