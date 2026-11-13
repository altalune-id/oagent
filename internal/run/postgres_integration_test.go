//go:build integration

package run_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/testutil/fakes"
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
	f.exec(t, `INSERT INTO `+p+`agent_versions (id, public_id, org_id, project_id, agent_id, version, config, created_at, kind) VALUES ($1, $2, $3, $4, $5, 1, '{}', now(), 'published')`, s.version, "v"+s.version.String()[:12], s.org, s.project, s.agent)
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
	claim, err := f.store.Claim(in(s.org), thread, token, ttl)
	ok := claim == run.Claimed
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
			claim, err := f.store.Claim(ctx, thread, uuid.NewString(), ttl)

			ok := claim == run.Claimed
			assert.NoError(t, err)
			wins[i] = ok
		})
	}
	wg.Wait()
	assert.NotEqual(t, wins[0], wins[1], "exactly one claim wins")

	f.exec(t, `UPDATE `+f.schema+`.threads SET lease_until = now() - interval '1 second' WHERE id = $1`, thread)
	claim, err := f.store.Claim(ctx, thread, uuid.NewString(), ttl)
	ok := claim == run.Claimed
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
	claim, err := f.store.Claim(in(b.org), thread, uuid.NewString(), ttl)
	ok := claim == run.Claimed
	require.NoError(t, err)
	assert.False(t, ok)
	assert.True(t, run.IsThreadNotFoundError(f.store.RequestCancel(in(b.org), thread, uuid.New())))
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
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err := f.store.StartRun(ctx, token, r)
	require.NoError(t, err)
	cancel, err := f.store.Heartbeat(ctx, thread, token, ttl)
	require.NoError(t, err)
	assert.False(t, cancel)
	require.NoError(t, f.store.RequestCancel(ctx, thread, r.ID))
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

	require.NoError(t, f.store.RequestCancel(ctx, thread, open.ID))
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
	claim, err := f.store.Claim(ctx, threads[4], uuid.NewString(), ttl)
	ok := claim == run.Claimed
	require.NoError(t, err)
	assert.False(t, ok, "a fifth live lease in the project is skipped")
	pending, err := f.store.PendingUnleased(ctx, time.Now(), 10)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{threads[4]}, pending, "its message stays pending for the reconciler")

	other := f.seed(t)
	elsewhere := f.pendingThreads(t, other, 1)[0]
	f.claimed(t, other, elsewhere)

	require.NoError(t, f.store.Release(ctx, threads[0], tokens[0]))
	claim, err = f.store.Claim(ctx, threads[4], uuid.NewString(), ttl)
	ok = claim == run.Claimed
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
			claim, err := f.store.Claim(ctx, th, uuid.NewString(), ttl)

			ok := claim == run.Claimed
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
	require.NoError(t, f.store.RequestCancel(ctx, thread, r.ID), "the cancel arrives as the turn finishes")
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

	require.NoError(t, f.store.RequestCancel(ctx, thread, r.ID))
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.runs WHERE id = $1 AND status = 'interrupted'`, r.ID),
		"the store never settles a cancel itself; the service does, so the event is emitted")
	th, err := f.store.Thread(ctx, thread)
	require.NoError(t, err)
	assert.True(t, th.CancelRequested)
	assert.True(t, run.IsThreadNotFoundError(f.store.RequestCancel(ctx, uuid.New(), r.ID)))
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
		claim, err := f.store.Claim(ctx, thread, "tok", ttl)

		ok := claim == run.Claimed
		require.NoError(t, err)
		require.True(t, ok)
		return errors.New("roll back")
	})
	require.Error(t, err)
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.threads WHERE id = $1 AND lease_owner IS NULL`, thread),
		"the claim rolled back with the caller's transaction")
}

func (f *pgFixture) endUser(t *testing.T, s scope) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.exec(t, `INSERT INTO `+f.schema+`.end_users (id, public_id, org_id, project_id, created_at, updated_at) VALUES ($1, $2, $3, $4, now(), now())`,
		id, "e"+id.String()[:12], s.org, s.project)
	return id
}

