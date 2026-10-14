//go:build integration

package outbox_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/outbox"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/testutil/pgtest"
	"altalune.id/oagent/nanoid"
)

type fixture struct {
	db    *sql.DB
	store outbox.Store
	pool  db.Pool
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	sqlDB, schemaName := pgtest.Migrated(t)
	pool := db.Pool{W: sqlDB, R: sqlDB}
	return &fixture{db: sqlDB, store: outbox.NewStore(pool, schemaName, ""), pool: pool}
}

func (f *fixture) seedTenant(t *testing.T) (context.Context, tenant.Context) {
	t.Helper()
	orgID, projID := uuid.New(), uuid.New()
	now := time.Now().UTC()
	_, err := f.db.ExecContext(t.Context(),
		`INSERT INTO orgs (id, public_id, slug, name, created_at, updated_at) VALUES ($1, $2, $2, 'Org', $3, $3)`,
		orgID, orgID.String(), now)
	require.NoError(t, err)
	_, err = f.db.ExecContext(t.Context(),
		`INSERT INTO projects (id, public_id, org_id, slug, name, created_at, updated_at) VALUES ($1, $2, $3, $2, 'P', $4, $4)`,
		projID, projID.String(), orgID, now)
	require.NoError(t, err)
	tc := tenant.Context{OrgID: orgID, ProjectID: projID, UserID: uuid.New()}
	return tenant.Into(t.Context(), tc), tc
}

func (f *fixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	_, err := f.db.ExecContext(t.Context(), query, args...)
	require.NoError(t, err)
}

func (f *fixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, f.db.QueryRowContext(t.Context(), query, args...).Scan(&n))
	return n
}

func newEntry(tc tenant.Context, target string) outbox.Entry {
	return outbox.Entry{
		ID:            uuid.New(),
		PublicID:      nanoid.MustNew(21),
		EventID:       uuid.New(),
		EventPublicID: nanoid.MustNew(21),
		OrgID:         tc.OrgID,
		ProjectID:     tc.ProjectID,
		Target:        target,
		Payload:       []byte(`{"k":"v"}`),
	}
}

func past() time.Time { return time.Now().UTC().Add(-time.Minute) }

func TestEnqueue_RoundTripsEveryField(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	jakarta := time.FixedZone("WIB", 7*3600)
	due := time.Date(2026, 10, 1, 23, 30, 0, 0, jakarta)
	e := newEntry(tc, "webhook:a")
	e.Payload = []byte{0x00, 0xff, 0x10}
	e.NextAttemptAt = due

	before := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, f.store.Enqueue(ctx, e))

	got, err := f.store.ByID(ctx, e.ID, e.Target)
	require.NoError(t, err)
	assert.Equal(t, e.ID, got.ID)
	assert.Equal(t, e.EventID, got.EventID)
	assert.Equal(t, tc.OrgID, got.OrgID)
	assert.Equal(t, tc.ProjectID, got.ProjectID)
	assert.Equal(t, "webhook:a", got.Target)
	assert.Equal(t, []byte{0x00, 0xff, 0x10}, got.Payload)
	assert.Equal(t, 0, got.Attempt)
	assert.Equal(t, outbox.StatusPending, got.Status)
	assert.Empty(t, got.LastError)
	assert.Same(t, time.UTC, got.NextAttemptAt.Location())
	assert.True(t, due.Equal(got.NextAttemptAt), "next_attempt_at %s, want %s", got.NextAttemptAt, due)
	assert.Same(t, time.UTC, got.CreatedAt.Location())
	assert.False(t, got.CreatedAt.Before(before))

	var deliveredAt sql.NullTime
	require.NoError(t, f.db.QueryRowContext(t.Context(),
		`SELECT delivered_at FROM outbox_entries WHERE id = $1`, e.ID).Scan(&deliveredAt))
	assert.False(t, deliveredAt.Valid)
}

func TestEnqueue_ZeroDueTimeMeansNow(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	e := newEntry(tc, "t")
	before := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, f.store.Enqueue(ctx, e))
	after := time.Now().UTC()

	got, err := f.store.ByID(ctx, e.ID, "t")
	require.NoError(t, err)
	assert.False(t, got.NextAttemptAt.Before(before))
	assert.False(t, got.NextAttemptAt.After(after))
	assert.True(t, got.NextAttemptAt.Equal(got.CreatedAt), "a zero due time is the insert time")
}

