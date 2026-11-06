//go:build integration

package run_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/run/snapshotstore"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/internal/testutil/pgtest"
)

type pgRunner struct {
	*pgFixture
	pool db.Pool
	s    scope
	rt   *fakes.Runtime
	svc  *run.Service
}

func newPgRunner(t *testing.T) *pgRunner {
	t.Helper()
	pool, admin, schema := pgtest.MigratedRLS(t)
	f := &pgFixture{pool: pool, admin: admin, schema: schema, store: run.NewStore(pool, schema, "")}
	p := &pgRunner{pgFixture: f, pool: pool, s: f.seed(t), rt: &fakes.Runtime{}}
	provider := uuid.New()
	version := &run.AgentVersion{ID: p.s.version, AgentID: p.s.agent, Version: 1, Model: "gpt-4o", ProviderID: provider}
	p.svc = run.NewService(f.store, db.NewTransactor(pool), apperror.NewReporter(nil, false).Unexpected).
		WithRuntime(p.rt).
		WithAgents(&fakes.RunAgents{Versions: map[uuid.UUID]*run.AgentVersion{version.ID: version}}).
		WithProviders(&fakes.RunProviders{Models: map[uuid.UUID]map[string]run.ResolvedModel{provider: {
			"gpt-4o": {BaseURL: "https://llm.example.com/v1", APIKey: "sk-test"},
		}}}).
		WithMCPServers(&fakes.RunMCPServers{}).
		WithSnapshots(snapshotstore.NewPostgres(pool, schema, ""))
	return p
}

type countingExecutor struct {
	inner   run.Executor
	running atomic.Int32
	peak    atomic.Int32
	calls   atomic.Int32
}

func (e *countingExecutor) Execute(ctx context.Context, thread uuid.UUID, token string) (*run.Run, error) {
	n := e.running.Add(1)
	defer e.running.Add(-1)
	if n > e.peak.Load() {
		e.peak.Store(n)
	}
	e.calls.Add(1)
	time.Sleep(50 * time.Millisecond)
	return e.inner.Execute(ctx, thread, token)
}

func runFor(t *testing.T, rs ...*run.Runner) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for _, r := range rs {
		wg.Go(func() { assert.NoError(t, r.Run(ctx)) })
	}
	return func() {
		cancel()
		wg.Wait()
	}
}

func (p *pgRunner) waitStatus(t *testing.T, want string) {
	t.Helper()
	require.Eventually(t, func() bool {
		return p.count(t, `SELECT count(*) FROM `+p.schema+`.runs WHERE status = $1`, want) > 0
	}, 10*time.Second, 20*time.Millisecond, "no run reached %s", want)
}

func TestTurnRunner_TwoWorkersOneLease(t *testing.T) {
	p := newPgRunner(t)
	thread := p.thread(t, p.s)
	p.say(t, p.s, thread)
	p.rt.Replies = []fakes.RuntimeReply{{Stream: stream(t, "plain_reply")}}
	exec := &countingExecutor{inner: p.svc}
	a := run.NewRunner(exec, p.store, runnerOpts())
	b := run.NewRunner(exec, p.store, runnerOpts())
	stop := runFor(t, a, b)
	k := run.Kick{OrgID: p.s.org, ThreadID: thread}
	require.True(t, a.Offer(k))
	require.True(t, b.Offer(k))
	p.waitStatus(t, "completed")
	stop()
	assert.Equal(t, int32(1), exec.peak.Load(), "only one worker ever held the thread")
	assert.Equal(t, 1, p.count(t, `SELECT count(*) FROM `+p.schema+`.runs WHERE thread_id = $1`, thread))
	assert.Equal(t, 1, p.count(t, `SELECT count(*) FROM `+p.schema+`.threads WHERE id = $1 AND lease_owner IS NULL`, thread))
}

func TestTurnRunner_LeaseExpiryResumesSameRun(t *testing.T) {
	p := newPgRunner(t)
	thread := p.thread(t, p.s)
	ctx := in(p.s.org)
	p.say(t, p.s, thread)
	dead := p.claimed(t, p.s, thread)
	r := run.New(p.s.org, p.s.project, thread, p.s.version, time.Now())
	_, err := p.store.StartRun(ctx, dead, r)
	require.NoError(t, err)
	p.expire(t, thread)

	k := &kicks{}
	for _, j := range run.Jobs(p.store, k, 15*time.Second) {
		if j.Name == "run-sweep" {
			require.NoError(t, j.Run(ctx))
		}
	}
	require.Len(t, k.got, 1, "the sweeper kicks the thread it reclaimed")
	assert.Equal(t, 1, p.count(t, `SELECT count(*) FROM `+p.schema+`.runs WHERE id = $1 AND status = 'interrupted'`, r.ID))

	p.rt.Replies = []fakes.RuntimeReply{{Stream: stream(t, "plain_reply")}}
	runner := run.NewRunner(p.svc, p.store, runnerOpts())
	stop := runFor(t, runner)
	require.True(t, runner.Offer(k.got[0]))
	p.waitStatus(t, "completed")
	stop()
	assert.Equal(t, 1, p.count(t, `SELECT count(*) FROM `+p.schema+`.runs WHERE id = $1 AND status = 'completed' AND attempt = 2`, r.ID),
		"the next claim resumed the same run")
	assert.Equal(t, 1, p.count(t, `SELECT count(*) FROM `+p.schema+`.runs WHERE thread_id = $1`, thread))
}