func TestStore_ClaimReportsWhy(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	threads := f.pendingThreads(t, s, 5)
	got, err := f.store.Claim(ctx, threads[0], "first", ttl)
	require.NoError(t, err)
	assert.Equal(t, run.Claimed, got)
	got, err = f.store.Claim(ctx, threads[0], "second", ttl)
	require.NoError(t, err)
	assert.Equal(t, run.LeaseLive, got, "the thread itself is held")
	for _, th := range threads[1:4] {
		f.claimed(t, s, th)
	}
	got, err = f.store.Claim(ctx, threads[4], "fifth", ttl)
	require.NoError(t, err)
	assert.Equal(t, run.ProjectBusy, got, "the thread is free but its project is at the cap")
	got, err = f.store.Claim(in(f.seed(t).org), threads[4], "stranger", ttl)
	require.NoError(t, err)
	assert.Equal(t, run.LeaseLive, got, "another tenant's thread reads as unavailable, never as a reason")
}

func TestStore_ClaimAndAppendIsAtomic(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	thread := f.thread(t, s)
	got, seq, err := f.store.ClaimAndAppend(ctx, thread, "a", ttl, userMsg("first", "idem:k1"))
	require.NoError(t, err)
	assert.Equal(t, run.Claimed, got)
	assert.Equal(t, int64(1), seq)
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.threads WHERE id = $1 AND lease_owner = 'a'`, thread))

	got, _, err = f.store.ClaimAndAppend(ctx, thread, "b", ttl, userMsg("second", ""))
	require.NoError(t, err)
	assert.Equal(t, run.LeaseLive, got)
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.messages WHERE thread_id = $1`, thread), "a refused claim inserts nothing")

	require.NoError(t, f.store.Release(ctx, thread, "a"))
	_, _, err = f.store.ClaimAndAppend(ctx, thread, "c", ttl, userMsg("again", "idem:k1"))
	dup, ok := errors.AsType[*run.DuplicateMessageError](err)
	require.True(t, ok, "%v", err)
	assert.Equal(t, int64(1), dup.Seq, "the duplicate names the message already stored")
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.messages WHERE thread_id = $1`, thread))
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.threads WHERE id = $1 AND lease_owner IS NULL`, thread),
		"the claim rolled back with the duplicate")
}

