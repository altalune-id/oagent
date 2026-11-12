//go:build integration

package schema_test

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/migrator"
	"altalune.id/oagent/schema"
)

const oneCall = `[{"tool_call_id":"call_1","interrupt_id":"intr_1","server":"yasaku","tool":"record_expense","args":{"amount":1},"args_hash":"h","keys":["amount"]}]`

func (s runScope) approval(db *sql.DB, t *testing.T, thread, run uuid.UUID, mutate string, args ...any) error {
	t.Helper()
	id := uuid.New()
	base := []any{id, "a" + id.String()[:12], s.orgA, s.prjA, s.agentA, run, thread}
	_, err := db.ExecContext(t.Context(), `INSERT INTO approvals (id, public_id, org_id, project_id, agent_id, run_id, thread_id, calls, preview, channel,
		raised_snapshot_hash, status, expires_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, `+mutate+`, now() + interval '1 day', now())`, append(base, args...)...)
	return err
}

func pending(channel string) string {
	return `'` + oneCall + `'::jsonb, 'yasaku · record_expense', '` + channel + `', '\x01', 'pending'`
}

func TestApprovals_OnePendingPerThread(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedRunScope(t, db)
	thread := s.thread(t, db, "one-pending")
	run, err := s.run(db, t, thread, "awaiting_approval")
	require.NoError(t, err)
	require.NoError(t, s.approval(db, t, thread, run, pending("api")))
	err = s.approval(db, t, thread, run, pending("api"))
	assert.Equal(t, "approvals_thread_pending_key", pgConstraint(err), "a second pending approval on the thread is refused")

	_, err = db.ExecContext(t.Context(), `UPDATE approvals SET status = 'approved', decided_at = now(), decided_by_kind = 'user', decided_by_id = 'u', decided_via = 'console' WHERE thread_id = $1`, thread)
	require.NoError(t, err)
	assert.NoError(t, s.approval(db, t, thread, run, pending("api")), "once decided, the thread may wait again")
}

func TestApprovals_Checks(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedRunScope(t, db)
	for _, channel := range []string{"whatsapp", "api", "console"} {
		thread := s.thread(t, db, "ch-"+channel)
		run, err := s.run(db, t, thread, "awaiting_approval")
		require.NoError(t, err)
		assert.NoError(t, s.approval(db, t, thread, run, pending(channel)), "channel %s, console included for try threads", channel)
	}
	nine := `'[` + oneCall[1:len(oneCall)-1]
	for range 8 {
		nine += `,` + oneCall[1:len(oneCall)-1]
	}
	nine += `]'::jsonb`
	for name, tc := range map[string]struct{ values, constraint string }{
		"another channel":      {`'` + oneCall + `'::jsonb, 'p', 'email', '\x01', 'pending'`, "approvals_channel_check"},
		"calls not an array":   {`'{"a":1}'::jsonb, 'p', 'api', '\x01', 'pending'`, "approvals_calls_shape"},
		"no calls":             {`'[]'::jsonb, 'p', 'api', '\x01', 'pending'`, "approvals_calls_shape"},
		"nine calls":           {nine + `, 'p', 'api', '\x01', 'pending'`, "approvals_calls_shape"},
		"decided with no time": {`'` + oneCall + `'::jsonb, 'p', 'api', '\x01', 'approved'`, "approvals_decided_shape"},
	} {
		thread := s.thread(t, db, "bad-"+name)
		run, err := s.run(db, t, thread, "awaiting_approval")
		require.NoError(t, err)
		err = s.approval(db, t, thread, run, tc.values)
		assert.Equal(t, tc.constraint, pgConstraint(err), name)
	}
	thread := s.thread(t, db, "decided-checks")
	run, err := s.run(db, t, thread, "awaiting_approval")
	require.NoError(t, err)
	require.NoError(t, s.approval(db, t, thread, run, pending("api")))
	for name, tc := range map[string]struct{ set, constraint string }{
		"another status":   {`status = 'executing', decided_at = now()`, "approvals_status_check"},
		"pending, decided": {`decided_at = now()`, "approvals_decided_shape"},
		"unknown decider":  {`decided_by_kind = 'robot'`, "approvals_decided_by_kind_check"},
		"unknown via":      {`decided_via = 'email'`, "approvals_decided_via_check"},
		"long text":        {`decision_text = repeat('x', 4001)`, "approvals_decision_text_length"},
	} {
		_, err := db.ExecContext(t.Context(), `UPDATE approvals SET `+tc.set+` WHERE thread_id = $1`, thread)
		assert.Equal(t, tc.constraint, pgConstraint(err), name)
	}
}

