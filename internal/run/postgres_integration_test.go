//go:build integration

package run_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/pgtest"
)

const ttl = time.Minute

type pgFixture struct {
	pool   db.Pool
	admin  *sql.DB
	schema string
	store  run.Store
}

type scope struct {
	org, project, agent, version uuid.UUID
}

func newPgFixture(t *testing.T) *pgFixture {
	t.Helper()
	pool, admin, schemaName := pgtest.MigratedRLS(t)
	return &pgFixture{pool: pool, admin: admin, schema: schemaName, store: run.NewStore(pool, schemaName, "")}
}

func in(orgID uuid.UUID) context.Context {
	return tenant.Into(context.Background(), tenant.Context{OrgID: orgID})
}

func (f *pgFixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	_, err := f.admin.ExecContext(t.Context(), q, args...)
	require.NoError(t, err)
}

func (f *pgFixture) seed(t *testing.T) scope {
	t.Helper()
	s := scope{org: uuid.New(), project: uuid.New(), agent: uuid.New(), version: uuid.New()}
	p := f.schema + "."
	f.exec(t, `INSERT INTO `+p+`orgs (id, public_id, slug, name, created_at, updated_at) VALUES ($1, $2, $2, 'O', now(), now())`, s.org, "o"+s.org.String()[:12])
	f.exec(t, `INSERT INTO `+p+`projects (id, public_id, org_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, 'p', 'P', now(), now())`, s.project, "p"+s.project.String()[:12], s.org)
	f.exec(t, `INSERT INTO `+p+`agents (id, public_id, org_id, project_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, $4, 'a', 'A', now(), now())`, s.agent, "a"+s.agent.String()[:12], s.org, s.project)
	f.exec(t, `INSERT INTO `+p+`agent_versions (id, public_id, org_id, project_id, agent_id, version, config, created_at) VALUES ($1, $2, $3, $4, $5, 1, '{}', now())`, s.version, "v"+s.version.String()[:12], s.org, s.project, s.agent)
	return s
}

func (f *pgFixture) thread(t *testing.T, s scope) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.exec(t, `INSERT INTO `+f.schema+`.threads (id, public_id, org_id, project_id, agent_id, kind, external_key, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, 'api', $6, now(), now())`,
		id, "t"+id.String()[:12], s.org, s.project, s.agent, id.String())
	return id
}

func (f *pgFixture) say(t *testing.T, s scope, thread uuid.UUID) {
	t.Helper()
	_, err := f.store.AppendMessage(in(s.org), thread, userMsg("hi", ""))
	require.NoError(t, err)
}

func (f *pgFixture) claimed(t *testing.T, s scope, thread uuid.UUID) string {
	t.Helper()
	token := uuid.NewString()
	ok, err := f.store.Claim(in(s.org), thread, token, ttl)
	require.NoError(t, err)
	require.True(t, ok)
	return token
}

func (f *pgFixture) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, f.admin.QueryRowContext(t.Context(), q, args...).Scan(&n))
	return n
}

func TestStore_AppendAssignsSeqInOrder(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	var wg sync.WaitGroup
	seqs := make([]int64, 10)
	for i := range seqs {
		wg.Go(func() {
			seq, err := f.store.AppendMessage(ctx, thread, userMsg("hi", ""))
			assert.NoError(t, err)
			seqs[i] = seq
		})
	}
	wg.Wait()
	seen := map[int64]bool{}
	for _, seq := range seqs {
		seen[seq] = true
	}
	for want := int64(1); want <= 10; want++ {
		assert.True(t, seen[want], "seq %d assigned once", want)
	}
	next, err := f.store.AppendMessage(ctx, thread, userMsg("last", ""))
	require.NoError(t, err)
	assert.Equal(t, int64(11), next)
}