func TestTurnRunner_SettleAndSnapshotCommitTogether(t *testing.T) {
	p := newPgRunner(t)
	thread := p.thread(t, p.s)
	p.say(t, p.s, thread)
	p.rt.Replies = []fakes.RuntimeReply{{Stream: stream(t, "plain_reply")}}
	token := p.claimed(t, p.s, thread)
	r, err := p.svc.Execute(in(p.s.org), thread, token)
	require.NoError(t, err)
	assert.Equal(t, run.StatusCompleted, r.Status)
	assert.Equal(t, 1, p.count(t, `SELECT count(*) FROM `+p.schema+`.thread_snapshots WHERE thread_id = $1 AND sdk_version = '1.23.0'`, thread))

	p.say(t, p.s, thread)
	p.rt.Replies = []fakes.RuntimeReply{{Stream: stream(t, "plain_reply")}}
	p.exec(t, `ALTER TABLE `+p.schema+`.thread_snapshots ADD CONSTRAINT no_writes CHECK (sdk_version = 'never')`+" NOT VALID")
	_, err = p.svc.Execute(in(p.s.org), thread, token)
	require.Error(t, err, "the snapshot write fails")
	assert.Equal(t, 1, p.count(t, `SELECT count(*) FROM `+p.schema+`.runs WHERE thread_id = $1 AND status = 'completed'`, thread),
		"the second run's settle rolled back with its snapshot")
}

func TestTurnRunner_AgentDeletedMidTurnEndsQuietly(t *testing.T) {
	p := newPgRunner(t)
	thread := p.thread(t, p.s)
	p.say(t, p.s, thread)
	p.rt.Replies = []fakes.RuntimeReply{{Stream: cut(stream(t, "plain_reply")), Block: true}}
	var reported atomic.Int32
	p.svc = run.NewService(p.store, db.NewTransactor(p.pool), func(ctx context.Context, msg string, err error, attrs ...any) *apperror.AppError {
		reported.Add(1)
		return apperror.NewReporter(nil, false).Unexpected(ctx, msg, err, attrs...)
	}).WithRuntime(p.rt).WithAgents(&fakes.RunAgents{Versions: map[uuid.UUID]*run.AgentVersion{p.s.version: {
		ID: p.s.version, AgentID: p.s.agent, Version: 1, Model: "gpt-4o", ProviderID: uuid.Nil,
	}}}).WithProviders(&fakes.RunProviders{Models: map[uuid.UUID]map[string]run.ResolvedModel{uuid.Nil: {
		"gpt-4o": {BaseURL: "https://llm.example.com/v1", APIKey: "sk-test"},
	}}}).WithMCPServers(&fakes.RunMCPServers{})
	opts := runnerOpts()
	opts.Heartbeat = 50 * time.Millisecond
	runner := run.NewRunner(p.svc, p.store, opts)
	stop := runFor(t, runner)
	require.True(t, runner.Offer(run.Kick{OrgID: p.s.org, ThreadID: thread}))
	require.Eventually(t, func() bool { return p.rt.Calls() == 1 }, 10*time.Second, 10*time.Millisecond)
	p.exec(t, `DELETE FROM `+p.schema+`.agents WHERE id = $1`, p.s.agent)
	time.Sleep(300 * time.Millisecond)
	stop()
	assert.Zero(t, reported.Load(), "a lease lost to a delete is not an error report")
	assert.Zero(t, p.count(t, `SELECT count(*) FROM `+p.schema+`.threads WHERE id = $1`, thread))
}

func TestSettle_PublisherFailureRollsTheSettleBack(t *testing.T) {
	p := newPgRunner(t)
	p.svc.WithPublisher(&fakes.Publisher{Err: assertErr{}})
	thread := p.thread(t, p.s)
	p.say(t, p.s, thread)
	p.rt.Replies = []fakes.RuntimeReply{{Stream: stream(t, "plain_reply")}}
	token := p.claimed(t, p.s, thread)
	_, err := p.svc.Execute(in(p.s.org), thread, token)
	require.Error(t, err)
	assert.Equal(t, 1, p.count(t, `SELECT count(*) FROM `+p.schema+`.runs WHERE thread_id = $1 AND status = 'running'`, thread),
		"the run, its messages and its snapshot wait for the retry")
	assert.Zero(t, p.count(t, `SELECT count(*) FROM `+p.schema+`.thread_snapshots WHERE thread_id = $1`, thread))
	assert.Zero(t, p.count(t, `SELECT count(*) FROM `+p.schema+`.messages WHERE thread_id = $1 AND role = 'assistant'`, thread))
}