func TestEnqueue_IsIdempotentPerOrgEventAndTarget(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	otherCtx, otherTC := f.seedTenant(t)

	first := newEntry(tc, "t1")
	require.NoError(t, f.store.Enqueue(ctx, first))

	repeat := first
	repeat.ID = uuid.New()
	repeat.PublicID = nanoid.MustNew(21)
	repeat.Payload = []byte("changed")
	require.NoError(t, f.store.Enqueue(ctx, repeat), "a repeat must be a silent no-op")

	sameEventOtherTarget := first
	sameEventOtherTarget.ID = uuid.New()
	sameEventOtherTarget.PublicID = nanoid.MustNew(21)
	sameEventOtherTarget.Target = "t2"
	require.NoError(t, f.store.Enqueue(ctx, sameEventOtherTarget))

	sameEventOtherOrg := newEntry(otherTC, "t1")
	sameEventOtherOrg.EventID = first.EventID
	require.NoError(t, f.store.Enqueue(otherCtx, sameEventOtherOrg))

	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM outbox_entries WHERE org_id = $1 AND target = 't1'`, tc.OrgID))
	kept, err := f.store.ByID(ctx, first.ID, "t1")
	require.NoError(t, err)
	assert.Equal(t, first.Payload, kept.Payload, "the repeat must not overwrite the queued entry")
	_, err = f.store.ByID(ctx, repeat.ID, "t1")
	assert.True(t, outbox.IsNotFoundError(err), "got %v", err)

	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM outbox_entries WHERE org_id = $1 AND target = 't2'`, tc.OrgID))
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM outbox_entries WHERE org_id = $1`, otherTC.OrgID))
}

func TestEnqueue_RejectsEntriesOutsideTheTenantScope(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	_, otherTC := f.seedTenant(t)

	tests := []struct {
		name  string
		mut   func(*outbox.Entry)
		field string
	}{
		{"other org", func(e *outbox.Entry) { e.OrgID = otherTC.OrgID }, "OrgID"},
		{"other project", func(e *outbox.Entry) { e.ProjectID = otherTC.ProjectID }, "ProjectID"},
		{"empty target", func(e *outbox.Entry) { e.Target = "" }, "Target"},
		{"long target", func(e *outbox.Entry) { e.Target = strings.Repeat("x", outbox.MaxTargetLen+1) }, "Target"},
		{"settled status", func(e *outbox.Entry) { e.Status = outbox.StatusDelivered }, "Status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEntry(tc, "t")
			tt.mut(&e)
			err := f.store.Enqueue(ctx, e)
			var inv *outbox.InvalidEntryError
			require.ErrorAs(t, err, &inv)
			assert.Equal(t, tt.field, inv.Field)
		})
	}
	assert.Zero(t, f.count(t, `SELECT count(*) FROM outbox_entries`))
}

func TestEnqueue_MaxLengthTargetIsAccepted(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	e := newEntry(tc, strings.Repeat("x", outbox.MaxTargetLen))
	require.NoError(t, f.store.Enqueue(ctx, e))
}

func TestEnqueue_UnknownProjectViolatesTheForeignKey(t *testing.T) {
	f := newFixture(t)
	_, tc := f.seedTenant(t)
	tc.ProjectID = uuid.New()
	ctx := tenant.Into(t.Context(), tc)

	err := f.store.Enqueue(ctx, newEntry(tc, "t"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outbox_entries_org_project_fkey")
}

func TestEnqueue_DeletingOrgOrProjectCascades(t *testing.T) {
	f := newFixture(t)
	ctxA, tcA := f.seedTenant(t)
	ctxB, tcB := f.seedTenant(t)
	require.NoError(t, f.store.Enqueue(ctxA, newEntry(tcA, "t")))
	require.NoError(t, f.store.Enqueue(ctxB, newEntry(tcB, "t")))

	f.exec(t, `DELETE FROM projects WHERE id = $1`, tcA.ProjectID)
	assert.Zero(t, f.count(t, `SELECT count(*) FROM outbox_entries WHERE org_id = $1`, tcA.OrgID))
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM outbox_entries WHERE org_id = $1`, tcB.OrgID))

	f.exec(t, `DELETE FROM orgs WHERE id = $1`, tcB.OrgID)
	assert.Zero(t, f.count(t, `SELECT count(*) FROM outbox_entries`))
}