func TestStore_DuplicateExternalIDIsNoOp(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	first, err := f.store.AppendMessage(ctx, thread, userMsg("hi", "wamid.1"))
	require.NoError(t, err)
	again, err := f.store.AppendMessage(ctx, thread, userMsg("hi again", "wamid.1"))
	require.NoError(t, err)
	assert.Equal(t, first, again)
	next, err := f.store.AppendMessage(ctx, thread, userMsg("other", "wamid.2"))
	require.NoError(t, err)
	assert.Equal(t, first+1, next, "the duplicate used no seq")
	assert.Equal(t, 2, f.count(t, `SELECT count(*) FROM `+f.schema+`.messages WHERE thread_id = $1`, thread))
}

func TestStore_TwoClaimsOneWins(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	var wg sync.WaitGroup
	wins := make([]bool, 2)
	for i := range wins {
		wg.Go(func() {
			ok, err := f.store.Claim(ctx, thread, uuid.NewString(), ttl)
			assert.NoError(t, err)
			wins[i] = ok
		})
	}
	wg.Wait()
	assert.NotEqual(t, wins[0], wins[1], "exactly one claim wins")

	f.exec(t, `UPDATE `+f.schema+`.threads SET lease_until = now() - interval '1 second' WHERE id = $1`, thread)
	ok, err := f.store.Claim(ctx, thread, uuid.NewString(), ttl)
	require.NoError(t, err)
	assert.True(t, ok, "an expired lease can be claimed")
}

func TestStore_WriteAfterLeaseLostRefused(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	_, err := f.store.AppendMessage(ctx, thread, userMsg("hi", ""))
	require.NoError(t, err)
	stale := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err = f.store.StartRun(ctx, stale, r)
	require.NoError(t, err)
	f.exec(t, `UPDATE `+f.schema+`.threads SET lease_until = now() - interval '1 second' WHERE id = $1`, thread)
	fresh := f.claimed(t, s, thread)
	before := f.count(t, `SELECT count(*) FROM `+f.schema+`.messages WHERE thread_id = $1`, thread)

	_, err = f.store.Heartbeat(ctx, thread, stale, ttl)
	assert.True(t, run.IsLeaseLostError(err), "heartbeat: %v", err)
	_, err = f.store.StartRun(ctx, stale, run.New(s.org, s.project, thread, s.version, time.Now()))
	assert.True(t, run.IsLeaseLostError(err), "start: %v", err)
	err = f.store.PutCheckpoint(ctx, stale, r.ID, "call_1", []run.Message{toolMsg("call_1")})
	assert.True(t, run.IsLeaseLostError(err), "checkpoint: %v", err)
	require.NoError(t, r.Move(run.StatusCompleted, time.Now()))
	err = f.store.Settle(ctx, stale, r, run.Usage{InputTokens: 5}, nil)
	assert.True(t, run.IsLeaseLostError(err), "settle: %v", err)
	err = f.store.Release(ctx, thread, stale)
	assert.True(t, run.IsLeaseLostError(err), "release: %v", err)

	assert.Equal(t, before, f.count(t, `SELECT count(*) FROM `+f.schema+`.messages WHERE thread_id = $1`, thread))
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.runs WHERE thread_id = $1 AND status = 'running' AND input_tokens = 0`, thread))
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.threads WHERE id = $1 AND lease_owner = $2`, thread, fresh), "the new lease stands")
}

