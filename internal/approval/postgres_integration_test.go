//go:build integration

package approval_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/approval"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/run"
	"altalune.id/oagent/internal/run/snapshotstore"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/internal/testutil/pgtest"
)

type pgWorld struct {
	svc                                *approval.Service
	store                              approval.Store
	pub                                *fakes.Publisher
	admin                              *sql.DB
	pool                               db.Pool
	schema                             string
	org, project, agent, version, sari uuid.UUID
	now                                time.Time
	ctx                                context.Context
	refs                               refs
}

func newPgWorld(t *testing.T) *pgWorld {
	t.Helper()
	pool, admin, schemaName := pgtest.MigratedRLS(t)
	w := &pgWorld{
		admin: admin, pool: pool, schema: schemaName, pub: &fakes.Publisher{}, now: time.Now().UTC().Truncate(time.Second),
		org: uuid.New(), project: uuid.New(), agent: uuid.New(), version: uuid.New(), sari: uuid.New(),
	}
	p := schemaName + "."
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO ` + p + `orgs (id, public_id, slug, name, created_at, updated_at) VALUES ($1, $2, $2, 'O', now(), now())`, []any{w.org, "o" + w.org.String()[:12]}},
		{`INSERT INTO ` + p + `projects (id, public_id, org_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, 'p', 'P', now(), now())`, []any{w.project, "p" + w.project.String()[:12], w.org}},
		{`INSERT INTO ` + p + `agents (id, public_id, org_id, project_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, $4, 'a', 'A', now(), now())`, []any{w.agent, "a" + w.agent.String()[:12], w.org, w.project}},
		{`INSERT INTO ` + p + `agent_versions (id, public_id, org_id, project_id, agent_id, version, config, created_at, kind) VALUES ($1, $2, $3, $4, $5, 1, '{}', now(), 'published')`, []any{w.version, "v" + w.version.String()[:12], w.org, w.project, w.agent}},
		{`INSERT INTO ` + p + `end_users (id, public_id, org_id, project_id, created_at, updated_at) VALUES ($1, $2, $3, $4, now(), now())`, []any{w.sari, "e" + w.sari.String()[:12], w.org, w.project}},
	} {
		_, err := admin.ExecContext(t.Context(), q.sql, q.args...)
		require.NoError(t, err)
	}
	ids := fakes.NewPublicIDs()
	ids.Org(w.org)
	ids.Project(w.project, w.org)
	ids.Resource(w.agent, w.org)
	w.refs = refs{PublicIDs: ids, threads: map[uuid.UUID]string{}, runs: map[uuid.UUID]string{}}
	w.store = approval.NewStore(pool, schemaName, "")
	w.svc = approval.NewService(w.store, db.NewTransactor(pool), w.refs, apperror.NewReporter(nil, false).Unexpected).
		WithPublisher(w.pub).WithClock(func() time.Time { return w.now })
	w.ctx = tenant.Into(context.Background(), tenant.Context{OrgID: w.org, ProjectID: w.project})
	return w
}

func (w *pgWorld) waiting(t *testing.T, kind string) (thread, run uuid.UUID) {
	t.Helper()
	thread, run = uuid.New(), uuid.New()
	p := w.schema + "."
	_, err := w.admin.ExecContext(t.Context(), `INSERT INTO `+p+`threads (id, public_id, org_id, project_id, agent_id, kind, external_key, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now())`, thread, "t"+thread.String()[:12], w.org, w.project, w.agent, kind, thread.String())
	require.NoError(t, err)
	_, err = w.admin.ExecContext(t.Context(), `INSERT INTO `+p+`runs (id, public_id, org_id, project_id, thread_id, agent_version_id, status, started_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'awaiting_approval', now(), now(), now())`, run, "r"+run.String()[:12], w.org, w.project, thread, w.version)
	require.NoError(t, err)
	w.refs.threads[thread], w.refs.runs[run] = "t"+thread.String()[:12], "r"+run.String()[:12]
	return thread, run
}

func (w *pgWorld) create(t *testing.T, kind string) *approval.Approval {
	t.Helper()
	thread, run := w.waiting(t, kind)
	a, err := w.svc.Create(w.ctx, approval.CreateInput{
		OrgID: w.org, ProjectID: w.project, AgentID: w.agent, RunID: run, ThreadID: thread, ThreadKind: kind,
		Calls: []approval.Call{expenseCall()}, Speakers: []uuid.NullUUID{{UUID: w.sari, Valid: true}}, RaisedSnapshotHash: []byte{1, 2, 3},
	})
	require.NoError(t, err)
	return a
}

// SECURITY: two transactions deciding at once: the row lock serializes them and the pending guard lets only the first write.
func TestPostgres_DecideIsSingleUse(t *testing.T) {
	w := newPgWorld(t)
	a := w.create(t, "api")
	var wins, losses int
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range 8 {
		wg.Go(func() {
			<-start
			_, err := w.svc.Decide(w.ctx, a.ID, approval.Answer{Approved: i%2 == 0, By: approval.Decider{Kind: approval.DeciderUser, ID: uuid.NewString()}, Via: approval.ViaConsole})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				wins++
			case approval.IsAlreadyDecidedError(err):
				losses++
			default:
				t.Errorf("decide: %v", err)
			}
		})
	}
	close(start)
	wg.Wait()
	assert.Equal(t, 1, wins, "one of eight concurrent decisions wins")
	assert.Equal(t, 7, losses)
	decided := 0
	for _, e := range w.pub.Events() {
		if e.Type == "approval.decided" {
			decided++
		}
	}
	assert.Equal(t, 1, decided, "and only the winner emits approval.decided")
}

func TestPostgres_RoundTripAndReads(t *testing.T) {
	w := newPgWorld(t)
	a := w.create(t, "dm")
	got, err := w.store.Get(w.ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, a.PublicID, got.PublicID)
	require.Len(t, got.Calls, 1)
	want := expenseCall()
	assert.JSONEq(t, string(want.Args), string(got.Calls[0].Args), "JSONB keeps the values, not the key order")
	got.Calls[0].Args, want.Args = nil, nil
	assert.Equal(t, want, got.Calls[0], "and the keys carry the schema order for the preview")
	assert.Equal(t, a.Preview, got.Preview)
	assert.Equal(t, approval.ChannelWhatsApp, got.Channel)
	assert.Equal(t, uuid.NullUUID{UUID: w.sari, Valid: true}, got.Requester)
	assert.Equal(t, []byte{1, 2, 3}, got.RaisedSnapshotHash)
	assert.Equal(t, approval.StatusPending, got.Status)
	assert.True(t, got.ExpiresAt.Equal(w.now.Add(24*time.Hour)))
	assert.Nil(t, got.DecidedAt)

	pending, err := w.store.PendingForThread(w.ctx, a.ThreadID)
	require.NoError(t, err)
	assert.Equal(t, a.ID, pending.ID)
	ref, err := w.store.Resolve(context.Background(), a.PublicID)
	require.NoError(t, err, "resolve needs no tenant")
	assert.Equal(t, approval.Ref{ID: a.ID, OrgID: w.org, ProjectID: w.project, AgentID: w.agent}, ref)
	_, err = w.store.Resolve(context.Background(), "nope")
	assert.True(t, approval.IsNotFoundError(err))

	w.now = w.now.Add(time.Second)
	b := w.create(t, "try")
	list, err := w.store.List(w.ctx, approval.Filter{ProjectID: w.project}, approval.Cursor{}, 1)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, b.ID, list[0].ID, "newest first")
	next, err := w.store.List(w.ctx, approval.Filter{ProjectID: w.project}, approval.Cursor{At: list[0].CreatedAt, PublicID: list[0].PublicID}, 5)
	require.NoError(t, err)
	require.Len(t, next, 1)
	assert.Equal(t, a.ID, next[0].ID)
	none, err := w.store.List(w.ctx, approval.Filter{ProjectID: w.project, AgentIDs: []uuid.UUID{uuid.New()}}, approval.Cursor{}, 5)
	require.NoError(t, err)
	assert.Empty(t, none, "an agent filter narrows")
	n, err := w.store.CountPending(w.ctx, w.project, nil, w.now)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	n, err = w.store.CountPending(w.ctx, w.project, []uuid.UUID{uuid.New()}, w.now)
	require.NoError(t, err)
	assert.Zero(t, n)
	n, err = w.store.CountPending(w.ctx, w.project, nil, w.now.Add(25*time.Hour))
	require.NoError(t, err)
	assert.Zero(t, n, "past their expiry, pending rows no longer count")
	late, err := w.store.List(w.ctx, approval.Filter{ProjectID: w.project, Status: approval.StatusPending, Now: w.now.Add(25 * time.Hour)}, approval.Cursor{}, 5)
	require.NoError(t, err)
	assert.Empty(t, late, "nor list as pending")

	other := tenant.Into(context.Background(), tenant.Context{OrgID: uuid.New()})
	_, err = w.store.Get(other, a.ID)
	assert.True(t, approval.IsNotFoundError(err), "another org never sees it")
}

func TestPostgres_DecideWritesTheDecisionAndCallStates(t *testing.T) {
	w := newPgWorld(t)
	a := w.create(t, "dm")
	by := approval.Decider{Kind: approval.DeciderEndUser, ID: w.sari.String()}
	got, err := w.svc.Decide(w.ctx, a.ID, approval.Answer{Approved: true, Text: "ya", By: by, Via: approval.ViaWhatsApp})
	require.NoError(t, err)
	assert.Equal(t, approval.StatusApproved, got.Status)
	assert.Equal(t, approval.CallApproved, got.Calls[0].State, "an approval marks every call approved")
	assert.Equal(t, approval.ViaWhatsApp, got.DecidedVia)
	assert.Equal(t, approval.DeciderEndUser, got.DecidedByKind)
	assert.Equal(t, "ya", got.DecisionText)
	require.NotNil(t, got.DecidedAt)

	require.NoError(t, w.svc.MarkExecuting(w.ctx, a.ID, []string{"call_1"}))
	assert.Error(t, w.svc.MarkExecuting(w.ctx, a.ID, []string{"call_1"}), "executing is entered once")
	require.NoError(t, w.svc.MarkExecuted(w.ctx, a.ID, "call_1"))
	stored, err := w.store.Get(w.ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, approval.CallExecuted, stored.Calls[0].State)

	r := w.create(t, "api")
	got, err = w.svc.Decide(w.ctx, r.ID, approval.Answer{Approved: false, Text: "no"})
	require.NoError(t, err)
	assert.Empty(t, got.Calls[0].State, "a rejection approves no call")

	w.create(t, "api")
	decided, err := w.store.List(w.ctx, approval.Filter{ProjectID: w.project, Status: approval.StatusDecided, Now: w.now}, approval.Cursor{}, 10)
	require.NoError(t, err)
	statuses := map[uuid.UUID]approval.Status{}
	for _, d := range decided {
		statuses[d.ID] = d.Status
	}
	assert.Equal(t, map[uuid.UUID]approval.Status{a.ID: approval.StatusApproved, r.ID: approval.StatusRejected}, statuses, "decided is approved or rejected, never pending")
}

func TestPostgres_DecideAfterExpiryIsAPR003(t *testing.T) {
	w := newPgWorld(t)
	a := w.create(t, "group")
	w.now = w.now.Add(10 * time.Minute)
	_, err := w.svc.Decide(w.ctx, a.ID, approval.Answer{Approved: true})
	var decided *approval.AlreadyDecidedError
	require.ErrorAs(t, err, &decided)
	assert.Equal(t, "expired", decided.Status)
}

func TestPostgres_OnePendingPerThroughTheStore(t *testing.T) {
	w := newPgWorld(t)
	a := w.create(t, "api")
	_, err := w.svc.Create(w.ctx, approval.CreateInput{
		OrgID: w.org, ProjectID: w.project, AgentID: w.agent, RunID: a.RunID, ThreadID: a.ThreadID, ThreadKind: "api",
		Calls: []approval.Call{expenseCall()}, RaisedSnapshotHash: []byte{1},
	})
	assert.Error(t, err, "the partial unique index refuses a second pending approval")
}

func TestPostgres_FailedEventLeavesNoRow(t *testing.T) {
	w := newPgWorld(t)
	w.pub.Err = errors.New("outbox down")
	thread, run := w.waiting(t, "api")
	_, err := w.svc.Create(w.ctx, approval.CreateInput{
		OrgID: w.org, ProjectID: w.project, AgentID: w.agent, RunID: run, ThreadID: thread, ThreadKind: "api",
		Calls: []approval.Call{expenseCall()}, RaisedSnapshotHash: []byte{1},
	})
	require.Error(t, err)
	_, err = w.store.PendingForThread(w.ctx, thread)
	assert.True(t, approval.IsNotFoundError(err), "the insert rolled back with the event")
}

// SECURITY: the hash is taken from the bytes the awaiting settle saves, in the same unit of work as the approval; never from a re-read.
func TestPostgres_RaisedSnapshotHashIsTheSavedSnapshotInOneUnitOfWork(t *testing.T) {
	w := newPgWorld(t)
	snaps := snapshotstore.NewPostgres(w.pool, w.schema, "")
	uow := db.NewTransactor(w.pool)
	settle := func(thread, runID uuid.UUID, blob string, fail error) (*approval.Approval, error) {
		var a *approval.Approval
		err := uow.RunInTx(w.ctx, func(ctx context.Context) error {
			snap := &run.Snapshot{ThreadID: thread, OrgID: w.org, ProjectID: w.project, AgentVersionID: w.version, SDKVersion: "1.23.0", Blob: []byte(blob), UpdatedAt: w.now}
			if err := snaps.Save(ctx, snap); err != nil {
				return err
			}
			var err error
			a, err = w.svc.Create(ctx, approval.CreateInput{
				OrgID: w.org, ProjectID: w.project, AgentID: w.agent, RunID: runID, ThreadID: thread, ThreadKind: "api",
				Calls: []approval.Call{expenseCall()}, RaisedSnapshotHash: run.SnapshotHash(snap),
			})
			if err != nil {
				return err
			}
			return fail
		})
		return a, err
	}

	thread, runID := w.waiting(t, "api")
	a, err := settle(thread, runID, "interrupted", nil)
	require.NoError(t, err)
	loaded, err := snaps.Load(w.ctx, thread)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	stored, err := w.store.Get(w.ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, run.SnapshotHash(loaded), stored.RaisedSnapshotHash, "the stored hash is the saved snapshot's, as loaded back for a resume")

	thread2, run2 := w.waiting(t, "api")
	_, err = settle(thread2, run2, "never saved", errors.New("settle failed"))
	require.Error(t, err)
	gone, err := snaps.Load(w.ctx, thread2)
	require.NoError(t, err)
	assert.Nil(t, gone, "the snapshot rolled back")
	_, err = w.store.PendingForThread(w.ctx, thread2)
	assert.True(t, approval.IsNotFoundError(err), "and the approval with it: one unit of work")
}

func TestPostgres_LatestForRunIsTheNewest(t *testing.T) {
	w := newPgWorld(t)
	first := w.create(t, "api")
	_, err := w.svc.Decide(w.ctx, first.ID, approval.Answer{Approved: true})
	require.NoError(t, err)
	w.now = w.now.Add(time.Second)
	second, err := w.svc.Create(w.ctx, approval.CreateInput{
		OrgID: w.org, ProjectID: w.project, AgentID: w.agent, RunID: first.RunID, ThreadID: first.ThreadID, ThreadKind: "api",
		Calls: []approval.Call{expenseCall()}, RaisedSnapshotHash: []byte{9},
	})
	require.NoError(t, err)
	latest, err := w.svc.LatestForRun(w.ctx, first.RunID)
	require.NoError(t, err)
	assert.Equal(t, second.ID, latest.ID, "a resume answers the run's newest approval, never an older one")
	_, err = w.svc.LatestForRun(w.ctx, uuid.New())
	assert.True(t, approval.IsNotFoundError(err))
}

func TestPostgres_ExpireOnlyAPendingRowPastExpiryOnTheDatabaseClock(t *testing.T) {
	w := newPgWorld(t)
	live := w.create(t, "api")
	ok, err := w.svc.Expire(w.ctx, live.ID)
	require.NoError(t, err)
	assert.False(t, ok, "not yet past its expiry")

	late := w.create(t, "api")
	_, err = w.admin.ExecContext(t.Context(), `UPDATE `+w.schema+`.approvals SET expires_at = now() - interval '1 second' WHERE id = $1`, late.ID)
	require.NoError(t, err)
	ok, err = w.svc.Expire(w.ctx, late.ID)
	require.NoError(t, err)
	require.True(t, ok)
	got, err := w.svc.Get(w.ctx, late.ID)
	require.NoError(t, err)
	assert.Equal(t, approval.StatusExpired, got.Status)
	assert.Equal(t, approval.DeciderSystem, got.DecidedByKind)
	require.NotNil(t, got.DecidedAt)

	ok, err = w.svc.Expire(w.ctx, late.ID)
	require.NoError(t, err)
	assert.False(t, ok, "single use")
	_, err = w.svc.PendingForThread(w.ctx, late.ThreadID)
	assert.True(t, approval.IsNotFoundError(err), "the thread may wait on a new approval again")
}

func TestPostgres_DueExpiriesAndExpireUnresumable(t *testing.T) {
	w := newPgWorld(t)
	live, late, approved, rejected := w.create(t, "api"), w.create(t, "api"), w.create(t, "api"), w.create(t, "api")
	_, err := w.admin.ExecContext(t.Context(), `UPDATE `+w.schema+`.approvals SET expires_at = now() - interval '1 second' WHERE id = $1`, late.ID)
	require.NoError(t, err)
	due, err := w.store.DueExpiries(w.ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{late.ID}, due, "only pending rows past their expiry on the database clock")

	person := approval.Decider{Kind: approval.DeciderUser, ID: uuid.NewString()}
	_, err = w.svc.Decide(w.ctx, approved.ID, approval.Answer{Approved: true, By: person, Via: approval.ViaConsole})
	require.NoError(t, err)
	_, err = w.svc.Decide(w.ctx, rejected.ID, approval.Answer{By: person, Via: approval.ViaConsole})
	require.NoError(t, err)
	for _, id := range []uuid.UUID{live.ID, approved.ID, rejected.ID} {
		require.NoError(t, w.svc.ExpireUnresumable(w.ctx, id))
	}
	got, err := w.svc.Get(w.ctx, live.ID)
	require.NoError(t, err)
	assert.Equal(t, approval.StatusExpired, got.Status)
	assert.Equal(t, approval.DeciderSystem, got.DecidedByKind)
	require.NotNil(t, got.DecidedAt)
	got, err = w.svc.Get(w.ctx, approved.ID)
	require.NoError(t, err)
	assert.Equal(t, approval.StatusExpired, got.Status, "approved calls that never ran are not left approved")
	assert.Equal(t, person.ID, got.DecidedByID, "the person who approved stays on record")
	got, err = w.svc.Get(w.ctx, rejected.ID)
	require.NoError(t, err)
	assert.Equal(t, approval.StatusRejected, got.Status)
}
