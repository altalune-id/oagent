//go:build integration

package agent_test

import (
	"context"
	"errors"
	"sync"
	"testing"

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

	vs, err := svc.Versions(ctx, a.ID)
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

	require.NoError(t, svc.Delete(ctx, a.ID))
	var left int
	require.NoError(t, f.sqlDB.QueryRowContext(t.Context(), `SELECT count(*) FROM agent_versions WHERE agent_id = $1`, a.ID).Scan(&left))
	assert.Zero(t, left, "versions go with their agent")
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
	vs, err := svc.Versions(ctx, a.ID)
	require.NoError(t, err)
	assert.Len(t, vs, 1)
}
