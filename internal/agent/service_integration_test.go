//go:build integration

package agent_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/events"
)

type txPublisher struct {
	err  error
	inTx []bool
}

func (p *txPublisher) Enqueue(ctx context.Context, _ events.Type, _ any) error {
	_, ok := db.CurrentTx(ctx)
	p.inTx = append(p.inTx, ok)
	return p.err
}

func (f *pgFixture) service(pub agent.Publisher) *agent.Service {
	return agent.NewService(f.store, db.NewTransactor(f.pool), apperror.NewReporter(nil, false).Unexpected).WithPublisher(pub)
}

func TestService_CreateWithAFailedEventSavesNothing(t *testing.T) {
	for _, slug := range []string{"named", ""} {
		t.Run("slug="+slug, func(t *testing.T) {
			f := newPgFixture(t)
			orgID, projectID := f.seedScope(t)
			ctx := in(orgID)
			pub := &txPublisher{err: errors.New("outbox down")}

			_, err := f.service(pub).Create(ctx, agent.CreateRequest{OrgID: orgID, ProjectID: projectID, Slug: slug, Name: "Bot"})
			require.Error(t, err)
			listed, lerr := f.store.List(ctx, projectID)
			require.NoError(t, lerr)
			assert.Empty(t, listed, "an agent is never saved without its event")
		})
	}
}

func TestService_UpdateWithAFailedEventChangesNothing(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	a := newAgent(t, orgID, projectID, "bot")
	require.NoError(t, f.store.Save(ctx, a))

	_, err := f.service(&txPublisher{err: errors.New("outbox down")}).Update(ctx, a.ID, "Renamed", "new")
	require.Error(t, err)
	got, gerr := f.store.ByID(ctx, a.ID)
	require.NoError(t, gerr)
	assert.Equal(t, a.Name, got.Name, "an update is never saved without its event")
}

func TestService_EventsAreEnqueuedInTheSaveTransaction(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	pub := &txPublisher{}
	svc := f.service(pub)

	a, err := svc.Create(ctx, agent.CreateRequest{OrgID: orgID, ProjectID: projectID, Name: "Bot"})
	require.NoError(t, err)
	_, err = svc.Update(ctx, a.ID, "Renamed", "")
	require.NoError(t, err)
	assert.Equal(t, []bool{true, true}, pub.inTx)
}

func TestService_DraftPublishAndVersionsInPostgres(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	svc := f.service(nil)
	a, err := svc.Create(ctx, agent.CreateRequest{OrgID: orgID, ProjectID: projectID, Slug: "support", Name: "Support"})
	require.NoError(t, err)
	cfg := agent.Config{Instructions: "Be brief.", Model: "m-1", Temperature: 0.4, Tools: []agent.Tool{{Name: "refund", RequireApproval: true}}}

	_, err = svc.UpdateDraft(ctx, a.ID, cfg)
	require.NoError(t, err)
	got, err := f.store.ByID(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, cfg, got.Draft, "the draft round-trips through JSONB")
	assert.True(t, got.Unpublished())

	by := uuid.New()
	_, v1, created, err := svc.Publish(ctx, a.ID, by)
	require.NoError(t, err)
	require.True(t, created)
	cfg.Model = "m-2"
	_, err = svc.UpdateDraft(ctx, a.ID, cfg)
	require.NoError(t, err)
	_, v2, created, err := svc.Publish(ctx, a.ID, uuid.Nil)
	require.NoError(t, err)
	require.True(t, created)

	vs, err := svc.Versions(ctx, a.ID, 0)
	require.NoError(t, err)
	require.Len(t, vs, 2)
	assert.Equal(t, []int32{2, 1}, []int32{vs[0].Version, vs[1].Version}, "newest first")
	assert.Equal(t, v2.PublicID, vs[0].PublicID)
	assert.Equal(t, uuid.NullUUID{UUID: by, Valid: true}, vs[1].CreatedBy)
	assert.Equal(t, "m-1", vs[1].Config.Model, "a version never changes")
	got, err = f.store.ByID(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(2), got.PublishedVersion)
	assert.False(t, got.Unpublished())

	dup := *v1
	dup.ID, dup.PublicID = uuid.New(), "dup"+v1.PublicID[3:]
	assert.Equal(t, "23505", pgCode(f.store.InsertVersion(ctx, &dup)), "one version number per agent")

	for _, stmt := range []string{`UPDATE agent_versions SET version = 99 WHERE agent_id = $1`, `DELETE FROM agent_versions WHERE agent_id = $1`} {
		var n int64
		require.NoError(t, db.Write(ctx, f.pool, func(q qrm.DB) error {
			res, err := q.ExecContext(ctx, stmt, a.ID)
			if err == nil {
				n, err = res.RowsAffected()
			}
			return err
		}))
		assert.Zero(t, n, "%s: a version never changes or goes, even in its own org", stmt)
	}

	require.NoError(t, svc.Delete(ctx, a.ID))
	var left int
	require.NoError(t, f.sqlDB.QueryRowContext(t.Context(), `SELECT count(*) FROM agent_versions WHERE agent_id = $1`, a.ID).Scan(&left))
	assert.Zero(t, left, "versions go with their agent")
}

