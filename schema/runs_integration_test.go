//go:build integration

package schema_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/testutil/pgtest"
)

type runScope struct {
	twoOrgs
	agentA, versionA, endUserA, agentB uuid.UUID
}

func seedRunScope(t *testing.T, db *sql.DB) runScope {
	t.Helper()
	s := runScope{twoOrgs: seedTwoOrgs(t, db), agentA: uuid.New(), versionA: uuid.New(), endUserA: uuid.New(), agentB: uuid.New()}
	ctx := t.Context()
	for agent, scope := range map[uuid.UUID][2]uuid.UUID{s.agentA: {s.orgA, s.prjA}, s.agentB: {s.orgB, s.prjB}} {
		_, err := db.ExecContext(ctx, `INSERT INTO agents (id, public_id, org_id, project_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, $4, $2, 'A', now(), now())`,
			agent, "a"+agent.String()[:8], scope[0], scope[1])
		require.NoError(t, err)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO agent_versions (id, public_id, org_id, project_id, agent_id, version, config, created_at) VALUES ($1, $2, $3, $4, $5, 1, '{}', now())`,
		s.versionA, "v"+s.versionA.String()[:8], s.orgA, s.prjA, s.agentA)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO end_users (id, public_id, org_id, project_id, created_at, updated_at) VALUES ($1, $2, $3, $4, now(), now())`,
		s.endUserA, "e"+s.endUserA.String()[:8], s.orgA, s.prjA)
	require.NoError(t, err)
	return s
}

func (s runScope) thread(t *testing.T, db *sql.DB, key string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := db.ExecContext(t.Context(), `INSERT INTO threads (id, public_id, org_id, project_id, agent_id, kind, external_key, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, 'api', $6, now(), now())`,
		id, "t"+id.String()[:12], s.orgA, s.prjA, s.agentA, key)
	require.NoError(t, err)
	return id
}

func (s runScope) run(db *sql.DB, t *testing.T, thread uuid.UUID, status string) (uuid.UUID, error) {
	t.Helper()
	id := uuid.New()
	_, err := db.ExecContext(t.Context(), `INSERT INTO runs (id, public_id, org_id, project_id, thread_id, agent_version_id, status, started_at, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now(), now())`,
		id, "r"+id.String()[:12], s.orgA, s.prjA, thread, s.versionA, status)
	return id, err
}

func pgConstraint(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.ConstraintName
	}
	return ""
}

func TestThreads_LeaseShapeAndKeys(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedRunScope(t, db)
	thread := s.thread(t, db, "k1")
	lease := func(owner, until any) error {
		_, err := db.ExecContext(t.Context(), `UPDATE threads SET lease_owner = $2, lease_until = $3 WHERE id = $1`, thread, owner, until)
		return err
	}
	require.NoError(t, lease("tok", "2026-11-06T10:00:00Z"), "a lease has an owner and an end")
	require.NoError(t, lease(nil, nil), "no lease has neither")
	assert.Equal(t, "threads_lease_shape", pgConstraint(lease("tok", nil)))
	assert.Equal(t, "threads_lease_shape", pgConstraint(lease(nil, "2026-11-06T10:00:00Z")))

	_, err := db.ExecContext(t.Context(), `INSERT INTO threads (id, public_id, org_id, project_id, agent_id, kind, external_key, created_at, updated_at) VALUES ($1, 'tdup', $2, $3, $4, 'api', 'k1', now(), now())`,
		uuid.New(), s.orgA, s.prjA, s.agentA)
	assert.Equal(t, "threads_agent_external_key_key", pgConstraint(err), "one thread per agent and external key")
	_, err = db.ExecContext(t.Context(), `INSERT INTO threads (id, public_id, org_id, project_id, agent_id, kind, external_key, created_at, updated_at) VALUES ($1, 'tkind', $2, $3, $4, 'email', 'k2', now(), now())`,
		uuid.New(), s.orgA, s.prjA, s.agentA)
	assert.Equal(t, "threads_kind_check", pgConstraint(err))
	_, err = db.ExecContext(t.Context(), `INSERT INTO threads (id, public_id, org_id, project_id, agent_id, kind, external_key, created_at, updated_at) VALUES ($1, 'tfk', $2, $3, $4, 'api', 'k3', now(), now())`,
		uuid.New(), s.orgA, s.prjA, s.agentB)
	assert.Equal(t, "23503", pgCode(err), "a thread's agent is in its org and project")
}