func TestStore_ConcurrentIdenticalPOSTsInsertOnce(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	thread := f.thread(t, s)
	var wg sync.WaitGroup
	results := make([]run.ClaimResult, 6)
	for i := range results {
		wg.Go(func() {
			got, _, err := f.store.ClaimAndAppend(ctx, thread, uuid.NewString(), ttl, userMsg("same", "idem:same"))
			if !errors.As(err, new(*run.DuplicateMessageError)) {
				assert.NoError(t, err)
			}
			results[i] = got
		})
	}
	wg.Wait()
	claimed := 0
	for _, r := range results {
		if r == run.Claimed {
			claimed++
		}
	}
	assert.Equal(t, 1, claimed, "exactly one POST holds the thread")
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.messages WHERE thread_id = $1 AND external_message_id = 'idem:same'`, thread))
}

func TestStore_TryThreadBindsTheMemberApartFromAPIThreads(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	member, other := f.endUser(t, s), f.endUser(t, s)
	api, err := f.store.UpsertAPIThread(ctx, s.project, s.agent, "conv-1", member)
	require.NoError(t, err)
	try, err := f.store.UpsertTryThread(ctx, s.project, s.agent, "conv-1", member)
	require.NoError(t, err)
	assert.Equal(t, "try", try.Kind)
	assert.NotEqual(t, api.ID, try.ID, "a try thread never resumes an API thread with the same key")
	again, err := f.store.UpsertTryThread(ctx, s.project, s.agent, "conv-1", member)
	require.NoError(t, err)
	assert.Equal(t, try.ID, again.ID)
	_, err = f.store.UpsertTryThread(ctx, s.project, s.agent, "conv-1", other)
	assert.True(t, run.IsThreadNotFoundError(err), "another member's try thread reads as not found: %v", err)
	got, err := f.store.ThreadByExternalKey(ctx, s.agent, "conv-1")
	require.NoError(t, err)
	assert.Equal(t, api.ID, got.ID, "the API lookup never returns a try thread")
}

func TestStore_APIThreadBindsFirstActor(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	alice, bob := f.endUser(t, s), f.endUser(t, s)
	th, err := f.store.UpsertAPIThread(ctx, s.project, s.agent, "conv-1", alice)
	require.NoError(t, err)
	assert.Equal(t, "api", th.Kind)
	assert.Equal(t, "conv-1", th.ExternalKey)
	assert.Equal(t, alice, th.ActorEndUserID)
	again, err := f.store.UpsertAPIThread(ctx, s.project, s.agent, "conv-1", alice)
	require.NoError(t, err)
	assert.Equal(t, th.ID, again.ID, "the same actor gets the same thread")

	_, err = f.store.UpsertAPIThread(ctx, s.project, s.agent, "conv-1", bob)
	assert.True(t, errors.As(err, new(*run.ThreadActorMismatchError)), "another actor on a bound thread: %v", err)
	assert.True(t, run.IsThreadNotFoundError(err), "and it reads as not found")

	f.exec(t, `DELETE FROM `+f.schema+`.end_users WHERE id = $1`, alice)
	_, err = f.store.UpsertAPIThread(ctx, s.project, s.agent, "conv-1", bob)
	assert.True(t, run.IsThreadNotFoundError(err), "a thread whose actor is gone is never handed to someone else")

	got, err := f.store.ThreadByExternalKey(ctx, s.agent, "conv-1")
	require.NoError(t, err)
	assert.Equal(t, th.ID, got.ID)
	_, err = f.store.ThreadByExternalKey(ctx, s.agent, "nope")
	assert.True(t, run.IsThreadNotFoundError(err))
	_, err = f.store.ThreadByExternalKey(in(f.seed(t).org), s.agent, "conv-1")
	assert.True(t, run.IsThreadNotFoundError(err), "another tenant sees nothing")
}

func TestStore_RunAndMessageLookups(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	thread := f.thread(t, s)
	msg := userMsg("hi", "idem:abc")
	msg.RequestHash = []byte{9, 9}
	_, _, err := f.store.ClaimAndAppend(ctx, thread, "tok", ttl, msg)
	require.NoError(t, err)
	stored, err := f.store.MessageByExternalID(ctx, thread, "idem:abc")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, []byte{9, 9}, stored.RequestHash)
	none, err := f.store.MessageByExternalID(ctx, thread, "idem:zzz")
	require.NoError(t, err)
	assert.Nil(t, none)

	r := run.New(s.org, s.project, thread, s.version, time.Now())
	r.IdempotencyKey = "abc"
	_, err = f.store.StartRun(ctx, "tok", r)
	require.NoError(t, err)
	byKey, err := f.store.RunByIdempotencyKey(ctx, thread, "abc")
	require.NoError(t, err)
	require.NotNil(t, byKey)
	assert.Equal(t, r.ID, byKey.ID)
	missing, err := f.store.RunByIdempotencyKey(ctx, thread, "other")
	require.NoError(t, err)
	assert.Nil(t, missing)

	byID, err := f.store.RunByPublicID(ctx, r.PublicID)
	require.NoError(t, err)
	assert.Equal(t, r.ID, byID.ID)
	_, err = f.store.RunByPublicID(ctx, "nope")
	assert.True(t, run.IsNotFoundError(err))
	_, err = f.store.RunByPublicID(in(f.seed(t).org), r.PublicID)
	assert.True(t, run.IsNotFoundError(err), "another tenant sees nothing")
}

func TestStore_OrphanedThreadNeverRebinds(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	alice := f.endUser(t, s)
	th, err := f.store.UpsertAPIThread(ctx, s.project, s.agent, "conv-orphan", alice)
	require.NoError(t, err)
	_, _, err = f.store.ClaimAndAppend(ctx, th.ID, "tok", ttl, userMsg("secret plans", ""))
	require.NoError(t, err)
	f.exec(t, `DELETE FROM `+f.schema+`.end_users WHERE id = $1`, alice)

	for name, actor := range map[string]uuid.UUID{"another user": f.endUser(t, s), "a fresh end user": f.endUser(t, s)} {
		_, err := f.store.UpsertAPIThread(ctx, s.project, s.agent, "conv-orphan", actor)
		assert.True(t, run.IsThreadNotFoundError(err), "%s: an orphaned thread answers not found: %v", name, err)
	}
	_, err = f.store.UpsertAPIThread(ctx, s.project, s.agent, "conv-orphan", uuid.Nil)
	assert.Error(t, err, "a caller with no actor never matches an orphan")
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.threads WHERE id = $1 AND actor_end_user_id IS NULL`, th.ID),
		"the binding is never rewritten")
}

