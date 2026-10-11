package webhook

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	webhookv1 "altalune.id/oagent/gen/go/webhook/v1"
	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
)

func TestWebhookReach(t *testing.T) {
	orgA, orgB, prj := uuid.New(), uuid.New(), uuid.New()
	e := &Endpoint{ID: uuid.New(), OrgID: orgB, ProjectID: prj, URL: "https://e.com", Active: true, EventTypes: []events.Type{events.AgentCreated}}
	cp, _, _ := controlPlane(t, &fakeStore{endpoints: []*Endpoint{e}})
	ctx := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, AllProjects: true})
	b, p, id := orgB.String(), prj.String(), e.ID.String()

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

	own := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgB, ProjectIDs: []uuid.UUID{prj}})
	_, err = cp.GetWebhook(own, connect.NewRequest(&webhookv1.GetWebhookRequest{OrgId: b, ProjectId: p, Id: id}))
	require.NoError(t, err)
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