func TestStore_StartRunTakesAllPendingInOrder(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	for _, text := range []string{"one", "two", "three"} {
		_, err := f.store.AppendMessage(ctx, thread, userMsg(text, ""))
		require.NoError(t, err)
	}
	token := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	r.Prices = &run.Prices{Input: new(int64(3))}
	taken, err := f.store.StartRun(ctx, token, r)
	require.NoError(t, err)
	require.Len(t, taken, 3)
	for i, m := range taken {
		assert.Equal(t, int64(i+1), m.Seq)
		assert.Equal(t, run.MessageConsumed, m.Status)
		assert.Equal(t, r.ID, *m.RunID)
	}
	assert.Contains(t, string(taken[0].Body), "one")
	assert.Contains(t, string(taken[2].Body), "three")
	assert.Zero(t, f.count(t, `SELECT count(*) FROM `+f.schema+`.messages WHERE thread_id = $1 AND status = 'pending'`, thread))

	_, err = f.store.AppendMessage(ctx, thread, userMsg("late", ""))
	require.NoError(t, err)
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.messages WHERE thread_id = $1 AND status = 'pending'`, thread), "a message after the start stays pending")

	open, err := f.store.OpenRun(ctx, thread)
	require.NoError(t, err)
	assert.Nil(t, open, "a running run is not resumable")
	require.NoError(t, r.Move(run.StatusInterrupted, time.Now()))
	require.NoError(t, f.store.Settle(ctx, token, r, run.Usage{InputTokens: 10, Estimated: true}, nil))
	open, err = f.store.OpenRun(ctx, thread)
	require.NoError(t, err)
	require.NotNil(t, open)
	assert.Equal(t, r.ID, open.ID)
	assert.Equal(t, run.StatusInterrupted, open.Status)
	assert.Equal(t, int64(10), open.Usage.InputTokens)
	assert.True(t, open.Usage.Estimated)
	require.NotNil(t, open.Prices)
	assert.Equal(t, int64(3), *open.Prices.Input)
	assert.Nil(t, open.Prices.Output)
}

func TestStore_CheckpointDuplicateIsNoOp(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	f.say(t, s, thread)
	token := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err := f.store.StartRun(ctx, token, r)
	require.NoError(t, err)
	require.NoError(t, f.store.PutCheckpoint(ctx, token, r.ID, "call_1", []run.Message{toolMsg("call_1")}))
	require.NoError(t, f.store.PutCheckpoint(ctx, token, r.ID, "call_1", []run.Message{toolMsg("call_1")}))
	require.NoError(t, f.store.PutCheckpoint(ctx, token, r.ID, "call_2", []run.Message{toolMsg("call_2")}))
	stored, err := f.store.StoredResults(ctx, r.ID)
	require.NoError(t, err)
	require.Len(t, stored, 2)
	assert.Equal(t, "call_1", stored[0].ToolUseID)
	assert.Equal(t, "call_2", stored[1].ToolUseID)
	assert.Contains(t, string(stored[0].Body), `"toolCallId": "call_1"`)
}

func TestStore_SettleIsOneTransaction(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	f.say(t, s, thread)
	token := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err := f.store.StartRun(ctx, token, r)
	require.NoError(t, err)
	require.NoError(t, r.Move(run.StatusCompleted, time.Now()))
	bad := run.Message{Role: run.RoleAssistant, Body: json.RawMessage(`[]`)}
	err = f.store.Settle(ctx, token, r, run.Usage{InputTokens: 7}, []run.Message{assistantMsg("ok"), bad})
	require.Error(t, err, "a body that is not an object fails the insert")
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.runs WHERE id = $1 AND status = 'running' AND input_tokens = 0`, r.ID))
	assert.Zero(t, f.count(t, `SELECT count(*) FROM `+f.schema+`.messages WHERE thread_id = $1 AND role = 'assistant'`, thread))

	r.Prices = &run.Prices{Input: new(int64(1_000_000)), Output: new(int64(2_000_000))}
	require.NoError(t, f.store.Settle(ctx, token, r, run.Usage{InputTokens: 7, OutputTokens: 3}, []run.Message{assistantMsg("ok")}))
	var status string
	var cost int64
	var ended sql.NullTime
	require.NoError(t, f.admin.QueryRowContext(t.Context(), `SELECT status, cost_micros, ended_at FROM `+f.schema+`.runs WHERE id = $1`, r.ID).Scan(&status, &cost, &ended))
	assert.Equal(t, "completed", status)
	assert.Equal(t, int64(13), cost)
	assert.True(t, ended.Valid)
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.messages WHERE thread_id = $1 AND run_id = $2 AND role = 'assistant' AND status = 'consumed'`, thread, r.ID))

	err = f.store.Settle(ctx, token, run.New(s.org, s.project, thread, s.version, time.Now()), run.Usage{}, nil)
	assert.Error(t, err, "settle needs a terminal or interrupted run")
}

func TestStore_ExpiredAndPendingUnleased(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	idle, leased, expired, quiet := f.thread(t, s), f.thread(t, s), f.thread(t, s), f.thread(t, s)
	for _, th := range []uuid.UUID{idle, leased, expired} {
		_, err := f.store.AppendMessage(ctx, th, userMsg("hi", ""))
		require.NoError(t, err)
	}
	f.claimed(t, s, leased)
	f.claimed(t, s, expired)
	f.exec(t, `UPDATE `+f.schema+`.threads SET lease_until = now() - interval '1 second' WHERE id = $1`, expired)
	_ = quiet

	got, err := f.store.Expired(ctx, time.Now())
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{expired}, got)
	got, err = f.store.PendingUnleased(ctx, time.Now(), 10)
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{idle, expired}, got)
	got, err = f.store.PendingUnleased(ctx, time.Now(), 1)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestStore_OtherTenantInvisible(t *testing.T) {
	f := newPgFixture(t)
	a, b := f.seed(t), f.seed(t)
	thread := f.thread(t, a)
	_, err := f.store.AppendMessage(in(a.org), thread, userMsg("hi", ""))
	require.NoError(t, err)

	_, err = f.store.AppendMessage(in(b.org), thread, userMsg("x", ""))
	assert.True(t, run.IsThreadNotFoundError(err), "append: %v", err)
	ok, err := f.store.Claim(in(b.org), thread, uuid.NewString(), ttl)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.True(t, run.IsThreadNotFoundError(f.store.RequestCancel(in(b.org), thread)))
	got, err := f.store.PendingUnleased(in(b.org), time.Now(), 10)
	require.NoError(t, err)
	assert.Empty(t, got)
	open, err := f.store.OpenRun(in(b.org), thread)
	require.NoError(t, err)
	assert.Nil(t, open)
}

func TestStore_CancelReadByHeartbeat(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	f.say(t, s, thread)
	token := f.claimed(t, s, thread)
	cancel, err := f.store.Heartbeat(ctx, thread, token, ttl)
	require.NoError(t, err)
	assert.False(t, cancel)
	require.NoError(t, f.store.RequestCancel(ctx, thread))
	cancel, err = f.store.Heartbeat(ctx, thread, token, ttl)
	require.NoError(t, err)
	assert.True(t, cancel)
	require.NoError(t, f.store.Release(ctx, thread, token))
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.threads WHERE id = $1 AND lease_owner IS NULL AND lease_until IS NULL`, thread))
}

