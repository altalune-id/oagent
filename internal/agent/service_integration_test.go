//go:build integration

package agent_test

import (
	"context"
	"errors"
	"testing"

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
			pub := &txPublisher{err: errors.New("outbox down")}

			_, err := f.service(pub).Create(t.Context(), agent.CreateRequest{OrgID: orgID, ProjectID: projectID, Slug: slug, Name: "Bot"})
			require.Error(t, err)
			listed, lerr := f.store.List(t.Context(), projectID)
			require.NoError(t, lerr)
			assert.Empty(t, listed, "an agent is never saved without its event")
		})
	}
}

func TestService_UpdateWithAFailedEventChangesNothing(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	a := newAgent(t, orgID, projectID, "bot")
	require.NoError(t, f.store.Save(t.Context(), a))

	_, err := f.service(&txPublisher{err: errors.New("outbox down")}).Update(t.Context(), a.ID, "Renamed", "new")
	require.Error(t, err)
	got, gerr := f.store.ByID(t.Context(), a.ID)
	require.NoError(t, gerr)
	assert.Equal(t, a.Name, got.Name, "an update is never saved without its event")
}

func TestService_EventsAreEnqueuedInTheSaveTransaction(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	pub := &txPublisher{}
	svc := f.service(pub)

	a, err := svc.Create(t.Context(), agent.CreateRequest{OrgID: orgID, ProjectID: projectID, Name: "Bot"})
	require.NoError(t, err)
	_, err = svc.Update(t.Context(), a.ID, "Renamed", "")
	require.NoError(t, err)
	assert.Equal(t, []bool{true, true}, pub.inTx)
}
