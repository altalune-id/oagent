package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/testutil/fakes"
)

type fakePublisher struct {
	types []events.Type
	data  []any
	err   error
}

func (f *fakePublisher) Enqueue(_ context.Context, t events.Type, data any) error {
	f.types = append(f.types, t)
	f.data = append(f.data, data)
	return f.err
}

func TestService_CreatePublishes(t *testing.T) {
	pub := &fakePublisher{}
	svc := agent.NewService(&fakes.AgentStore{}, fakes.Transactor{}, testUnexpected()).WithPublisher(pub)

	a, err := svc.Create(context.Background(), agent.CreateRequest{OrgID: uuid.New(), ProjectID: uuid.New(), Slug: "acme", Name: "Acme"})
	require.NoError(t, err)
	require.Len(t, pub.types, 1)
	assert.Equal(t, events.AgentCreated, pub.types[0])

	payload, ok := pub.data[0].(events.AgentCreatedV1)
	require.True(t, ok)
	assert.Equal(t, a.PublicID, payload.ID)
}

func TestService_CreateGeneratedPublishes(t *testing.T) {
	pub := &fakePublisher{}
	svc := agent.NewService(&fakes.AgentStore{}, fakes.Transactor{}, testUnexpected()).WithPublisher(pub)

	_, err := svc.Create(context.Background(), agent.CreateRequest{OrgID: uuid.New(), ProjectID: uuid.New(), Name: "Acme"})
	require.NoError(t, err)
	require.Len(t, pub.types, 1)
	assert.Equal(t, events.AgentCreated, pub.types[0])
}

func TestService_UpdatePublishes(t *testing.T) {
	existing, err := agent.New(uuid.New(), uuid.New(), "acme", "Acme", "", uuid.Nil)
	require.NoError(t, err)
	pub := &fakePublisher{}
	svc := agent.NewService(&fakes.AgentStore{Agent: existing}, fakes.Transactor{}, testUnexpected()).WithPublisher(pub)

	_, err = svc.Update(context.Background(), existing.ID, "Acme Two", "d")
	require.NoError(t, err)
	require.Len(t, pub.types, 1)
	assert.Equal(t, events.AgentUpdated, pub.types[0])
}

func TestService_PublishFailureFailsTheMutation(t *testing.T) {
	pub := &fakePublisher{err: errors.New("boom")}
	svc := agent.NewService(&fakes.AgentStore{}, fakes.Transactor{}, testUnexpected()).WithPublisher(pub)

	_, err := svc.Create(context.Background(), agent.CreateRequest{OrgID: uuid.New(), ProjectID: uuid.New(), Slug: "acme", Name: "Acme"})
	require.Error(t, err)
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperror.KindInternal, apperror.KindOf(ae.Code()), "a lost event is an unexpected error, never a silent success")
}

func TestService_NoPublisher(t *testing.T) {
	svc := agent.NewService(&fakes.AgentStore{}, fakes.Transactor{}, testUnexpected())
	_, err := svc.Create(context.Background(), agent.CreateRequest{OrgID: uuid.New(), ProjectID: uuid.New(), Slug: "acme", Name: "Acme"})
	require.NoError(t, err)
}