func TestRuns_OneOpenRunPerThread(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedRunScope(t, db)
	thread := s.thread(t, db, "k")
	for _, done := range []string{"completed", "failed", "cancelled", "limit_reached", "expired"} {
		_, err := s.run(db, t, thread, done)
		require.NoError(t, err, done)
	}
	for _, open := range []string{"running", "interrupted", "awaiting_approval"} {
		first, err := s.run(db, t, thread, open)
		require.NoError(t, err, open)
		_, err = s.run(db, t, thread, "running")
		assert.Equal(t, "runs_one_open_per_thread", pgConstraint(err), "a second open run beside %s", open)
		_, err = db.ExecContext(t.Context(), `UPDATE runs SET status = 'completed' WHERE id = $1`, first)
		require.NoError(t, err)
	}
	_, err := s.run(db, t, thread, "queued")
	assert.Equal(t, "runs_status_check", pgConstraint(err))

	run, err := s.run(db, t, thread, "running")
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `UPDATE runs SET attempt = 0 WHERE id = $1`, run)
	assert.Equal(t, "runs_attempt_positive", pgConstraint(err))
	_, err = db.ExecContext(t.Context(), `UPDATE runs SET idempotency_key = 'idem' WHERE id = $1`, run)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `UPDATE runs SET idempotency_key = 'idem' WHERE thread_id = $1 AND status = 'completed' AND id = (SELECT id FROM runs WHERE thread_id = $1 AND status = 'completed' LIMIT 1)`, thread)
	assert.Equal(t, "runs_thread_idempotency_key", pgConstraint(err), "one run per thread and idempotency key")
}