func TestEnqueue_HonorsTheUnitOfWork(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	rolledBack, committed := newEntry(tc, "t"), newEntry(tc, "t")
	boom := errors.New("boom")

	err := db.RunInTx(ctx, f.pool, func(ctx context.Context) error {
		require.NoError(t, f.store.Enqueue(ctx, rolledBack))
		_, err := f.store.ByID(ctx, rolledBack.ID, "t")
		require.NoError(t, err, "a read inside the unit of work must see its own write")
		return boom
	})
	require.ErrorIs(t, err, boom)
	_, err = f.store.ByID(ctx, rolledBack.ID, "t")
	assert.True(t, outbox.IsNotFoundError(err), "a rolled-back enqueue must leave nothing, got %v", err)

	require.NoError(t, db.RunInTx(ctx, f.pool, func(ctx context.Context) error {
		return f.store.Enqueue(ctx, committed)
	}))
	_, err = f.store.ByID(ctx, committed.ID, "t")
	assert.NoError(t, err)
}

func TestStore_RequiresATenant(t *testing.T) {
	f := newFixture(t)
	_, tc := f.seedTenant(t)
	ctx := t.Context()

	assert.True(t, tenant.IsMissingError(f.store.Enqueue(ctx, newEntry(tc, "t"))))
	_, err := f.store.ClaimDue(ctx, time.Now(), 10)
	assert.True(t, tenant.IsMissingError(err))
	_, err = f.store.ByID(ctx, uuid.New(), "t")
	assert.True(t, tenant.IsMissingError(err))
	_, err = f.store.ListByTarget(ctx, "t", 10)
	assert.True(t, tenant.IsMissingError(err))
	_, err = f.store.RequeueFailed(ctx, "t")
	assert.True(t, tenant.IsMissingError(err))
}

func TestClaimDue_HonorsDueTimeLimitAndOrder(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	var dueIDs []uuid.UUID
	for i := range 5 {
		e := newEntry(tc, "t")
		e.NextAttemptAt = now.Add(-time.Duration(5-i) * time.Minute)
		require.NoError(t, f.store.Enqueue(ctx, e))
		dueIDs = append(dueIDs, e.ID)
	}
	atNow := newEntry(tc, "t")
	atNow.NextAttemptAt = now
	require.NoError(t, f.store.Enqueue(ctx, atNow))
	future := newEntry(tc, "t")
	future.NextAttemptAt = now.Add(time.Second)
	require.NoError(t, f.store.Enqueue(ctx, future))

	first, err := f.store.ClaimDue(ctx, now, 3)
	require.NoError(t, err)
	require.Len(t, first, 3)
	got := map[uuid.UUID]outbox.Entry{}
	for _, e := range first {
		got[e.ID] = e
		assert.Equal(t, 1, e.Attempt)
		assert.True(t, now.Add(outbox.ClaimLease).Equal(e.NextAttemptAt), "lease %s", e.NextAttemptAt)
		assert.Equal(t, outbox.StatusPending, e.Status)
	}
	for _, id := range dueIDs[:3] {
		assert.Contains(t, got, id, "the oldest due entries must be claimed first")
	}

	rest, err := f.store.ClaimDue(ctx, now, 100)
	require.NoError(t, err)
	ids := make([]uuid.UUID, 0, len(rest))
	for _, e := range rest {
		ids = append(ids, e.ID)
	}
	assert.ElementsMatch(t, []uuid.UUID{dueIDs[3], dueIDs[4], atNow.ID}, ids, "due is inclusive of now; future stays")

	stored, err := f.store.ByID(ctx, future.ID, "t")
	require.NoError(t, err)
	assert.Equal(t, 0, stored.Attempt)
}

