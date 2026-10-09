package webhook

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/outbox"
)

func TestServiceDeliveries(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project}
	entry := outbox.Entry{ID: uuid.New(), Payload: []byte(`{"type":"agent.created"}`), Status: outbox.StatusDelivered, Attempt: 1}
	svc := testService(t, &fakeStore{endpoints: []*Endpoint{e}}, &fakeOutbox{entries: []outbox.Entry{entry}})

	got, err := svc.Deliveries(tenantCtx(org, project), e.ID, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, events.AgentCreated, got[0].EventType)

	_, err = svc.Deliveries(tenantCtx(org, project), uuid.New(), 10)
	assert.True(t, IsNotFoundError(err))

	bad := testService(t, &fakeStore{endpoints: []*Endpoint{e}}, &fakeOutbox{listErr: errors.New("boom")})
	_, err = bad.Deliveries(tenantCtx(org, project), e.ID, 10)
	assert.Error(t, err)
}

func TestServiceDeliveriesUnreadableEnvelope(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project}
	svc := testService(t, &fakeStore{endpoints: []*Endpoint{e}}, &fakeOutbox{entries: []outbox.Entry{{ID: uuid.New(), Payload: []byte("nope")}}})

	got, err := svc.Deliveries(tenantCtx(org, project), e.ID, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Empty(t, got[0].EventType)
}

func TestServiceAttempts(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project}
	svc := testService(t, &fakeStore{endpoints: []*Endpoint{e}, attempts: []Attempt{{Attempt: 1, StatusCode: 200}}}, &fakeOutbox{})

	got, err := svc.Attempts(tenantCtx(org, project), e.ID, uuid.New())
	require.NoError(t, err)
	assert.Len(t, got, 1)

	_, err = svc.Attempts(tenantCtx(org, project), uuid.New(), uuid.New())
	assert.True(t, IsNotFoundError(err))

	bad := testService(t, &fakeStore{endpoints: []*Endpoint{e}, attemptErr: errors.New("boom")}, &fakeOutbox{})
	_, err = bad.Attempts(tenantCtx(org, project), e.ID, uuid.New())
	assert.Error(t, err)
}