func TestMessages_UniqueKeysAndChecks(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedRunScope(t, db)
	thread, other := s.thread(t, db, "k1"), s.thread(t, db, "k2")
	run, err := s.run(db, t, thread, "running")
	require.NoError(t, err)
	insert := func(thread uuid.UUID, seq int, body string, ext, runID, toolCall any) error {
		_, err := db.ExecContext(t.Context(), `INSERT INTO messages (id, org_id, project_id, thread_id, seq, role, body, speaker_id, status, external_message_id, run_id, tool_call_id, created_at)
			VALUES ($1, $2, $3, $4, $5, 'user', $6, $7, 'pending', $8, $9, $10, now())`,
			uuid.New(), s.orgA, s.prjA, thread, seq, body, s.endUserA, ext, runID, toolCall)
		return err
	}
	require.NoError(t, insert(thread, 1, `{"role":"user"}`, "wamid.1", nil, nil))
	assert.Equal(t, "messages_thread_seq_key", pgConstraint(insert(thread, 1, `{}`, nil, nil, nil)))
	assert.Equal(t, "messages_thread_external_message_id_key", pgConstraint(insert(thread, 2, `{}`, "wamid.1", nil, nil)), "a duplicate inbound message is refused")
	require.NoError(t, insert(other, 1, `{}`, "wamid.1", nil, nil), "the same external id on another thread is fine")
	require.NoError(t, insert(thread, 3, `{}`, nil, run, "call_1"))
	assert.Equal(t, "messages_run_tool_call_id_key", pgConstraint(insert(thread, 4, `{}`, nil, run, "call_1")), "a duplicate checkpoint is refused")
	assert.Equal(t, "messages_body_object", pgConstraint(insert(thread, 5, `[]`, nil, nil, nil)))
	_, err = db.ExecContext(t.Context(), `INSERT INTO messages (id, org_id, project_id, thread_id, seq, role, body, status, created_at) VALUES ($1, $2, $3, $4, 6, 'robot', '{}', 'pending', now())`,
		uuid.New(), s.orgA, s.prjA, thread)
	assert.Equal(t, "messages_role_check", pgConstraint(err))

	_, err = db.ExecContext(t.Context(), `DELETE FROM end_users WHERE id = $1`, s.endUserA)
	require.NoError(t, err)
	var speakers, orgs int
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT count(speaker_id), count(org_id) FROM messages WHERE thread_id = $1`, thread).Scan(&speakers, &orgs))
	assert.Zero(t, speakers, "deleting an end user clears the speaker")
	assert.Equal(t, 2, orgs, "and keeps the message's org")
}

func TestThreads_DeletingTheAgentCascades(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedRunScope(t, db)
	thread := s.thread(t, db, "k")
	run, err := s.run(db, t, thread, "running")
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `INSERT INTO messages (id, org_id, project_id, thread_id, seq, role, body, status, run_id, created_at) VALUES ($1, $2, $3, $4, 1, 'user', '{}', 'consumed', $5, now())`,
		uuid.New(), s.orgA, s.prjA, thread, run)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `INSERT INTO thread_snapshots (thread_id, org_id, project_id, agent_version_id, sdk_version, blob, updated_at) VALUES ($1, $2, $3, $4, '1.0.0', '\x01', now())`,
		thread, s.orgA, s.prjA, s.versionA)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `DELETE FROM agents WHERE id = $1`, s.agentA)
	require.NoError(t, err)
	for _, table := range []string{"threads", "runs", "messages", "thread_snapshots"} {
		var n int
		require.NoError(t, db.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table+` WHERE org_id = $1`, s.orgA).Scan(&n))
		assert.Zero(t, n, table)
	}
}

func TestThreads_ActorBindingSurvivesItsEndUserAsNull(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedRunScope(t, db)
	thread := s.thread(t, db, "bound")
	_, err := db.ExecContext(t.Context(), `UPDATE threads SET actor_end_user_id = $2 WHERE id = $1`, thread, s.endUserA)
	require.NoError(t, err)
	otherProjectUser := uuid.New()
	_, err = db.ExecContext(t.Context(), `INSERT INTO end_users (id, public_id, org_id, project_id, created_at, updated_at) VALUES ($1, $2, $3, $4, now(), now())`,
		otherProjectUser, "e"+otherProjectUser.String()[:8], s.orgB, s.prjB)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `UPDATE threads SET actor_end_user_id = $2 WHERE id = $1`, thread, otherProjectUser)
	assert.Equal(t, "23503", pgCode(err), "a thread is bound only to an end user of its own project")

	_, err = db.ExecContext(t.Context(), `DELETE FROM end_users WHERE id = $1`, s.endUserA)
	require.NoError(t, err)
	var actor sql.NullString
	var org uuid.UUID
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT actor_end_user_id, org_id FROM threads WHERE id = $1`, thread).Scan(&actor, &org))
	assert.False(t, actor.Valid, "deleting the end user clears the binding")
	assert.Equal(t, s.orgA, org, "and keeps the thread's org")
}

func TestMessages_RequestHashIsOptional(t *testing.T) {
	db, _ := pgtest.Migrated(t)
	s := seedRunScope(t, db)
	thread := s.thread(t, db, "k")
	_, err := db.ExecContext(t.Context(), `INSERT INTO messages (id, org_id, project_id, thread_id, seq, role, body, status, external_message_id, request_hash, created_at)
		VALUES ($1, $2, $3, $4, 1, 'user', '{}', 'pending', 'idem:k1', '\x0102', now())`, uuid.New(), s.orgA, s.prjA, thread)
	require.NoError(t, err)
	var hash []byte
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT request_hash FROM messages WHERE external_message_id = 'idem:k1'`).Scan(&hash))
	assert.Equal(t, []byte{1, 2}, hash)
}