func TestClaimDue_LimitIsClampedToAtLeastOne(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	for range 3 {
		require.NoError(t, f.store.Enqueue(ctx, newEntry(tc, "t")))
	}
	for _, limit := range []int{0, -5} {
		claimed, err := f.store.ClaimDue(ctx, time.Now(), limit)
		require.NoError(t, err)
		assert.Len(t, claimed, 1, "limit %d", limit)
	}
}

func TestClaimDue_LeaseHidesAClaimUntilItExpires(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	e := newEntry(tc, "t")
	require.NoError(t, f.store.Enqueue(ctx, e))
	now := time.Now().UTC()

	claimed, err := f.store.ClaimDue(ctx, now, 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	again, err := f.store.ClaimDue(ctx, now.Add(outbox.ClaimLease-time.Second), 10)
	require.NoError(t, err)
	assert.Empty(t, again, "a leased entry must not be re-claimed before the lease ends")

	reclaimed, err := f.store.ClaimDue(ctx, now.Add(outbox.ClaimLease), 10)
	require.NoError(t, err)
	require.Len(t, reclaimed, 1)
	assert.Equal(t, 2, reclaimed[0].Attempt)

	err = f.store.Succeed(ctx, claimed[0], time.Now())
	var stale *outbox.StaleClaimError
	require.ErrorAs(t, err, &stale, "the dispatcher whose lease expired must not settle")
	assert.Equal(t, 1, stale.Claimed)
	assert.Equal(t, 2, stale.Current)
	require.NoError(t, f.store.Succeed(ctx, reclaimed[0], time.Now()))
}

func TestClaimDue_OnlyClaimsTheCallersOrg(t *testing.T) {
	f := newFixture(t)
	ctxA, tcA := f.seedTenant(t)
	ctxB, tcB := f.seedTenant(t)
	a := newEntry(tcA, "t")
	require.NoError(t, f.store.Enqueue(ctxA, a))
	require.NoError(t, f.store.Enqueue(ctxB, newEntry(tcB, "t")))

	claimed, err := f.store.ClaimDue(ctxA, time.Now(), 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, a.ID, claimed[0].ID)
	assert.Equal(t, 0, f.count(t, `SELECT attempt FROM outbox_entries WHERE org_id = $1`, tcB.OrgID))
}

func TestClaimDue_SkipsRowsLockedByAnOpenClaim(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	locked, free := newEntry(tc, "t"), newEntry(tc, "t")
	locked.NextAttemptAt = past().Add(-time.Minute)
	free.NextAttemptAt = past()
	require.NoError(t, f.store.Enqueue(ctx, locked))
	require.NoError(t, f.store.Enqueue(ctx, free))

	tx, err := f.db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	held, err := f.store.ClaimDue(db.ContextWithTx(ctx, tx), time.Now(), 1)
	require.NoError(t, err)
	require.Len(t, held, 1)
	require.Equal(t, locked.ID, held[0].ID)

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	other, err := f.store.ClaimDue(waitCtx, time.Now(), 10)
	require.NoError(t, err, "a second claimer must skip locked rows, not block on them")
	require.Len(t, other, 1)
	assert.Equal(t, free.ID, other[0].ID)

	require.NoError(t, tx.Rollback())
	after, err := f.store.ClaimDue(ctx, time.Now(), 10)
	require.NoError(t, err)
	require.Len(t, after, 1, "the rolled-back claim must release its row")
	assert.Equal(t, locked.ID, after[0].ID)
	assert.Equal(t, 1, after[0].Attempt)
}

func TestClaimDue_ConcurrentClaimersNeverShareAnEntry(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	const total, workers = 40, 6
	for range total {
		e := newEntry(tc, "t")
		e.NextAttemptAt = past()
		require.NoError(t, f.store.Enqueue(ctx, e))
	}

	claimed := make([][]outbox.Entry, workers)
	errs := make([]error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range workers {
		wg.Go(func() {
			<-start
			for {
				batch, err := f.store.ClaimDue(ctx, time.Now(), 3)
				if err != nil || len(batch) == 0 {
					errs[i] = err
					return
				}
				claimed[i] = append(claimed[i], batch...)
			}
		})
	}
	close(start)
	wg.Wait()

	seen := map[uuid.UUID]int{}
	for i := range workers {
		require.NoError(t, errs[i])
		for _, e := range claimed[i] {
			seen[e.ID]++
			assert.Equal(t, 1, e.Attempt)
		}
	}
	assert.Len(t, seen, total)
	for id, n := range seen {
		assert.Equal(t, 1, n, "entry %s claimed %d times", id, n)
	}
}

func TestClaimDue_ReapsExhaustedEntries(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	exhausted, notYet := newEntry(tc, "t"), newEntry(tc, "t")
	require.NoError(t, f.store.Enqueue(ctx, exhausted))
	require.NoError(t, f.store.Enqueue(ctx, notYet))
	f.exec(t, `UPDATE outbox_entries SET attempt = $1, next_attempt_at = $2 WHERE id = $3`,
		outbox.MaxAttempts, past(), exhausted.ID)
	f.exec(t, `UPDATE outbox_entries SET attempt = $1, next_attempt_at = $2 WHERE id = $3`,
		outbox.MaxAttempts, time.Now().Add(time.Hour), notYet.ID)

	claimed, err := f.store.ClaimDue(ctx, time.Now(), 10)
	require.NoError(t, err)
	assert.Empty(t, claimed, "an exhausted entry must never be claimed again")

	got, err := f.store.ByID(ctx, exhausted.ID, "t")
	require.NoError(t, err)
	assert.Equal(t, outbox.StatusFailed, got.Status)
	got, err = f.store.ByID(ctx, notYet.ID, "t")
	require.NoError(t, err)
	assert.Equal(t, outbox.StatusPending, got.Status, "an exhausted entry still under lease is not reaped")
}

func claimOne(t *testing.T, ctx context.Context, f *fixture, tc tenant.Context, target string) outbox.Entry {
	t.Helper()
	e := newEntry(tc, target)
	e.NextAttemptAt = past()
	require.NoError(t, f.store.Enqueue(ctx, e))
	claimed, err := f.store.ClaimDue(ctx, time.Now(), 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, e.ID, claimed[0].ID)
	return claimed[0]
}

func TestSucceed_SettlesAsDelivered(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	e := claimOne(t, ctx, f, tc, "t")
	f.exec(t, `UPDATE outbox_entries SET last_error = 'earlier' WHERE id = $1`, e.ID)
	at := time.Date(2026, 10, 1, 23, 30, 0, 0, time.FixedZone("WIB", 7*3600))

	require.NoError(t, f.store.Succeed(ctx, e, at))

	got, err := f.store.ByID(ctx, e.ID, "t")
	require.NoError(t, err)
	assert.Equal(t, outbox.StatusDelivered, got.Status)
	assert.Empty(t, got.LastError)
	assert.Equal(t, 1, got.Attempt)
	var deliveredAt time.Time
	require.NoError(t, f.db.QueryRowContext(t.Context(),
		`SELECT delivered_at FROM outbox_entries WHERE id = $1`, e.ID).Scan(&deliveredAt))
	assert.True(t, at.Equal(deliveredAt))

	err = f.store.Succeed(ctx, e, at)
	var term *outbox.TerminalStateError
	require.ErrorAs(t, err, &term)
	assert.Equal(t, outbox.StatusDelivered, term.Status)

	claimed, err := f.store.ClaimDue(ctx, time.Now().Add(outbox.ClaimLease*2), 10)
	require.NoError(t, err)
	assert.Empty(t, claimed, "a delivered entry is never claimed again")
}

func TestSettle_RefusesUnknownAndForeignEntries(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	otherCtx, _ := f.seedTenant(t)
	e := claimOne(t, ctx, f, tc, "t")

	unknown := e
	unknown.ID = uuid.New()
	assert.True(t, outbox.IsNotFoundError(f.store.Succeed(ctx, unknown, time.Now())))
	assert.True(t, outbox.IsNotFoundError(f.store.Fail(ctx, unknown, time.Now(), "x")))
	assert.True(t, outbox.IsNotFoundError(f.store.Succeed(otherCtx, e, time.Now())), "another org must not settle it")
	assert.True(t, outbox.IsNotFoundError(f.store.Fail(otherCtx, e, time.Now(), "x")))

	got, err := f.store.ByID(ctx, e.ID, "t")
	require.NoError(t, err)
	assert.Equal(t, outbox.StatusPending, got.Status)
}

func TestFail_ReschedulesThenSettlesAtMaxAttempts(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	e := claimOne(t, ctx, f, tc, "t")
	retryAt := time.Now().UTC().Add(time.Minute).Truncate(time.Microsecond)

	require.NoError(t, f.store.Fail(ctx, e, retryAt, "502 from endpoint"))
	got, err := f.store.ByID(ctx, e.ID, "t")
	require.NoError(t, err)
	assert.Equal(t, outbox.StatusPending, got.Status)
	assert.Equal(t, "502 from endpoint", got.LastError)
	assert.True(t, retryAt.Equal(got.NextAttemptAt))

	early, err := f.store.ClaimDue(ctx, retryAt.Add(-time.Second), 10)
	require.NoError(t, err)
	assert.Empty(t, early, "a failed entry is not claimable before its retry time")

	at := retryAt
	var last outbox.Entry
	for i := 2; i <= outbox.MaxAttempts; i++ {
		claimed, cErr := f.store.ClaimDue(ctx, at, 10)
		require.NoError(t, cErr)
		require.Len(t, claimed, 1, "attempt %d", i)
		require.Equal(t, i, claimed[0].Attempt)
		last = claimed[0]
		at = at.Add(time.Minute)
		require.NoError(t, f.store.Fail(ctx, last, at, "still failing"))
	}

	got, err = f.store.ByID(ctx, e.ID, "t")
	require.NoError(t, err)
	assert.Equal(t, outbox.StatusFailed, got.Status, "the MaxAttempts-th failure settles the entry")
	assert.Equal(t, outbox.MaxAttempts, got.Attempt)
	assert.Equal(t, "still failing", got.LastError)

	var term *outbox.TerminalStateError
	require.ErrorAs(t, f.store.Fail(ctx, last, at, "again"), &term)
	assert.Equal(t, outbox.StatusFailed, term.Status)

	claimed, err := f.store.ClaimDue(ctx, at.Add(time.Hour), 10)
	require.NoError(t, err)
	assert.Empty(t, claimed)
}

func TestFail_TruncatesTheCause(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	e := claimOne(t, ctx, f, tc, "t")
	cause := strings.Repeat("a", outbox.MaxCauseLen-1) + "é" + "tail"

	require.NoError(t, f.store.Fail(ctx, e, time.Now(), cause))
	got, err := f.store.ByID(ctx, e.ID, "t")
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("a", outbox.MaxCauseLen-1), got.LastError)
}

func markFailed(t *testing.T, f *fixture, id uuid.UUID) {
	t.Helper()
	f.exec(t, `UPDATE outbox_entries SET status = 'failed', attempt = $1, last_error = 'dead' WHERE id = $2`,
		outbox.MaxAttempts, id)
}

func TestRequeue_ResetsAFailedEntry(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	e := newEntry(tc, "t")
	require.NoError(t, f.store.Enqueue(ctx, e))
	markFailed(t, f, e.ID)

	before := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, f.store.Requeue(ctx, e.ID, "t"))
	got, err := f.store.ByID(ctx, e.ID, "t")
	require.NoError(t, err)
	assert.Equal(t, outbox.StatusPending, got.Status)
	assert.Equal(t, 0, got.Attempt)
	assert.Empty(t, got.LastError)
	assert.False(t, got.NextAttemptAt.Before(before))

	claimed, err := f.store.ClaimDue(ctx, time.Now(), 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, 1, claimed[0].Attempt)
}