func TestStore_StartRunRecordsTheIdempotencyKey(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	thread := f.thread(t, s)
	_, _, err := f.store.ClaimAndAppend(ctx, thread, "tok", ttl, userMsg("hi", "idem:retry-7"))
	require.NoError(t, err)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err = f.store.StartRun(ctx, "tok", r)
	require.NoError(t, err)
	assert.Equal(t, "retry-7", r.IdempotencyKey, "the run carries the key of the message it took")
	found, err := f.store.RunByIdempotencyKey(ctx, thread, "retry-7")
	require.NoError(t, err)
	require.NotNil(t, found, "a retried POST finds the run")
	assert.Equal(t, r.ID, found.ID)
}

func TestStore_ThreadMessagesPageBySeq(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	thread := f.thread(t, s)
	for _, text := range []string{"one", "two", "three"} {
		_, err := f.store.AppendMessage(ctx, thread, userMsg(text, ""))
		require.NoError(t, err)
	}
	page, err := f.store.ThreadMessages(ctx, thread, 0, 2)
	require.NoError(t, err)
	require.Len(t, page, 2)
	assert.Equal(t, []int64{1, 2}, []int64{page[0].Seq, page[1].Seq})
	page, err = f.store.ThreadMessages(ctx, thread, 2, 10)
	require.NoError(t, err)
	require.Len(t, page, 1)
	assert.Equal(t, int64(3), page[0].Seq, "pending messages are listed too, oldest first")
	page, err = f.store.ThreadMessages(in(f.seed(t).org), thread, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, page, "another tenant sees nothing")
}

func TestStore_RunByID(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	ctx := in(s.org)
	thread := f.thread(t, s)
	f.say(t, s, thread)
	token := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err := f.store.StartRun(ctx, token, r)
	require.NoError(t, err)
	got, err := f.store.RunByID(ctx, r.ID)
	require.NoError(t, err)
	assert.Equal(t, r.PublicID, got.PublicID)
	_, err = f.store.RunByID(in(f.seed(t).org), r.ID)
	assert.True(t, run.IsNotFoundError(err))
}