func TestStore_ResumeKeepsTheRunsMessagesAndResults(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	_, err := f.store.AppendMessage(ctx, thread, userMsg("first", "wamid.1"))
	require.NoError(t, err)
	token := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	r.IdempotencyKey = "idem-1"
	_, err = f.store.StartRun(ctx, token, r)
	require.NoError(t, err)
	require.NoError(t, f.store.PutCheckpoint(ctx, token, r.ID, "call_1", []run.Message{toolMsg("call_1")}))
	require.NoError(t, r.Move(run.StatusInterrupted, time.Now()))
	require.NoError(t, f.store.Settle(ctx, token, r, run.Usage{InputTokens: 4, Estimated: true}, nil))
	require.NoError(t, f.store.Release(ctx, thread, token))
	_, err = f.store.AppendMessage(ctx, thread, userMsg("second", ""))
	require.NoError(t, err)

	token = f.claimed(t, s, thread)
	open, err := f.store.OpenRun(ctx, thread)
	require.NoError(t, err)
	require.NotNil(t, open)
	assert.Equal(t, "idem-1", open.IdempotencyKey)
	assert.Nil(t, open.Prices)
	require.NoError(t, open.Move(run.StatusRunning, time.Now()))
	require.Equal(t, int32(2), open.Attempt)
	taken, err := f.store.StartRun(ctx, token, open)
	require.NoError(t, err)
	require.Len(t, taken, 1, "a resumed run keeps its own messages, not the new pending one")
	assert.Equal(t, "wamid.1", taken[0].ExternalMessageID)
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.runs WHERE id = $1 AND status = 'running' AND attempt = 2`, r.ID))
	stored, err := f.store.StoredResults(ctx, r.ID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	_, err = f.store.StartRun(ctx, token, open)
	assert.Error(t, err, "only an interrupted run resumes")

	require.NoError(t, f.store.RequestCancel(ctx, thread))
	require.NoError(t, open.Move(run.StatusCancelled, time.Now()))
	open.ErrorCode = "RUN001"
	require.NoError(t, f.store.Settle(ctx, token, open, run.Usage{}, nil))
	cancel, err := f.store.Heartbeat(ctx, thread, token, ttl)
	require.NoError(t, err)
	assert.False(t, cancel, "settling a cancelled run clears the request")
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.runs WHERE id = $1 AND error_code = 'RUN001' AND latency_ms IS NOT NULL AND cost_micros IS NULL`, r.ID))

	err = f.store.PutCheckpoint(ctx, token, uuid.New(), "call_9", []run.Message{toolMsg("call_9")})
	assert.True(t, run.IsNotFoundError(err), "checkpoint of an unknown run: %v", err)
	err = f.store.PutCheckpoint(ctx, token, r.ID, "call_9", []run.Message{toolMsg("call_8")})
	assert.Error(t, err, "a checkpoint must carry its tool call")
}

