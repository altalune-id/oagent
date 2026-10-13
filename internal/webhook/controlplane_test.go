package webhook

import (
	"context"
	"strconv"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	webhookv1 "altalune.id/oagent/gen/go/webhook/v1"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/outbox"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/nanoid"
)

type testScope struct {
	org, project uuid.UUID
	o, p         string
	refs         *fakes.PublicIDs
}

func newScope() testScope {
	return newScopeIn(fakes.NewPublicIDs(), uuid.New())
}

func newScopeIn(refs *fakes.PublicIDs, org uuid.UUID) testScope {
	project := uuid.New()
	return testScope{org: org, project: project, o: refs.Org(org), p: refs.Project(project, org), refs: refs}
}

func (s testScope) endpoint(types ...events.Type) *Endpoint {
	return &Endpoint{ID: uuid.New(), PublicID: nanoid.MustNew(21), OrgID: s.org, ProjectID: s.project, URL: "https://e.com", Active: true, EventTypes: types}
}

func controlPlane(t *testing.T, store *fakeStore, s testScope) *WebhookControlPlane {
	t.Helper()
	return NewControlPlane(testService(t, store, &fakeOutbox{}), s.refs)
}

func systemCtx() context.Context {
	return session.PrincipalInto(context.Background(), session.System())
}

func TestControlPlaneCreate(t *testing.T) {
	s := newScope()
	store := &fakeStore{}
	resp, err := controlPlane(t, store, s).CreateWebhook(systemCtx(), connect.NewRequest(&webhookv1.CreateWebhookRequest{
		OrgId: s.o, ProjectId: s.p, Url: "https://e.com", EventTypes: []string{string(events.AgentCreated)},
	}))
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Msg.GetSecret())
	require.Len(t, store.saved, 1)
	assert.Equal(t, s.org, store.saved[0].OrgID)
	assert.Equal(t, s.project, store.saved[0].ProjectID)
	assert.Equal(t, s.o, resp.Msg.GetWebhook().GetOrgId())
	assert.Equal(t, s.p, resp.Msg.GetWebhook().GetProjectId())
	assert.Equal(t, store.saved[0].PublicID, resp.Msg.GetWebhook().GetId())
}

// SECURITY: an unknown, UUID-shaped or another org's org or project id answers the same not-found.
func TestControlPlaneUnknownScope(t *testing.T) {
	s := newScope()
	other := newScopeIn(s.refs, uuid.New())
	sibling := newScopeIn(s.refs, s.org)
	cp := controlPlane(t, &fakeStore{}, s)
	for name, ids := range map[string][2]string{
		"bad ids":                {"bad", "bad"},
		"uuid org":               {s.org.String(), s.p},
		"uuid project":           {s.o, s.project.String()},
		"project as org":         {s.p, s.p},
		"another org's project":  {s.o, other.p},
		"org of another project": {other.o, s.p},
	} {
		_, err := cp.CreateWebhook(systemCtx(), connect.NewRequest(&webhookv1.CreateWebhookRequest{
			OrgId: ids[0], ProjectId: ids[1], Url: "https://e.com", EventTypes: []string{string(events.AgentCreated)},
		}))
		assert.True(t, IsNotFoundError(err), "%s: %v", name, err)
	}
	_, err := cp.ListWebhooks(systemCtx(), connect.NewRequest(&webhookv1.ListWebhooksRequest{OrgId: sibling.o, ProjectId: sibling.p}))
	require.NoError(t, err, "a sibling project of the same org resolves")
}

