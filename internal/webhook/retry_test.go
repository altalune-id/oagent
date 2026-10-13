package webhook

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	webhookv1 "altalune.id/oagent/gen/go/webhook/v1"
	"altalune.id/oagent/internal/kernel/outbox"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/nanoid"
)

func TestServiceRetry(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project}
	ctx := tenantCtx(org, project)

	ok := testService(t, &fakeStore{endpoints: []*Endpoint{e}}, &fakeOutbox{})
	require.NoError(t, ok.Retry(ctx, e.ID, uuid.New()))

	missing := testService(t, &fakeStore{endpoints: []*Endpoint{e}}, &fakeOutbox{})
	assert.True(t, IsNotFoundError(missing.Retry(ctx, uuid.New(), uuid.New())))

	notFailed := testService(t, &fakeStore{endpoints: []*Endpoint{e}}, &fakeOutbox{requeueErr: &outbox.NotFailedError{ID: "x"}})
	assert.True(t, IsDeliveryNotRetryableError(notFailed.Retry(ctx, e.ID, uuid.New())))

	gone := testService(t, &fakeStore{endpoints: []*Endpoint{e}}, &fakeOutbox{requeueErr: &outbox.NotFoundError{ID: "x"}})
	assert.True(t, IsDeliveryNotFoundError(gone.Retry(ctx, e.ID, uuid.New())))

	boom := testService(t, &fakeStore{endpoints: []*Endpoint{e}}, &fakeOutbox{requeueErr: errors.New("boom")})
	assert.Error(t, boom.Retry(ctx, e.ID, uuid.New()))
}

func TestServiceRetryFailed(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	e := &Endpoint{ID: uuid.New(), OrgID: org, ProjectID: project}
	ctx := tenantCtx(org, project)

	svc := testService(t, &fakeStore{endpoints: []*Endpoint{e}}, &fakeOutbox{requeued: 3})
	n, err := svc.RetryFailed(ctx, e.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	_, err = svc.RetryFailed(ctx, uuid.New())
	assert.True(t, IsNotFoundError(err))

	boom := testService(t, &fakeStore{endpoints: []*Endpoint{e}}, &fakeOutbox{requeueFailedErr: errors.New("boom")})
	_, err = boom.RetryFailed(ctx, e.ID)
	assert.Error(t, err)
}

func TestControlPlaneRetry(t *testing.T) {
	s := newScope()
	e := s.endpoint()
	entry := outbox.Entry{ID: uuid.New(), PublicID: nanoid.MustNew(21), Target: e.ID.String(), Status: outbox.StatusFailed}
	cp := NewControlPlane(testService(t, &fakeStore{endpoints: []*Endpoint{e}}, &fakeOutbox{entries: []outbox.Entry{entry}, requeued: 2}), s.refs)
	ctx := session.PrincipalInto(context.Background(), session.System())

	_, err := cp.RetryWebhookDelivery(ctx, connect.NewRequest(&webhookv1.RetryWebhookDeliveryRequest{
		OrgId: s.o, ProjectId: s.p, Id: e.PublicID, DeliveryId: entry.PublicID,
	}))
	require.NoError(t, err)

	for _, id := range []string{"bad", entry.ID.String()} {
		_, err = cp.RetryWebhookDelivery(ctx, connect.NewRequest(&webhookv1.RetryWebhookDeliveryRequest{
			OrgId: s.o, ProjectId: s.p, Id: e.PublicID, DeliveryId: id,
		}))
		assert.True(t, IsDeliveryNotFoundError(err), "%s: %v", id, err)
	}

	resp, err := cp.RetryFailedWebhookDeliveries(ctx, connect.NewRequest(&webhookv1.RetryFailedWebhookDeliveriesRequest{
		OrgId: s.o, ProjectId: s.p, Id: e.PublicID,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(2), resp.Msg.GetRequeued())

	for _, id := range []string{"bad", e.ID.String()} {
		_, err = cp.RetryFailedWebhookDeliveries(ctx, connect.NewRequest(&webhookv1.RetryFailedWebhookDeliveriesRequest{
			OrgId: s.o, ProjectId: s.p, Id: id,
		}))
		assert.True(t, IsNotFoundError(err), "%s: %v", id, err)
	}
}