func TestStore_AwaitingRunOpensAndResumesWithTheSameAttempt(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	_, err := f.store.AppendMessage(ctx, thread, userMsg("record lunch", "wamid.1"))
	require.NoError(t, err)
	token := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err = f.store.StartRun(ctx, token, r)
	require.NoError(t, err)
	require.NoError(t, r.Move(run.StatusAwaitingApproval, time.Now()))
	r.ReplyState, r.ReplyKind = run.ReplyDue, run.ReplyKindPrompt
	require.NoError(t, f.store.Settle(ctx, token, r, run.Usage{}, nil), "an awaiting run settles")
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.runs WHERE id = $1 AND status = 'awaiting_approval' AND reply_kind = 'prompt'`, r.ID))
	require.NoError(t, f.store.Release(ctx, thread, token))
	_, err = f.store.AppendMessage(ctx, thread, userMsg("and coffee", ""))
	require.NoError(t, err)

	token = f.claimed(t, s, thread)
	open, err := f.store.OpenRun(ctx, thread)
	require.NoError(t, err)
	require.NotNil(t, open, "an awaiting run is open")
	assert.Equal(t, r.ID, open.ID)
	require.NoError(t, open.Move(run.StatusRunning, time.Now()))
	taken, err := f.store.ResumeApproval(ctx, token, open)
	require.NoError(t, err)
	require.Len(t, taken, 1, "the run's own message, not the new one")
	assert.Equal(t, "wamid.1", taken[0].ExternalMessageID)
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.runs WHERE id = $1 AND status = 'running' AND attempt = 1`, r.ID), "the same attempt")
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.runs WHERE id = $1 AND reply_state IS NULL AND reply_kind IS NULL AND reply_next_at IS NULL`, r.ID),
		"a prompt not yet sent never goes out after the decision")
	_, err = f.store.ResumeApproval(ctx, token, open)
	assert.ErrorIs(t, err, run.ErrNothingPending, "only an awaiting run resumes this way")
}

func TestStore_PendingUnleasedFindsAnAwaitingRunNoApprovalHolds(t *testing.T) {
	p := newPgRunner(t)
	p.svc.WithApprovals(&fakes.RunApprovals{})
	for name, tc := range map[string]struct {
		status, expires string
		found           bool
	}{
		"pending, not yet expired": {"pending", "now() + interval '1 hour'", false},
		"pending past expiry":      {"pending", "now() - interval '1 second'", true},
		"decided, kick lost":       {"approved", "now() + interval '1 hour'", true},
		"no row":                   {"", "", true},
	} {
		thread := p.thread(t, p.s)
		_, err := p.store.AppendMessage(in(p.s.org), thread, userMsg("record lunch", ""))
		require.NoError(t, err, name)
		r, err := p.turn(t, thread, fakes.RuntimeReply{Stream: awaitStream(t, "call_1")})
		require.NoError(t, err, name)
		require.Equal(t, run.StatusAwaitingApproval, r.Status, name)
		if tc.status != "" {
			decided := "NULL"
			if tc.status != "pending" {
				decided = "now()"
			}
			p.exec(t, `INSERT INTO `+p.schema+`.approvals (id, public_id, org_id, project_id, agent_id, run_id, thread_id, calls, preview, channel,
				raised_snapshot_hash, status, expires_at, created_at, decided_at, decided_by_kind)
				VALUES ($1, $2, $3, $4, $5, $6, $7, '[{"tool_call_id":"call_1"}]', 'p', 'api', '\x01', $8, `+tc.expires+`, now(), `+decided+`, `+map[bool]string{true: "NULL", false: "'system'"}[decided == "NULL"]+`)`,
				uuid.New(), "a"+uuid.NewString()[:12], p.s.org, p.s.project, p.s.agent, r.ID, thread, tc.status)
		}
		got, err := p.store.PendingUnleased(in(p.s.org), time.Now(), 100)
		require.NoError(t, err, name)
		assert.Equal(t, tc.found, slices.Contains(got, thread), name)
	}
}

func TestSettle_ToolMessageOncePerCallID(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	_, err := f.store.AppendMessage(ctx, thread, userMsg("record lunch and wipe", ""))
	require.NoError(t, err)
	token := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	_, err = f.store.StartRun(ctx, token, r)
	require.NoError(t, err)
	require.NoError(t, r.Move(run.StatusAwaitingApproval, time.Now()))
	require.NoError(t, f.store.Settle(ctx, token, r, run.Usage{}, []run.Message{toolMsg("call_free")}), "leg 1 stores the free sibling's result")

	open, err := f.store.OpenRun(ctx, thread)
	require.NoError(t, err)
	require.NoError(t, open.Move(run.StatusRunning, time.Now()))
	_, err = f.store.ResumeApproval(ctx, token, open)
	require.NoError(t, err)
	require.NoError(t, open.Move(run.StatusCompleted, time.Now()))
	require.NoError(t, f.store.Settle(ctx, token, open, run.Usage{}, []run.Message{toolMsg("call_free"), toolMsg("call_gated"), assistantMsg("done")}),
		"leg 2 appends every tool message of the run again; the repeat is skipped, not an error")
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.messages WHERE run_id = $1 AND tool_call_id = 'call_free'`, r.ID))
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.messages WHERE run_id = $1 AND tool_call_id = 'call_gated'`, r.ID))
}

func TestStore_PendingUnleasedFindsAParkedRunWithTheCancelFlag(t *testing.T) {
	p := newPgRunner(t)
	p.svc.WithApprovals(&fakes.RunApprovals{})
	thread := p.thread(t, p.s)
	_, err := p.store.AppendMessage(in(p.s.org), thread, userMsg("record lunch", ""))
	require.NoError(t, err)
	r, err := p.turn(t, thread, fakes.RuntimeReply{Stream: awaitStream(t, "call_1")})
	require.NoError(t, err)
	require.Equal(t, run.StatusAwaitingApproval, r.Status)
	p.exec(t, `INSERT INTO `+p.schema+`.approvals (id, public_id, org_id, project_id, agent_id, run_id, thread_id, calls, preview, channel,
		raised_snapshot_hash, status, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, '[{"tool_call_id":"call_1"}]', 'p', 'api', '\x01', 'pending', now() + interval '1 hour', now())`,
		uuid.New(), "a"+uuid.NewString()[:12], p.s.org, p.s.project, p.s.agent, r.ID, thread)
	got, err := p.store.PendingUnleased(in(p.s.org), time.Now(), 100)
	require.NoError(t, err)
	require.NotContains(t, got, thread, "parked on a live approval")

	require.NoError(t, p.store.RequestCancel(in(p.s.org), thread, r.ID))
	got, err = p.store.PendingUnleased(in(p.s.org), time.Now(), 100)
	require.NoError(t, err)
	assert.Contains(t, got, thread, "a parked run with the cancel flag gets a drain, so the cancel is applied")
}

func TestStore_HoldLeaseOnlyForTheHolder(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	token := f.claimed(t, s, thread)
	require.NoError(t, f.store.HoldLease(in(s.org), token, thread))
	var lost *run.LeaseLostError
	require.ErrorAs(t, f.store.HoldLease(in(s.org), "another-claim", thread), &lost)
}

func TestStore_HelperUsageRoundTrips(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	f.say(t, s, thread)
	token := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	r.Prices = &run.Prices{
		Input: new(int64(1_000_000)), Output: new(int64(1_000_000)), CacheRead: new(int64(1_000_000)), CacheWrite: new(int64(1_000_000)),
		HelperInput: new(int64(100_000)), HelperOutput: new(int64(400_000)), HelperCacheRead: new(int64(10_000)), HelperCacheWrite: new(int64(125_000)),
	}
	_, err := f.store.StartRun(ctx, token, r)
	require.NoError(t, err)

	require.NoError(t, r.Move(run.StatusCompleted, time.Now()))
	u := run.Usage{
		InputTokens: 100, HelperInputTokens: 1_000_000, HelperOutputTokens: 1_000_000, HelperCacheReadTokens: 1_000_000, HelperCacheWriteTokens: 1_000_000,
	}
	require.NoError(t, f.store.Settle(ctx, token, r, u, nil))

	got, err := f.store.RunByID(ctx, r.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Prices)
	assert.Equal(t, r.Prices, got.Prices, "the pinned helper prices are stored at run open")
	assert.Equal(t, u, got.Usage)
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.runs WHERE id = $1 AND cost_micros = 635100`, r.ID),
		"the stored cost prices helper tokens at the helper prices")
}