func TestControlPlaneGetList(t *testing.T) {
	s := newScope()
	e := s.endpoint(events.AgentCreated)
	cp := controlPlane(t, &fakeStore{endpoints: []*Endpoint{e}}, s)

	got, err := cp.GetWebhook(systemCtx(), connect.NewRequest(&webhookv1.GetWebhookRequest{OrgId: s.o, ProjectId: s.p, Id: e.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, e.PublicID, got.Msg.GetWebhook().GetId())
	assert.Equal(t, s.o, got.Msg.GetWebhook().GetOrgId())

	list, err := cp.ListWebhooks(systemCtx(), connect.NewRequest(&webhookv1.ListWebhooksRequest{OrgId: s.o, ProjectId: s.p}))
	require.NoError(t, err)
	require.Len(t, list.Msg.GetWebhooks(), 1)
	assert.Equal(t, e.PublicID, list.Msg.GetWebhooks()[0].GetId())

	for _, id := range []string{"bad", e.ID.String()} {
		_, err = cp.GetWebhook(systemCtx(), connect.NewRequest(&webhookv1.GetWebhookRequest{OrgId: s.o, ProjectId: s.p, Id: id}))
		assert.True(t, IsNotFoundError(err), "%s: %v", id, err)
	}
}

// SECURITY: an endpoint of another project, even in the same org, is not found through this project.
func TestControlPlaneEndpointOfAnotherProject(t *testing.T) {
	s := newScope()
	sibling := newScopeIn(s.refs, s.org)
	e := sibling.endpoint(events.AgentCreated)
	cp := controlPlane(t, &fakeStore{endpoints: []*Endpoint{e}}, s)

	_, err := cp.GetWebhook(systemCtx(), connect.NewRequest(&webhookv1.GetWebhookRequest{OrgId: s.o, ProjectId: s.p, Id: e.PublicID}))
	assert.True(t, IsNotFoundError(err), "%v", err)
	_, err = cp.DeleteWebhook(systemCtx(), connect.NewRequest(&webhookv1.DeleteWebhookRequest{OrgId: s.o, ProjectId: s.p, Id: e.PublicID}))
	assert.True(t, IsNotFoundError(err), "%v", err)
}

func TestControlPlaneUpdateDelete(t *testing.T) {
	s := newScope()
	e := s.endpoint(events.AgentCreated)
	store := &fakeStore{endpoints: []*Endpoint{e}}
	cp := controlPlane(t, store, s)

	upd, err := cp.UpdateWebhook(systemCtx(), connect.NewRequest(&webhookv1.UpdateWebhookRequest{
		OrgId: s.o, ProjectId: s.p, Id: e.PublicID, Url: "https://e2.com", EventTypes: []string{string(events.AgentUpdated)}, Active: true,
	}))
	require.NoError(t, err)
	assert.Equal(t, "https://e2.com", upd.Msg.GetWebhook().GetUrl())
	assert.Equal(t, e.PublicID, upd.Msg.GetWebhook().GetId())

	_, err = cp.DeleteWebhook(systemCtx(), connect.NewRequest(&webhookv1.DeleteWebhookRequest{OrgId: s.o, ProjectId: s.p, Id: e.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{e.ID}, store.deleted)
}

func TestControlPlaneRotatePing(t *testing.T) {
	s := newScope()
	e := s.endpoint(events.AgentCreated)
	cp := controlPlane(t, &fakeStore{endpoints: []*Endpoint{e}}, s)

	rot, err := cp.RotateWebhookSecret(systemCtx(), connect.NewRequest(&webhookv1.RotateWebhookSecretRequest{OrgId: s.o, ProjectId: s.p, Id: e.PublicID}))
	require.NoError(t, err)
	assert.NotEmpty(t, rot.Msg.GetSecret())

	_, err = cp.PingWebhook(systemCtx(), connect.NewRequest(&webhookv1.PingWebhookRequest{OrgId: s.o, ProjectId: s.p, Id: e.PublicID}))
	require.NoError(t, err)
}

func TestControlPlaneErrors(t *testing.T) {
	s := newScope()
	cp := controlPlane(t, &fakeStore{}, s)
	ctx := systemCtx()
	id := nanoid.MustNew(21)

	_, err := cp.GetWebhook(ctx, connect.NewRequest(&webhookv1.GetWebhookRequest{OrgId: s.o, ProjectId: s.p, Id: id}))
	assert.True(t, IsNotFoundError(err), "get: %v", err)
	_, err = cp.UpdateWebhook(ctx, connect.NewRequest(&webhookv1.UpdateWebhookRequest{OrgId: s.o, ProjectId: s.p, Id: id, Url: "https://e.com", EventTypes: []string{"agent.created"}}))
	assert.True(t, IsNotFoundError(err), "update: %v", err)
	_, err = cp.DeleteWebhook(ctx, connect.NewRequest(&webhookv1.DeleteWebhookRequest{OrgId: s.o, ProjectId: s.p, Id: id}))
	assert.True(t, IsNotFoundError(err), "delete: %v", err)
	_, err = cp.RotateWebhookSecret(ctx, connect.NewRequest(&webhookv1.RotateWebhookSecretRequest{OrgId: s.o, ProjectId: s.p, Id: id}))
	assert.True(t, IsNotFoundError(err), "rotate: %v", err)
	_, err = cp.PingWebhook(ctx, connect.NewRequest(&webhookv1.PingWebhookRequest{OrgId: s.o, ProjectId: s.p, Id: id}))
	assert.True(t, IsNotFoundError(err), "ping: %v", err)

	cp2 := controlPlane(t, &fakeStore{listErr: errBoom}, s)
	_, err = cp2.ListWebhooks(ctx, connect.NewRequest(&webhookv1.ListWebhooksRequest{OrgId: s.o, ProjectId: s.p}))
	assert.Error(t, err)

	s.refs.Err = errBoom
	_, err = cp.ListWebhooks(ctx, connect.NewRequest(&webhookv1.ListWebhooksRequest{OrgId: s.o, ProjectId: s.p}))
	assert.ErrorIs(t, err, errBoom)
}

func TestControlPlaneBadProject(t *testing.T) {
	s := newScope()
	_, err := controlPlane(t, &fakeStore{}, s).ListWebhooks(systemCtx(), connect.NewRequest(&webhookv1.ListWebhooksRequest{OrgId: s.o, ProjectId: "bad"}))
	assert.True(t, IsNotFoundError(err), "%v", err)
}

func TestControlPlaneDeliveryLogs(t *testing.T) {
	s := newScope()
	e := s.endpoint()
	entry := outbox.Entry{ID: uuid.New(), PublicID: nanoid.MustNew(21), EventPublicID: nanoid.MustNew(21), Target: e.ID.String(), Payload: []byte(`{"type":"agent.created"}`)}
	ob := &fakeOutbox{entries: []outbox.Entry{entry}}
	store := &fakeStore{endpoints: []*Endpoint{e}, attempts: []Attempt{{Attempt: 1, StatusCode: 200}}}
	cp := NewControlPlane(testService(t, store, ob), s.refs)
	ctx := systemCtx()

	resp, err := cp.ListWebhookDeliveries(ctx, connect.NewRequest(&webhookv1.ListWebhookDeliveriesRequest{
		OrgId: s.o, ProjectId: s.p, Id: e.PublicID,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetDeliveries(), 1)
	d := resp.Msg.GetDeliveries()[0]
	assert.Equal(t, string(events.AgentCreated), d.GetEventType())
	assert.Equal(t, entry.PublicID, d.GetId())
	assert.Equal(t, entry.EventPublicID, d.GetEventId())

	ar, err := cp.ListWebhookDeliveryAttempts(ctx, connect.NewRequest(&webhookv1.ListWebhookDeliveryAttemptsRequest{
		OrgId: s.o, ProjectId: s.p, Id: e.PublicID, DeliveryId: entry.PublicID,
	}))
	require.NoError(t, err)
	require.Len(t, ar.Msg.GetAttempts(), 1)

	for _, id := range []string{"bad", entry.ID.String()} {
		_, err = cp.ListWebhookDeliveryAttempts(ctx, connect.NewRequest(&webhookv1.ListWebhookDeliveryAttemptsRequest{
			OrgId: s.o, ProjectId: s.p, Id: e.PublicID, DeliveryId: id,
		}))
		assert.True(t, IsDeliveryNotFoundError(err), "%s: %v", id, err)
	}
}

// SECURITY: a delivery of another endpoint is not found through this one.
func TestControlPlaneDeliveryOfAnotherEndpoint(t *testing.T) {
	s := newScope()
	mine, theirs := s.endpoint(), s.endpoint()
	entry := outbox.Entry{ID: uuid.New(), PublicID: nanoid.MustNew(21), Target: theirs.ID.String(), Status: outbox.StatusFailed}
	cp := NewControlPlane(testService(t, &fakeStore{endpoints: []*Endpoint{mine, theirs}}, &fakeOutbox{entries: []outbox.Entry{entry}}), s.refs)

	_, err := cp.RetryWebhookDelivery(systemCtx(), connect.NewRequest(&webhookv1.RetryWebhookDeliveryRequest{
		OrgId: s.o, ProjectId: s.p, Id: mine.PublicID, DeliveryId: entry.PublicID,
	}))
	assert.True(t, IsDeliveryNotFoundError(err), "%v", err)
	_, err = cp.ListWebhookDeliveryAttempts(systemCtx(), connect.NewRequest(&webhookv1.ListWebhookDeliveryAttemptsRequest{
		OrgId: s.o, ProjectId: s.p, Id: mine.PublicID, DeliveryId: entry.PublicID,
	}))
	assert.True(t, IsDeliveryNotFoundError(err), "%v", err)
}

func TestControlPlaneErrorCodes(t *testing.T) {
	ctx := systemCtx()
	s := newScope()
	full := &fakeStore{}
	for range MaxEndpointsPerProject {
		full.endpoints = append(full.endpoints, s.endpoint())
	}
	e := s.endpoint(events.AgentCreated)

	t.Run("http url", func(t *testing.T) {
		_, err := controlPlane(t, &fakeStore{}, s).CreateWebhook(ctx, connect.NewRequest(&webhookv1.CreateWebhookRequest{
			OrgId: s.o, ProjectId: s.p, Url: "http://x", EventTypes: []string{string(events.AgentCreated)},
		}))
		ae := appErr(t, err)
		assert.Equal(t, apperror.CodeWebhookInvalidURL, ae.Code())
		assert.Equal(t, "https_required", ae.Meta()["reason"])
	})
	t.Run("endpoint limit", func(t *testing.T) {
		_, err := controlPlane(t, full, s).CreateWebhook(ctx, connect.NewRequest(&webhookv1.CreateWebhookRequest{
			OrgId: s.o, ProjectId: s.p, Url: "https://e.com", EventTypes: []string{string(events.AgentCreated)},
		}))
		ae := appErr(t, err)
		assert.Equal(t, apperror.CodeWebhookEndpointLimit, ae.Code())
		assert.Equal(t, strconv.Itoa(MaxEndpointsPerProject), ae.Meta()["limit"])
	})
	t.Run("unknown id", func(t *testing.T) {
		for _, id := range []string{nanoid.MustNew(21), e.ID.String()} {
			_, err := controlPlane(t, &fakeStore{endpoints: []*Endpoint{e}}, s).GetWebhook(ctx, connect.NewRequest(&webhookv1.GetWebhookRequest{OrgId: s.o, ProjectId: s.p, Id: id}))
			assert.Equal(t, apperror.CodeWebhookNotFound, appErr(t, err).Code(), id)
		}
	})
	t.Run("unknown org", func(t *testing.T) {
		_, err := controlPlane(t, &fakeStore{}, s).ListWebhooks(ctx, connect.NewRequest(&webhookv1.ListWebhooksRequest{OrgId: s.org.String(), ProjectId: s.p}))
		assert.Equal(t, apperror.CodeWebhookNotFound, appErr(t, err).Code())
	})
	t.Run("retry a delivery that has not failed", func(t *testing.T) {
		entry := outbox.Entry{ID: uuid.New(), PublicID: nanoid.MustNew(21), Target: e.ID.String()}
		ob := &fakeOutbox{entries: []outbox.Entry{entry}, requeueErr: &outbox.NotFailedError{ID: "x"}}
		cp := NewControlPlane(testService(t, &fakeStore{endpoints: []*Endpoint{e}}, ob), s.refs)
		_, err := cp.RetryWebhookDelivery(ctx, connect.NewRequest(&webhookv1.RetryWebhookDeliveryRequest{
			OrgId: s.o, ProjectId: s.p, Id: e.PublicID, DeliveryId: entry.PublicID,
		}))
		assert.Equal(t, apperror.CodeWebhookDeliveryNotRetryable, appErr(t, err).Code())
	})
}