func (f *pgFixture) pendingThreads(t *testing.T, s scope, n int) []uuid.UUID {
	t.Helper()
	out := make([]uuid.UUID, n)
	for i := range out {
		out[i] = f.thread(t, s)
		_, err := f.store.AppendMessage(in(s.org), out[i], userMsg("hi", ""))
		require.NoError(t, err)
	}
	return out
}

func TestClaim_OverConcurrencySkipsNotLoses(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	threads := f.pendingThreads(t, s, 5)
	tokens := make([]string, 4)
	for i := range tokens {
		tokens[i] = f.claimed(t, s, threads[i])
	}
	ok, err := f.store.Claim(ctx, threads[4], uuid.NewString(), ttl)
	require.NoError(t, err)
	assert.False(t, ok, "a fifth live lease in the project is skipped")
	pending, err := f.store.PendingUnleased(ctx, time.Now(), 10)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{threads[4]}, pending, "its message stays pending for the reconciler")

	other := f.seed(t)
	elsewhere := f.pendingThreads(t, other, 1)[0]
	f.claimed(t, other, elsewhere)

	require.NoError(t, f.store.Release(ctx, threads[0], tokens[0]))
	ok, err = f.store.Claim(ctx, threads[4], uuid.NewString(), ttl)
	require.NoError(t, err)
	assert.True(t, ok, "once a lease is released the skipped thread is claimed")
}

func TestClaim_ConcurrencyCapUnderRace(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	threads := f.pendingThreads(t, s, 10)
	var wg sync.WaitGroup
	wins := make([]bool, len(threads))
	for i, th := range threads {
		wg.Go(func() {
			ok, err := f.store.Claim(ctx, th, uuid.NewString(), ttl)
			assert.NoError(t, err)
			wins[i] = ok
		})
	}
	wg.Wait()
	n := 0
	for _, w := range wins {
		if w {
			n++
		}
	}
	assert.Equal(t, run.DefaultMaxConcurrentPerProject, n, "parallel claims never pass the project cap")
	assert.Equal(t, run.DefaultMaxConcurrentPerProject, f.count(t, `SELECT count(*) FROM `+f.schema+`.threads WHERE project_id = $1 AND lease_until > now()`, s.project))
}