func TestStore_RunWithoutHelperReadsZeros(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	f.say(t, s, thread)
	token := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	r.Prices = &run.Prices{Input: new(int64(1_000_000))}
	_, err := f.store.StartRun(ctx, token, r)
	require.NoError(t, err)

	got, err := f.store.RunByID(ctx, r.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Prices)
	assert.Nil(t, got.Prices.HelperInput, "a run opened without helper prices, or by the app before them, has none")
	assert.Nil(t, got.Prices.HelperOutput)
	assert.Nil(t, got.Prices.HelperCacheRead)
	assert.Nil(t, got.Prices.HelperCacheWrite)
	assert.Equal(t, run.Usage{}, got.Usage, "helper token columns default to 0")
}

func TestStore_KnowledgeUsageRoundTrips(t *testing.T) {
	f := newPgFixture(t)
	s := f.seed(t)
	thread := f.thread(t, s)
	ctx := in(s.org)
	f.say(t, s, thread)
	token := f.claimed(t, s, thread)
	r := run.New(s.org, s.project, thread, s.version, time.Now())
	r.Prices = &run.Prices{Input: new(int64(1_000_000)), Embedding: new(int64(20_000))}
	_, err := f.store.StartRun(ctx, token, r)
	require.NoError(t, err)

	require.NoError(t, r.Move(run.StatusCompleted, time.Now()))
	r.KnowledgeRetrieveCalls, r.KnowledgeRetrieveMs, r.KnowledgePassages = 2, 340, 7
	require.NoError(t, f.store.Settle(ctx, token, r, run.Usage{InputTokens: 100, EmbeddingTokens: 1_000_000}, nil))

	got, err := f.store.RunByID(ctx, r.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Prices)
	assert.Equal(t, int64(20_000), *got.Prices.Embedding, "the pinned embedding price is stored at run open")
	assert.Equal(t, int64(1_000_000), got.Usage.EmbeddingTokens)
	assert.Equal(t, int32(2), got.KnowledgeRetrieveCalls)
	assert.Equal(t, int64(340), got.KnowledgeRetrieveMs)
	assert.Equal(t, int32(7), got.KnowledgePassages)
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM `+f.schema+`.runs WHERE id = $1 AND cost_micros = 20100`, r.ID),
		"the stored cost prices embedding tokens at the embedding price")
}
