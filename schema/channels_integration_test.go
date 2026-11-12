//go:build integration

package schema_test

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/pgtest"
)

func (s runScope) connection(t *testing.T, db *sql.DB) (id uuid.UUID, publicID string) {
	t.Helper()
	id = uuid.New()
	publicID = "c" + id.String()[:12]
	_, err := db.ExecContext(t.Context(), `INSERT INTO channel_connections (id, public_id, org_id, project_id, agent_id, provider, status, base_url, openwa_org, openwa_project,
		device_id, signing_secret_primary_sealed, device_key_sealed, reply_failure, reply_voice, reply_photo_unreadable, approval_prompt, approval_expired, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'openwa', 'active', 'https://wa.example.com', 'acme', 'main', 'dev_1', '\x01', '\x02', 'f', 'v', 'p', 'ap', 'ae', now(), now())`,
		id, publicID, s.orgA, s.prjA, s.agentA)
	require.NoError(t, err)
	return id, publicID
}

func (s runScope) channelThread(db *sql.DB, t *testing.T, conn uuid.UUID, key string) (uuid.UUID, error) {
	t.Helper()
	id := uuid.New()
	_, err := db.ExecContext(t.Context(), `INSERT INTO threads (id, public_id, org_id, project_id, agent_id, kind, external_key, connection_id, reply_via, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'dm', $6, $7, 'wa_phone', now(), now())`, id, "t"+id.String()[:12], s.orgA, s.prjA, s.agentA, key, conn)
	return id, err
}

