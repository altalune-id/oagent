//go:build integration

package run_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/run"
)

// SECURITY: a channel message keyed to another org's connection meets the thread upsert's org guard and answers like an unknown thread.
func TestStore_ChannelThreadOfAnotherOrgIsRefused(t *testing.T) {
	f := newPgFixture(t)
	a, b := f.seed(t), f.seed(t)
	conn := uuid.New()
	f.exec(t, `INSERT INTO `+f.schema+`.channel_connections (id, public_id, org_id, project_id, agent_id, provider, status, base_url, openwa_org, openwa_project, device_id,
		signing_secret_primary_sealed, device_key_sealed, reply_failure, reply_voice, reply_photo_unreadable, approval_prompt, approval_expired, quiet_window_ms, quiet_max_ms,
		created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'openwa', 'active', 'https://wa.example.com', 'acme', 'main', 'dev_1', '\x01', '\x02', 'f', 'v', 'p', 'ap', 'ae', `+quietDefaults+`, now(), now())`,
		conn, "c"+conn.String()[:12], a.org, a.project, a.agent)
	msg := func(s scope, address string) run.ChannelMessage {
		return run.ChannelMessage{
			ProjectID: s.project, AgentID: s.agent, ConnectionID: conn, ChatKey: "cht_shared", Kind: "dm", ReplyVia: "wa_phone", ChannelAddress: address,
			Message: run.Message{Body: []byte(`{"id":"m","role":"user","content":"hi"}`), ExternalMessageID: "msg_" + address}, SenderCap: 5,
		}
	}
	first, err := f.store.AppendChannelMessage(in(a.org), msg(a, "mine"))
	require.NoError(t, err)

	_, err = f.store.AppendChannelMessage(in(b.org), msg(b, "hijack"))
	assert.True(t, run.IsThreadNotFoundError(err), "got %T: %v", err, err)
	var address string
	require.NoError(t, f.admin.QueryRowContext(t.Context(), `SELECT channel_address FROM `+f.schema+`.threads WHERE id = $1`, first.ThreadID).Scan(&address))
	assert.Equal(t, "mine", address, "the other org's thread is untouched")
}