func TestRequeue_Refusals(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	otherCtx, _ := f.seedTenant(t)
	pending := newEntry(tc, "t")
	require.NoError(t, f.store.Enqueue(ctx, pending))
	failed := newEntry(tc, "t")
	require.NoError(t, f.store.Enqueue(ctx, failed))
	markFailed(t, f, failed.ID)

	var nf *outbox.NotFailedError
	require.ErrorAs(t, f.store.Requeue(ctx, pending.ID, "t"), &nf)
	assert.Equal(t, outbox.StatusPending, nf.Status)

	assert.True(t, outbox.IsNotFoundError(f.store.Requeue(ctx, failed.ID, "other-target")))
	assert.True(t, outbox.IsNotFoundError(f.store.Requeue(ctx, uuid.New(), "t")))
	assert.True(t, outbox.IsNotFoundError(f.store.Requeue(otherCtx, failed.ID, "t")))

	got, err := f.store.ByID(ctx, failed.ID, "t")
	require.NoError(t, err)
	assert.Equal(t, outbox.StatusFailed, got.Status)
}

func TestRequeueFailed_TouchesOnlyFailedEntriesOfTheTargetInTheOrg(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	otherCtx, otherTC := f.seedTenant(t)

	var failedIDs []uuid.UUID
	for range 3 {
		e := newEntry(tc, "t")
		require.NoError(t, f.store.Enqueue(ctx, e))
		markFailed(t, f, e.ID)
		failedIDs = append(failedIDs, e.ID)
	}
	pending := newEntry(tc, "t")
	require.NoError(t, f.store.Enqueue(ctx, pending))
	delivered := claimOne(t, ctx, f, tc, "t")
	require.NoError(t, f.store.Succeed(ctx, delivered, time.Now()))
	otherTarget := newEntry(tc, "u")
	require.NoError(t, f.store.Enqueue(ctx, otherTarget))
	markFailed(t, f, otherTarget.ID)
	otherOrg := newEntry(otherTC, "t")
	require.NoError(t, f.store.Enqueue(otherCtx, otherOrg))
	markFailed(t, f, otherOrg.ID)

	n, err := f.store.RequeueFailed(ctx, "t")
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	for _, id := range failedIDs {
		got, gErr := f.store.ByID(ctx, id, "t")
		require.NoError(t, gErr)
		assert.Equal(t, outbox.StatusPending, got.Status)
		assert.Equal(t, 0, got.Attempt)
		assert.Empty(t, got.LastError)
	}

	statusOf := func(ctx context.Context, id uuid.UUID, target string) outbox.Status {
		got, gErr := f.store.ByID(ctx, id, target)
		require.NoError(t, gErr)
		return got.Status
	}
	assert.Equal(t, outbox.StatusDelivered, statusOf(ctx, delivered.ID, "t"))
	assert.Equal(t, outbox.StatusFailed, statusOf(ctx, otherTarget.ID, "u"))
	assert.Equal(t, outbox.StatusFailed, statusOf(otherCtx, otherOrg.ID, "t"))

	n, err = f.store.RequeueFailed(ctx, "t")
	require.NoError(t, err)
	assert.Zero(t, n)
}