func TestService_VersionsHonoursTheLimitInSQL(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	svc := f.service(nil)
	a, err := svc.Create(ctx, agent.CreateRequest{OrgID: orgID, ProjectID: projectID, Slug: "versions", Name: "Versions"})
	require.NoError(t, err)
	for _, model := range []string{"m-1", "m-2", "m-3"} {
		_, err = svc.UpdateDraft(ctx, a.ID, agent.Config{Model: model})
		require.NoError(t, err)
		_, _, _, err = svc.Publish(ctx, a.ID, uuid.Nil)
		require.NoError(t, err)
	}
	vs, err := svc.Versions(ctx, a.ID, 1)
	require.NoError(t, err)
	require.Len(t, vs, 1, "the console asks for the live version only")
	assert.Equal(t, int32(3), vs[0].Version)
	vs, err = svc.Versions(ctx, a.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, []int32{3, 2, 1}, []int32{vs[0].Version, vs[1].Version, vs[2].Version})
}

func TestService_ConcurrentPublishesCreateOneVersion(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	svc := f.service(nil)
	a, err := svc.Create(ctx, agent.CreateRequest{OrgID: orgID, ProjectID: projectID, Slug: "racer", Name: "Racer"})
	require.NoError(t, err)
	_, err = svc.UpdateDraft(ctx, a.ID, agent.Config{Model: "m-1"})
	require.NoError(t, err)

	const n = 5
	var wg sync.WaitGroup
	results := make([]bool, n)
	errs := make([]error, n)
	for i := range n {
		wg.Go(func() { _, _, results[i], errs[i] = svc.Publish(ctx, a.ID, uuid.Nil) })
	}
	wg.Wait()
	created := 0
	for i := range n {
		require.NoError(t, errs[i], "the row lock queues publishes, so none hits the unique version")
		if results[i] {
			created++
		}
	}
	assert.Equal(t, 1, created)
	vs, err := svc.Versions(ctx, a.ID, 0)
	require.NoError(t, err)
	assert.Len(t, vs, 1)
}

// NOTE: pausedPublish runs during once, between the publish row lock and its commit.
type pausedPublish struct {
	agent.Store
	once   sync.Once
	during func()
}

func (p *pausedPublish) LatestVersion(ctx context.Context, agentID uuid.UUID) (*agent.Version, error) {
	p.once.Do(p.during)
	return p.Store.LatestVersion(ctx, agentID)
}

func TestService_DraftEditDuringAPublishKeepsTheVersion(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	svc := f.service(nil)
	a, err := svc.Create(ctx, agent.CreateRequest{OrgID: orgID, ProjectID: projectID, Slug: "racer", Name: "Racer"})
	require.NoError(t, err)
	_, err = svc.UpdateDraft(ctx, a.ID, agent.Config{Model: "m-1"})
	require.NoError(t, err)

	var edit sync.WaitGroup
	var editErr error
	paused := &pausedPublish{Store: f.store, during: func() {
		edit.Go(func() { _, editErr = svc.UpdateDraft(ctx, a.ID, agent.Config{Model: "m-2"}) })
		time.Sleep(200 * time.Millisecond)
	}}
	publisher := agent.NewService(paused, db.NewTransactor(f.pool), apperror.NewReporter(nil, false).Unexpected)
	_, v1, created, err := publisher.Publish(ctx, a.ID, uuid.Nil)
	require.NoError(t, err)
	require.True(t, created)
	edit.Wait()
	require.NoError(t, editErr)

	got, err := f.store.ByID(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(1), got.PublishedVersion, "the draft edit never writes the publish state back")
	assert.Equal(t, "m-2", got.Draft.Model)
	_, v2, created, err := svc.Publish(ctx, a.ID, uuid.Nil)
	require.NoError(t, err, "the next publish finds a free version number")
	assert.True(t, created)
	assert.Equal(t, []int32{1, 2}, []int32{v1.Version, v2.Version})
}

func TestService_DraftEditDuringAPublishStaysUnpublished(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	svc := f.service(nil)
	a, err := svc.Create(ctx, agent.CreateRequest{OrgID: orgID, ProjectID: projectID, Slug: "late-edit", Name: "Late edit"})
	require.NoError(t, err)
	_, err = svc.UpdateDraft(ctx, a.ID, agent.Config{Model: "m-1"})
	require.NoError(t, err)

	var edit sync.WaitGroup
	var editErr error
	paused := &pausedPublish{Store: f.store, during: func() {
		edit.Go(func() { _, editErr = svc.UpdateDraft(ctx, a.ID, agent.Config{Model: "m-2"}) })
		time.Sleep(200 * time.Millisecond)
	}}
	publisher := agent.NewService(paused, db.NewTransactor(f.pool), apperror.NewReporter(nil, false).Unexpected)
	_, _, _, err = publisher.Publish(ctx, a.ID, uuid.Nil)
	require.NoError(t, err)
	edit.Wait()
	require.NoError(t, editErr)

	got, err := f.store.ByID(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, "m-2", got.Draft.Model)
	assert.True(t, got.Unpublished(), "an edit that waited on a publish lands after it, so it is not yet live")
}

// Run with -race: saving never-published agents at once shares no SQL expression.
func TestService_ConcurrentCreatesShareNoExpression(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	svc := f.service(nil)
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Go(func() {
			_, errs[i] = svc.Create(ctx, agent.CreateRequest{OrgID: orgID, ProjectID: projectID, Name: "Bot"})
		})
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
}
