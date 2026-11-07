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

func TestChannelConnectionPayloads_MatchTheirGoldens(t *testing.T) {
	for golden, payload := range map[string]any{
		"channel_connection_created_v1": events.ChannelConnectionCreatedV1{
			ID: "cnxVb3Kq8Rt2Wy5Zp9Lm1", ProjectID: "Pj4kW9sQ2mN7vB3xC8zLr", AgentID: "AgTn6Hs1Jd4Kf7Lp0Qw3E", Provider: "openwa", Status: "waiting",
		},
		"channel_connection_updated_v1": events.ChannelConnectionUpdatedV1{
			ID: "cnxVb3Kq8Rt2Wy5Zp9Lm1", ProjectID: "Pj4kW9sQ2mN7vB3xC8zLr", AgentID: "AgTn6Hs1Jd4Kf7Lp0Qw3E", Provider: "openwa", Status: "connected",
		},
		"channel_connection_deleted_v1": events.ChannelConnectionDeletedV1{
			ID: "cnxVb3Kq8Rt2Wy5Zp9Lm1", ProjectID: "Pj4kW9sQ2mN7vB3xC8zLr", AgentID: "AgTn6Hs1Jd4Kf7Lp0Qw3E", Provider: "openwa", Status: "connected",
		},
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

func TestChannelConnectionPayloads_PublicIDsProviderAndStatusOnly(t *testing.T) {
	want := []string{"agent_id", "id", "project_id", "provider", "status"}
	for _, payload := range []any{events.ChannelConnectionCreatedV1{}, events.ChannelConnectionUpdatedV1{}, events.ChannelConnectionDeletedV1{}} {
		keys := jsonKeys(t, payload)
		assert.Equal(t, want, keys, "no secret, key, base URL or device data")
	}
}