func (f *pgFixture) expire(t *testing.T, thread uuid.UUID) {
	t.Helper()
	f.exec(t, `UPDATE `+f.schema+`.threads SET lease_until = now() - interval '1 second' WHERE id = $1`, thread)
}

func TestStore_ReclaimExpiredLease(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	f.say(t, s, thread)
	token := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err := f.store.StartRun(ctx, token, r)
	require.NoError(t, err)

	ok, err := f.store.Reclaim(ctx, thread)
	require.NoError(t, err)
	assert.False(t, ok, "a live lease is not reclaimed")

	f.expire(t, thread)
	ok, err = f.store.Reclaim(ctx, thread)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.threads WHERE id = $1 AND lease_owner IS NULL AND lease_until IS NULL`, thread))
	open, err := f.store.OpenRun(ctx, thread)
	require.NoError(t, err)
	require.NotNil(t, open, "the running run is now interrupted and resumable")
	assert.Equal(t, r.ID, open.ID)
	_, err = f.store.Heartbeat(ctx, thread, token, ttl)
	assert.True(t, run.IsLeaseLostError(err), "the old holder's writes are fenced out")

	ok, err = f.store.Reclaim(ctx, thread)
	require.NoError(t, err)
	assert.False(t, ok, "nothing left to reclaim")
	ok, err = f.store.Reclaim(in(f.seed(t).org), thread)
	require.NoError(t, err)
	assert.False(t, ok, "another tenant reclaims nothing")
}

func TestStore_LateCancelDoesNotKillTheNextRun(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	f.say(t, s, thread)
	token := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err := f.store.StartRun(ctx, token, r)
	require.NoError(t, err)
	require.NoError(t, f.store.RequestCancel(ctx, thread), "the cancel arrives as the turn finishes")
	require.NoError(t, r.Move(run.StatusCompleted, time.Now()))
	require.NoError(t, f.store.Settle(ctx, token, r, run.Usage{}, nil))
	cancel, err := f.store.Heartbeat(ctx, thread, token, ttl)
	require.NoError(t, err)
	assert.False(t, cancel, "settling any run clears the request")

	f.exec(t, `UPDATE `+f.schema+`.threads SET cancel_requested = true WHERE id = $1`, thread)
	_, err = f.store.AppendMessage(ctx, thread, userMsg("next", ""))
	require.NoError(t, err)
	_, err = f.store.StartRun(ctx, token, run.New(s.org, s.project, thread, s.version, time.Now()))
	require.NoError(t, err)
	cancel, err = f.store.Heartbeat(ctx, thread, token, ttl)
	require.NoError(t, err)
	assert.False(t, cancel, "a new run starts with no stale request")
}

func TestStore_CancelOnAnIdleThreadOnlyFlagsIt(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	f.say(t, s, thread)
	token := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err := f.store.StartRun(ctx, token, r)
	require.NoError(t, err)
	require.NoError(t, r.Move(run.StatusInterrupted, time.Now()))
	require.NoError(t, f.store.Settle(ctx, token, r, run.Usage{}, nil))
	require.NoError(t, f.store.Release(ctx, thread, token))

	require.NoError(t, f.store.RequestCancel(ctx, thread))
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.runs WHERE id = $1 AND status = 'interrupted'`, r.ID),
		"the store never settles a cancel itself; the service does, so the event is emitted")
	th, err := f.store.Thread(ctx, thread)
	require.NoError(t, err)
	assert.True(t, th.CancelRequested)
	assert.True(t, run.IsThreadNotFoundError(f.store.RequestCancel(ctx, uuid.New())))
}

