//go:build integration

package schema_test

import (
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/migrator"
	"altalune.id/oagent/schema"
)

//nolint:gochecknoglobals // the quiet times fixtures write, from the Go defaults.
var quietDefaults = strconv.FormatInt(channel.DefaultQuietWindow.Milliseconds(), 10) + ", " + strconv.FormatInt(channel.DefaultQuietMax.Milliseconds(), 10)

const insertBareConnection = `INSERT INTO channel_connections (id, public_id, org_id, project_id, agent_id, provider, status, base_url, openwa_org, openwa_project,
	device_id, signing_secret_primary_sealed, device_key_sealed, reply_failure, reply_voice, reply_photo_unreadable, approval_prompt, approval_expired, created_at, updated_at)
	VALUES ($1, $2, $3, $4, $5, 'openwa', 'active', 'https://wa.example.com', 'acme', 'main', 'dev_1', '\x01', '\x02', 'f', 'v', 'p', 'ap', 'ae', now(), now())`

// NOTE: production has data; dropping the defaults must keep every stored quiet time and leave Go as the only source.
func TestMigration034_ChannelQuietDefaultsDropped(t *testing.T) {
	db, schemaName := pgtest.Migrated(t)
	cfg := migrator.Config{Schema: schemaName}
	ctx := t.Context()
	require.NoError(t, schema.MigrateDownTo(ctx, db, cfg, 33))
	s := seedRunScope(t, db)
	before := uuid.New()
	_, err := db.ExecContext(ctx, insertBareConnection, before, "c"+before.String()[:12], s.orgA, s.prjA, s.agentA)
	require.NoError(t, err, "a connection as 033 shaped it, quiet times from the column defaults")

	require.NoError(t, schema.MigrateUp(ctx, db, cfg))
	var window, maxWait int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT quiet_window_ms, quiet_max_ms FROM channel_connections WHERE id = $1`, before).Scan(&window, &maxWait))
	assert.Equal(t, 3000, window, "a stored row keeps its value")
	assert.Equal(t, 10000, maxWait)

	var defaults int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema = $1 AND table_name = 'channel_connections'
		AND column_name IN ('quiet_window_ms', 'quiet_max_ms') AND column_default IS NULL AND is_nullable = 'NO'`, schemaName).Scan(&defaults))
	assert.Equal(t, 2, defaults, "NOT NULL with no default: Go writes every new row's quiet times")

	after := uuid.New()
	_, err = db.ExecContext(ctx, insertBareConnection, after, "c"+after.String()[:12], s.orgA, s.prjA, s.agentA)
	assert.Error(t, err, "an insert must name the quiet times")
}

// SECURITY: the database refuses what Go refuses, so a quiet time over channel.MaxQuiet never reaches a stored row.
func TestChannelQuiet_ChecksMatchGo(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedRunScope(t, db)
	conn, _ := s.connection(t, db)
	maxMS := int(channel.MaxQuiet.Milliseconds())
	set := func(window, maxWait int) error {
		_, err := db.ExecContext(t.Context(), `UPDATE channel_connections SET quiet_window_ms = $2, quiet_max_ms = $3 WHERE id = $1`, conn, window, maxWait)
		return err
	}
	require.NoError(t, set(0, 0))
	require.NoError(t, set(maxMS, maxMS))
	assert.Error(t, set(maxMS+1, maxMS+1), "a window past the max")
	_, err := db.ExecContext(t.Context(), `UPDATE channel_connections SET quiet_window_ms = $2 WHERE id = $1`, conn, maxMS+1)
	assert.Contains(t, []string{"channel_connections_quiet_window_check", "channel_connections_quiet_max_check"}, pgConstraint(err))
	assert.Equal(t, "channel_connections_quiet_max_check", pgConstraint(set(maxMS, maxMS+1)))
}
