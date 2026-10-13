package webhook

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	webhookv1 "altalune.id/oagent/gen/go/webhook/v1"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/outbox"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
)

func TestWebhookReach(t *testing.T) {
	sb := newScope()
	sa := newScopeIn(sb.refs, uuid.New())
	orgA, orgB, prj := sa.org, sb.org, sb.project
	e := sb.endpoint(events.AgentCreated)
	cp := controlPlane(t, &fakeStore{endpoints: []*Endpoint{e}}, sb)
	ctx := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, AllProjects: true})
	b, p, id := sb.o, sb.p, e.PublicID

	_, err := cp.CreateWebhook(ctx, connect.NewRequest(&webhookv1.CreateWebhookRequest{OrgId: b, ProjectId: p, Url: "https://x.com", EventTypes: []string{string(events.AgentCreated)}}))
	assert.True(t, IsNotFoundError(err), "create: %v", err)
	_, err = cp.ListWebhooks(ctx, connect.NewRequest(&webhookv1.ListWebhooksRequest{OrgId: b, ProjectId: p}))
	assert.True(t, IsNotFoundError(err), "list: %v", err)
	_, err = cp.GetWebhook(ctx, connect.NewRequest(&webhookv1.GetWebhookRequest{OrgId: b, ProjectId: p, Id: id}))
	assert.True(t, IsNotFoundError(err), "get: %v", err)
	_, err = cp.DeleteWebhook(ctx, connect.NewRequest(&webhookv1.DeleteWebhookRequest{OrgId: b, ProjectId: p, Id: id}))
	assert.True(t, IsNotFoundError(err), "delete: %v", err)
	_, err = cp.RotateWebhookSecret(ctx, connect.NewRequest(&webhookv1.RotateWebhookSecretRequest{OrgId: b, ProjectId: p, Id: id}))
	assert.True(t, IsNotFoundError(err), "rotate: %v", err)

	_, err = cp.GetWebhook(ctx, connect.NewRequest(&webhookv1.GetWebhookRequest{OrgId: sa.o, ProjectId: p, Id: id}))
	assert.True(t, IsNotFoundError(err), "own org with another org's project: %v", err)
	_, err = cp.GetWebhook(ctx, connect.NewRequest(&webhookv1.GetWebhookRequest{OrgId: sa.o, ProjectId: sa.p, Id: id}))
	assert.True(t, IsNotFoundError(err), "another org's endpoint through an own project: %v", err)
	_, err = cp.GetWebhook(ctx, connect.NewRequest(&webhookv1.GetWebhookRequest{OrgId: orgB.String(), ProjectId: prj.String(), Id: e.ID.String()}))
	assert.True(t, IsNotFoundError(err), "uuids: %v", err)

	own := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgB, ProjectIDs: []uuid.UUID{prj}})
	got, err := cp.GetWebhook(own, connect.NewRequest(&webhookv1.GetWebhookRequest{OrgId: b, ProjectId: p, Id: id}))
	require.NoError(t, err)
	assert.Equal(t, e.PublicID, got.Msg.GetWebhook().GetId())
}

type projectOrgs map[uuid.UUID]uuid.UUID

func (m projectOrgs) OrgOf(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
	if org, ok := m[id]; ok {
		return org, nil
	}
	return uuid.Nil, &NotFoundError{}
}

func TestCreate_ProjectMustBelongToOrg(t *testing.T) {
	orgA, orgB, prjB := uuid.New(), uuid.New(), uuid.New()
	store := &fakeStore{}
	svc := NewService(store, nil, unexpectedStub, testSealer(t), &fakeOutbox{}, fakeResolver{}, WithProjects(projectOrgs{prjB: orgB}))

	_, _, err := svc.Create(tenantCtx(orgA, prjB), "https://e.com", "", []events.Type{events.AgentCreated})
	assert.True(t, tenant.IsProjectOutsideOrgError(err), "got %v", err)
	assert.Empty(t, store.endpoints)

	_, _, err = svc.Create(tenantCtx(orgB, prjB), "https://e.com", "", []events.Type{events.AgentCreated})
	require.NoError(t, err)
}