func TestRuns_ReplyKind(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedRunScope(t, db)
	thread := s.thread(t, db, "reply-kind")
	run, err := s.run(db, t, thread, "completed")
	require.NoError(t, err)
	for _, kind := range []string{"reply", "prompt", "expired"} {
		_, err := db.ExecContext(t.Context(), `UPDATE runs SET reply_kind = $1 WHERE id = $2`, kind, run)
		assert.NoError(t, err, kind)
	}
	_, err = db.ExecContext(t.Context(), `UPDATE runs SET reply_kind = 'email' WHERE id = $1`, run)
	assert.Equal(t, "runs_reply_kind_check", pgConstraint(err))
}

// NOTE: production has data, so a NOT NULL column added to a table with rows must give those rows a value.
func TestMigration028_ExistingRowsKeepWorking(t *testing.T) {
	db, schemaName := pgtest.Migrated(t)
	cfg := migrator.Config{Schema: schemaName}
	ctx := t.Context()
	require.NoError(t, schema.MigrateDownTo(ctx, db, cfg, 27))
	s := seedRunScope(t, db)
	connID := uuid.New()
	_, err := db.ExecContext(ctx, `INSERT INTO channel_connections (id, public_id, org_id, project_id, agent_id, provider, status, base_url, openwa_org, openwa_project,
		device_id, signing_secret_primary_sealed, device_key_sealed, reply_failure, reply_voice, reply_photo_unreadable, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'openwa', 'active', 'https://wa.example.com', 'acme', 'main', 'dev_1', '\x01', '\x02', 'f', 'v', 'p', now(), now())`,
		connID, "c"+connID.String()[:12], s.orgA, s.prjA, s.agentA)
	require.NoError(t, err, "a connection as 027 shaped it")
	thread := s.thread(t, db, "before-028")
	run, err := s.run(db, t, thread, "completed")
	require.NoError(t, err)

	require.NoError(t, schema.MigrateUp(ctx, db, cfg))
	var prompt, expired string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT approval_prompt, approval_expired FROM channel_connections WHERE id = $1`, connID).Scan(&prompt, &expired))
	assert.Equal(t, "{preview}\n\nReply YES or NO", prompt)
	assert.Equal(t, "This request expired. Please ask again.", expired)
	var kind sql.NullString
	require.NoError(t, db.QueryRowContext(ctx, `SELECT reply_kind FROM runs WHERE id = $1`, run).Scan(&kind))
	assert.False(t, kind.Valid, "an existing run has no reply kind")

	var defaults int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema = $1 AND table_name = 'channel_connections'
		AND column_name IN ('approval_prompt', 'approval_expired') AND column_default IS NULL AND is_nullable = 'NO'`, schemaName).Scan(&defaults))
	assert.Equal(t, 2, defaults, "NOT NULL with the default dropped again: Go writes every new row's text")
}

func TestApprovals_RunIndex(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	var def string
	require.NoError(t, db.QueryRowContext(t.Context(),
		`SELECT indexdef FROM pg_indexes WHERE tablename = 'approvals' AND indexname = 'approvals_run_idx'`).Scan(&def),
		"a run's approval is read by index")
	assert.Contains(t, def, "(org_id, run_id)")
}