func TestStore_ReconcileFindsAnInterruptedRunWithNothingPending(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	f.say(t, s, thread)
	has, err := f.store.HasPending(ctx, thread)
	require.NoError(t, err)
	assert.True(t, has)
	token := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err = f.store.StartRun(ctx, token, r)
	require.NoError(t, err)
	has, err = f.store.HasPending(ctx, thread)
	require.NoError(t, err)
	assert.False(t, has, "the run took the message")
	require.NoError(t, r.Move(run.StatusInterrupted, time.Now()))
	require.NoError(t, f.store.Settle(ctx, token, r, run.Usage{}, nil))
	require.NoError(t, f.store.Release(ctx, thread, token))

	got, err := f.store.PendingUnleased(ctx, time.Now(), 10)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{thread}, got, "an interrupted run is kicked even with no message pending")
}

func TestStore_NewRunNeedsAPendingMessage(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	token := f.claimed(t, s, thread)
	_, err := f.store.StartRun(in(s.org), token, run.New(s.org, s.project, thread, s.version, time.Now()))
	assert.ErrorIs(t, err, run.ErrNothingPending)
	assert.Zero(t, f.count(t, `SELECT count(*) FROM `+f.schema+`.runs WHERE thread_id = $1`, thread), "nothing is stored")
}

func TestStore_ThreadAndHistory(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	th, err := f.store.Thread(ctx, thread)
	require.NoError(t, err)
	assert.Equal(t, run.Thread{
		ID: thread, PublicID: "t" + thread.String()[:12], OrgID: s.org, ProjectID: s.project, AgentID: s.agent,
		Kind: "api", ExternalKey: thread.String(),
	}, *th)
	_, err = f.store.Thread(in(f.seed(t).org), thread)
	assert.True(t, run.IsThreadNotFoundError(err))

	f.say(t, s, thread)
	token := f.claimed(t, s, thread)
	first := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err = f.store.StartRun(ctx, token, first)
	require.NoError(t, err)
	require.NoError(t, first.Move(run.StatusCompleted, time.Now()))
	require.NoError(t, f.store.Settle(ctx, token, first, run.Usage{}, []run.Message{assistantMsg("hello back")}))
	f.say(t, s, thread)
	second := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err = f.store.StartRun(ctx, token, second)
	require.NoError(t, err)

	history, err := f.store.History(ctx, thread, second.ID)
	require.NoError(t, err)
	require.Len(t, history, 2, "the earlier turn only, in order")
	assert.Equal(t, run.RoleUser, history[0].Role)
	assert.Equal(t, run.RoleAssistant, history[1].Role)
}

func TestStore_SweepRecoversAnOrphanRunningRun(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	f.say(t, s, thread)
	token := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err := f.store.StartRun(ctx, token, r)
	require.NoError(t, err)
	require.NoError(t, f.store.Release(ctx, thread, token), "a crash path that released without settling")

	expired, err := f.store.Expired(ctx, time.Now())
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{thread}, expired, "an orphan running run is found with no lease at all")
	ok, err := f.store.Reclaim(ctx, thread)
	require.NoError(t, err)
	assert.True(t, ok)
	open, err := f.store.OpenRun(ctx, thread)
	require.NoError(t, err)
	require.NotNil(t, open)
	assert.Equal(t, r.ID, open.ID)

	token = f.claimed(t, s, thread)
	require.NoError(t, open.Move(run.StatusRunning, time.Now()))
	_, err = f.store.StartRun(ctx, token, open)
	require.NoError(t, err, "the next turn proceeds")
	expired, err = f.store.Expired(ctx, time.Now())
	require.NoError(t, err)
	assert.Empty(t, expired, "a running run under a live lease is not an orphan")
}

func TestClaim_JoinsTheCallersTransaction(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	err := db.NewTransactor(f.pool).RunInTx(in(s.org), func(ctx context.Context) error {
		ok, err := f.store.Claim(ctx, thread, "tok", ttl)
		require.NoError(t, err)
		require.True(t, ok)
		return errors.New("roll back")
	})
	require.Error(t, err)
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.threads WHERE id = $1 AND lease_owner IS NULL`, thread),
		"the claim rolled back with the caller's transaction")
}