func TestOrgWideEventsNeedWholeOrgAndMemberRead(t *testing.T) {
	s := newScope()
	org, prj := s.org, s.project
	e := s.endpoint(events.AgentCreated)
	cp := controlPlane(t, &fakeStore{endpoints: []*Endpoint{e}}, s)
	o, p, id := s.o, s.p, e.PublicID
	callers := map[string]struct {
		p    session.Principal
		want bool
	}{
		"project-limited key":         {session.Principal{Source: session.SourceAPIKey, ActiveOrgID: org, ProjectIDs: []uuid.UUID{prj}, Scopes: []string{"webhook:write", "member:read"}}, false},
		"org key without member:read": {session.Principal{Source: session.SourceAPIKey, ActiveOrgID: org, AllProjects: true, Scopes: []string{"webhook:write"}}, false},
		"org key with member:read":    {session.Principal{Source: session.SourceAPIKey, ActiveOrgID: org, AllProjects: true, Scopes: []string{"webhook:write", "member:read"}}, true},
		"person with member:read":     {session.Principal{Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: org, Scopes: []string{"webhook:write", "member:read"}}, true},
	}
	for name, c := range callers {
		t.Run(name, func(t *testing.T) {
			ctx := session.PrincipalInto(context.Background(), c.p)
			for _, typ := range []events.Type{events.InviteCreated, events.MemberAdded} {
				_, err := cp.CreateWebhook(ctx, connect.NewRequest(&webhookv1.CreateWebhookRequest{OrgId: o, ProjectId: p, Url: "https://x.com", EventTypes: []string{string(typ)}}))
				_, uerr := cp.UpdateWebhook(ctx, connect.NewRequest(&webhookv1.UpdateWebhookRequest{OrgId: o, ProjectId: p, Id: id, Url: "https://x.com", EventTypes: []string{string(typ)}, Active: true}))
				if c.want {
					require.NoError(t, err, "create %s", typ)
					require.NoError(t, uerr, "update %s", typ)
					continue
				}
				assert.True(t, authz.IsForbiddenError(err), "create %s: %v", typ, err)
				assert.True(t, authz.IsForbiddenError(uerr), "update %s: %v", typ, uerr)
			}
			_, err := cp.CreateWebhook(ctx, connect.NewRequest(&webhookv1.CreateWebhookRequest{OrgId: o, ProjectId: p, Url: "https://x.com", EventTypes: []string{string(events.AgentCreated)}}))
			require.NoError(t, err, "project events stay open")
		})
	}
}

func TestOrgWideDeliveryPayloadsAreHiddenFromProjectCallers(t *testing.T) {
	s := newScope()
	org, prj := s.org, s.project
	e := s.endpoint(events.InviteCreated, events.AgentCreated)
	entries := []outbox.Entry{
		{ID: uuid.New(), Payload: []byte(`{"type":"invite.created","data":{"email":"bob@x.io"}}`), Status: outbox.StatusDelivered, Attempt: 1},
		{ID: uuid.New(), Payload: []byte(`{"type":"agent.created"}`), Status: outbox.StatusDelivered, Attempt: 1},
	}
	cp := NewControlPlane(testService(t, &fakeStore{endpoints: []*Endpoint{e}}, &fakeOutbox{entries: entries}), s.refs)
	req := &webhookv1.ListWebhookDeliveriesRequest{OrgId: s.o, ProjectId: s.p, Id: e.PublicID, Limit: 10}

	limited := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, ActiveOrgID: org, ProjectIDs: []uuid.UUID{prj}, Scopes: []string{"webhook:read", "member:read"}})
	got, err := cp.ListWebhookDeliveries(limited, connect.NewRequest(req))
	require.NoError(t, err)
	require.Len(t, got.Msg.GetDeliveries(), 2)
	for _, d := range got.Msg.GetDeliveries() {
		if d.GetEventType() == string(events.InviteCreated) {
			assert.Empty(t, d.GetPayload(), "an org event's payload stays hidden")
		} else {
			assert.NotEmpty(t, d.GetPayload())
		}
	}

	whole := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: org, Scopes: []string{"webhook:read", "member:read"}})
	got, err = cp.ListWebhookDeliveries(whole, connect.NewRequest(req))
	require.NoError(t, err)
	for _, d := range got.Msg.GetDeliveries() {
		assert.NotEmpty(t, d.GetPayload(), d.GetEventType())
	}
}