func TestResolveChannelConnection_PublicColumnsOnly(t *testing.T) {
	svc, admin, schemaName := pgtest.MigratedRLS(t)
	s := seedRunScope(t, admin)
	conn, publicID := s.connection(t, admin)
	rows, err := svc.W.QueryContext(t.Context(), `SELECT * FROM `+schemaName+`.resolve_channel_connection_by_public_id($1)`, publicID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	require.NoError(t, err)
	assert.Equal(t, []string{"id", "org_id", "project_id", "agent_id", "provider", "device_id", "status", "secured"}, cols, "no secret, key or URL leaves the definer")
	require.True(t, rows.Next())
	var id, org, project, agent uuid.UUID
	var provider, device, status string
	var secured bool
	require.NoError(t, rows.Scan(&id, &org, &project, &agent, &provider, &device, &status, &secured))
	assert.True(t, secured, "whether a signing secret is set, never the secret")
	assert.Equal(t, conn, id)
	assert.Equal(t, s.orgA, org)
	assert.Equal(t, "openwa", provider)
	assert.Equal(t, "dev_1", device)
	assert.False(t, rows.Next())

	var n int
	require.NoError(t, svc.W.QueryRowContext(t.Context(), `SELECT count(*) FROM `+schemaName+`.resolve_channel_connection_by_public_id('nope')`).Scan(&n))
	assert.Zero(t, n)
}

func TestThreads_APIKeyDoesNotBlockChannelThread(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedRunScope(t, db)
	conn, _ := s.connection(t, db)
	s.thread(t, db, "cht_Gr5Tn8Vc1Hs6Jq0b")
	_, err := s.channelThread(db, t, conn, "cht_Gr5Tn8Vc1Hs6Jq0b")
	require.NoError(t, err, "an API thread with a cht_ key never blocks the WhatsApp thread")
	_, err = s.channelThread(db, t, conn, "cht_Gr5Tn8Vc1Hs6Jq0b")
	assert.Equal(t, "threads_connection_external_key_key", pgConstraint(err),
		"one thread per connection and chat")
	_, err = db.ExecContext(t.Context(), `INSERT INTO threads (id, public_id, org_id, project_id, agent_id, kind, external_key, created_at, updated_at) VALUES ($1, 'tdup2', $2, $3, $4, 'api', 'cht_Gr5Tn8Vc1Hs6Jq0b', now(), now())`,
		uuid.New(), s.orgA, s.prjA, s.agentA)
	assert.Equal(t, "threads_agent_external_key_key", pgConstraint(err), "API threads still key on (agent, external key)")
}

func TestChannelSchema_Checks(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedRunScope(t, db)
	conn, _ := s.connection(t, db)
	set := func(q string, args ...any) error {
		_, err := db.ExecContext(t.Context(), q, args...)
		return err
	}
	assert.Equal(t, "channel_connections_provider_check", pgConstraint(set(`UPDATE channel_connections SET provider = 'telegram' WHERE id = $1`, conn)))
	require.NoError(t, set(`UPDATE channel_connections SET status = 'disabled' WHERE id = $1`, conn))
	assert.Equal(t, "channel_connections_status_check", pgConstraint(set(`UPDATE channel_connections SET status = 'connected' WHERE id = $1`, conn)),
		"status is the on/off switch; how a connection is doing lives in last_result")
	assert.Equal(t, "channel_connections_quiet_window_check", pgConstraint(set(`UPDATE channel_connections SET quiet_window_ms = -1 WHERE id = $1`, conn)))
	assert.Equal(t, "channel_connections_quiet_max_check", pgConstraint(set(`UPDATE channel_connections SET quiet_window_ms = 5000, quiet_max_ms = 4000 WHERE id = $1`, conn)))
	assert.Equal(t, "channel_connections_last_result_check", pgConstraint(set(`UPDATE channel_connections SET last_result = 'bad_signature' WHERE id = $1`, conn)),
		"a bad signature is never a result")
	assert.Equal(t, "channel_connections_reply_failure_length", pgConstraint(set(`UPDATE channel_connections SET reply_failure = '' WHERE id = $1`, conn)))
	var window, maxWait int
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT quiet_window_ms, quiet_max_ms FROM channel_connections WHERE id = $1`, conn).Scan(&window, &maxWait))
	assert.Equal(t, 3000, window)
	assert.Equal(t, 10000, maxWait)

	require.NoError(t, set(`INSERT INTO ingest_deliveries (org_id, project_id, connection_id, delivery_id, received_at) VALUES ($1, $2, $3, 'dlv_1', now())`, s.orgA, s.prjA, conn))
	assert.Equal(t, "ingest_deliveries_pkey", pgConstraint(set(`INSERT INTO ingest_deliveries (org_id, project_id, connection_id, delivery_id, received_at) VALUES ($1, $2, $3, 'dlv_1', now())`,
		s.orgA, s.prjA, conn)), "a delivery is recorded once per connection")

	thread := s.thread(t, db, "k-skip")
	assert.Equal(t, "messages_skip_reason_check", pgConstraint(set(`INSERT INTO messages (id, org_id, project_id, thread_id, seq, role, body, status, skip_reason, created_at)
		VALUES ($1, $2, $3, $4, 1, 'user', '{}', 'consumed', 'turn_cap', now())`, uuid.New(), s.orgA, s.prjA, thread)))
	run, err := s.run(db, t, thread, "completed")
	require.NoError(t, err)
	assert.Equal(t, "runs_reply_state_check", pgConstraint(set(`UPDATE runs SET reply_state = 'queued' WHERE id = $1`, run)))
	assert.Equal(t, "threads_reply_via_check", pgConstraint(set(`UPDATE threads SET reply_via = 'sms' WHERE id = $1`, thread)))
	assert.Equal(t, "end_users_display_name_length", pgConstraint(set(`UPDATE end_users SET display_name = repeat('x', 257) WHERE id = $1`, s.endUserA)))

	require.NoError(t, set(`DELETE FROM channel_connections WHERE id = $1`, conn))
	var left int
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT count(*) FROM ingest_deliveries WHERE connection_id = $1`, conn).Scan(&left))
	assert.Zero(t, left, "deliveries go with their connection")
}

func TestChannelConnections_DeleteKeepsHistory(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedRunScope(t, db)
	conn, _ := s.connection(t, db)
	thread, err := s.channelThread(db, t, conn, "cht_Gr5Tn8Vc1Hs6Jq0b")
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `INSERT INTO messages (id, org_id, project_id, thread_id, seq, role, body, status, created_at)
		VALUES ($1, $2, $3, $4, 1, 'user', '{}', 'consumed', now())`, uuid.New(), s.orgA, s.prjA, thread)
	require.NoError(t, err)
	_, err = s.run(db, t, thread, "completed")
	require.NoError(t, err)

	_, err = db.ExecContext(t.Context(), `DELETE FROM channel_connections WHERE id = $1`, conn)
	require.NoError(t, err)

	var detached sql.Null[uuid.UUID]
	var project uuid.UUID
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT connection_id, project_id FROM threads WHERE id = $1`, thread).Scan(&detached, &project),
		"deleting a connection keeps its threads")
	assert.False(t, detached.Valid, "the thread is detached from the deleted connection")
	assert.Equal(t, s.prjA, project, "only connection_id is cleared")
	var messages, runs int
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT (SELECT count(*) FROM messages WHERE thread_id = $1), (SELECT count(*) FROM runs WHERE thread_id = $1)`,
		thread).Scan(&messages, &runs))
	assert.Equal(t, 1, messages, "messages survive")
	assert.Equal(t, 1, runs, "runs survive")
}