func TestByID_IsScopedByOrgAndTarget(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	otherCtx, _ := f.seedTenant(t)
	e := newEntry(tc, "t")
	require.NoError(t, f.store.Enqueue(ctx, e))

	_, err := f.store.ByID(ctx, e.ID, "t")
	require.NoError(t, err)

	tests := []struct {
		name   string
		ctx    context.Context
		id     uuid.UUID
		target string
	}{
		{"wrong target", ctx, e.ID, "u"},
		{"other org", otherCtx, e.ID, "t"},
		{"unknown id", ctx, uuid.New(), "t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.store.ByID(tt.ctx, tt.id, tt.target)
			var nf *outbox.NotFoundError
			require.ErrorAs(t, err, &nf)
			assert.Equal(t, tt.id.String(), nf.ID)
		})
	}
}

func TestListByTarget_NewestFirstScopedAndLimited(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	otherCtx, otherTC := f.seedTenant(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	var ids []uuid.UUID
	for i := range 4 {
		e := newEntry(tc, "t")
		require.NoError(t, f.store.Enqueue(ctx, e))
		f.exec(t, `UPDATE outbox_entries SET created_at = $1 WHERE id = $2`, base.Add(time.Duration(i)*time.Hour), e.ID)
		ids = append(ids, e.ID)
	}
	tieA, tieB := newEntry(tc, "t"), newEntry(tc, "t")
	require.NoError(t, f.store.Enqueue(ctx, tieA))
	require.NoError(t, f.store.Enqueue(ctx, tieB))
	tie := base.Add(-time.Hour)
	f.exec(t, `UPDATE outbox_entries SET created_at = $1 WHERE id IN ($2, $3)`, tie, tieA.ID, tieB.ID)
	hi, lo := tieA.ID, tieB.ID
	if strings.Compare(hi.String(), lo.String()) < 0 {
		hi, lo = lo, hi
	}

	require.NoError(t, f.store.Enqueue(ctx, newEntry(tc, "u")))
	require.NoError(t, f.store.Enqueue(otherCtx, newEntry(otherTC, "t")))

	got, err := f.store.ListByTarget(ctx, "t", 100)
	require.NoError(t, err)
	gotIDs := make([]uuid.UUID, 0, len(got))
	for _, e := range got {
		gotIDs = append(gotIDs, e.ID)
		assert.Equal(t, tc.OrgID, e.OrgID)
		assert.Equal(t, "t", e.Target)
	}
	assert.Equal(t, []uuid.UUID{ids[3], ids[2], ids[1], ids[0], hi, lo}, gotIDs)

	limited, err := f.store.ListByTarget(ctx, "t", 2)
	require.NoError(t, err)
	require.Len(t, limited, 2)
	assert.Equal(t, ids[3], limited[0].ID)

	one, err := f.store.ListByTarget(ctx, "t", 0)
	require.NoError(t, err)
	assert.Len(t, one, 1, "a non-positive limit is clamped to one")

	none, err := f.store.ListByTarget(ctx, "nobody", 10)
	require.NoError(t, err)
	assert.NotNil(t, none)
	assert.Empty(t, none)
}

func TestListByTarget_CapsTheLimit(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	f.exec(t, `INSERT INTO outbox_entries (id, event_id, org_id, project_id, target, payload, next_attempt_at, created_at, public_id, event_public_id)
		SELECT gen_random_uuid(), gen_random_uuid(), $1, $2, 't', '\x00', now(), now(), gen_random_uuid()::text, 'e' FROM generate_series(1, $3)`,
		tc.OrgID, tc.ProjectID, outbox.MaxListLimit+5)

	got, err := f.store.ListByTarget(ctx, "t", outbox.MaxListLimit*10)
	require.NoError(t, err)
	assert.Len(t, got, outbox.MaxListLimit)
}

func TestEnqueue_PayloadNullability(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)

	empty := newEntry(tc, "t")
	empty.Payload = []byte{}
	require.NoError(t, f.store.Enqueue(ctx, empty))
	got, err := f.store.ByID(ctx, empty.ID, "t")
	require.NoError(t, err)
	assert.Empty(t, got.Payload)

	nilPayload := newEntry(tc, "t")
	nilPayload.Payload = nil
	err = f.store.Enqueue(ctx, nilPayload)
	require.Error(t, err, "a nil payload is not validated and reaches the NOT NULL constraint")
	assert.False(t, outbox.IsInvalidEntryError(err))
	assert.Contains(t, err.Error(), "payload")
}

func TestIDByPublicID(t *testing.T) {
	f := newFixture(t)
	ctx, tc := f.seedTenant(t)
	otherCtx, _ := f.seedTenant(t)
	e := newEntry(tc, "ep-1")
	require.NoError(t, f.store.Enqueue(ctx, e))

	got, err := f.store.IDByPublicID(ctx, e.PublicID, "ep-1")
	require.NoError(t, err)
	assert.Equal(t, e.ID, got)
	stored, err := f.store.ByID(ctx, e.ID, "ep-1")
	require.NoError(t, err)
	assert.Equal(t, e.PublicID, stored.PublicID)
	assert.Equal(t, e.EventPublicID, stored.EventPublicID)

	_, err = f.store.IDByPublicID(ctx, e.PublicID, "ep-2")
	assert.True(t, outbox.IsNotFoundError(err), "another target's delivery is not found")
	_, err = f.store.IDByPublicID(otherCtx, e.PublicID, "ep-1")
	assert.True(t, outbox.IsNotFoundError(err), "another org's delivery is not found")
	_, err = f.store.IDByPublicID(ctx, e.ID.String(), "ep-1")
	assert.True(t, outbox.IsNotFoundError(err), "a uuid is not a public id")
	_, err = f.store.IDByPublicID(t.Context(), e.PublicID, "ep-1")
	assert.True(t, tenant.IsMissingError(err))
}
