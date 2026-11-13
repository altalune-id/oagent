//go:build integration

package knowledge_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	knowledgev1 "altalune.id/oagent/gen/go/knowledge/v1"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/scope"
)

// SECURITY: agent names and ids in used_by are shown only to a caller that may read agents.
func TestKnowledgeService_UsedByNeedsAgentRead(t *testing.T) {
	e := newEnv(t)
	src := e.uploadSource(t, "Policies")
	used := []knowledge.UsedBy{{AgentPublicID: "agentpublicid00000001", AgentName: "Support"}}
	c := newCP(t, e, e.svc.WithBindings(fakeBindings{used: map[uuid.UUID][]knowledge.UsedBy{src.ID: used}}))
	key := func(scopes ...string) context.Context {
		return session.PrincipalInto(context.Background(), session.Principal{
			Source: session.SourceAPIKey, ActiveOrgID: e.org, ProjectIDs: []uuid.UUID{e.prj}, Scopes: scopes,
		})
	}
	usedBy := func(ctx context.Context) (list, get []*knowledgev1.KnowledgeUsedBy) {
		l, err := c.client.ListKnowledgeSources(ctx, connect.NewRequest(&knowledgev1.ListKnowledgeSourcesRequest{ProjectId: c.prjPub}))
		require.NoError(t, err)
		require.Len(t, l.Msg.GetSources(), 1)
		g, err := c.client.GetKnowledgeSource(ctx, connect.NewRequest(&knowledgev1.GetKnowledgeSourceRequest{Id: src.PublicID}))
		require.NoError(t, err)
		return l.Msg.GetSources()[0].GetUsedBy(), g.Msg.GetSource().GetUsedBy()
	}

	list, get := usedBy(key(scope.KnowledgeRead))
	assert.Empty(t, list, "list without agent:read")
	assert.Empty(t, get, "get without agent:read")

	list, get = usedBy(key(scope.KnowledgeRead, scope.AgentRead))
	require.Len(t, list, 1)
	assert.Equal(t, "Support", list[0].GetAgentName())
	require.Len(t, get, 1)
	assert.Equal(t, "agentpublicid00000001", get[0].GetAgentId())
}

// SECURITY: a provider that no longer resolves renders empty and flagged, never as its UUID and never as an error.
func TestKnowledgeService_EmbeddingModelOfAGoneProvider(t *testing.T) {
	e := newEnv(t)
	svc := e.withEmbedding(1536, nil, fakeModels{"text-embedding-3-small": embeddingModels["text-embedding-3-small"]})
	_, err := svc.SetEmbeddingModel(e.ctx, e.prj, e.provider(t), "text-embedding-3-small")
	require.NoError(t, err)
	c := newCP(t, e, svc)

	l, err := c.client.ListKnowledgeSources(fakes.OperatorContext(context.Background()),
		connect.NewRequest(&knowledgev1.ListKnowledgeSourcesRequest{ProjectId: c.prjPub}))
	require.NoError(t, err)
	got := l.Msg.GetEmbeddingModel()
	assert.True(t, got.GetProviderMissing())
	assert.Empty(t, got.GetProviderId())
	assert.Equal(t, "text-embedding-3-small", got.GetModel())
}

// The console reads the upload limits from here instead of keeping its own copy.
func TestKnowledgeService_ListSendsTheUploadLimits(t *testing.T) {
	e := newEnv(t)
	c := newCP(t, e, e.svc)
	l, err := c.client.ListKnowledgeSources(fakes.OperatorContext(context.Background()),
		connect.NewRequest(&knowledgev1.ListKnowledgeSourcesRequest{ProjectId: c.prjPub}))
	require.NoError(t, err)
	assert.Equal(t, knowledge.UploadPurpose.MaxBytes, l.Msg.GetMaxUploadBytes())
	assert.Equal(t, knowledge.UploadPurpose.Types, l.Msg.GetAllowedContentTypes())
}

// SECURITY: a key limited to some resources sees only the agents it reaches among a source's users.
func TestKnowledgeService_UsedByShowsOnlyReachedAgents(t *testing.T) {
	e := newEnv(t)
	src := e.uploadSource(t, "Policies")
	mine, other := uuid.New(), uuid.New()
	used := []knowledge.UsedBy{
		{AgentID: mine, AgentPublicID: "agentpublicid00000001", AgentName: "Support"},
		{AgentID: other, AgentPublicID: "agentpublicid00000002", AgentName: "Billing"},
	}
	c := newCP(t, e, e.svc.WithBindings(fakeBindings{used: map[uuid.UUID][]knowledge.UsedBy{src.ID: used}}))
	names := func(p session.Principal) (list, get []string) {
		ctx := session.PrincipalInto(context.Background(), p)
		l, err := c.client.ListKnowledgeSources(ctx, connect.NewRequest(&knowledgev1.ListKnowledgeSourcesRequest{ProjectId: c.prjPub}))
		require.NoError(t, err)
		require.Len(t, l.Msg.GetSources(), 1)
		g, err := c.client.GetKnowledgeSource(ctx, connect.NewRequest(&knowledgev1.GetKnowledgeSourceRequest{Id: src.PublicID}))
		require.NoError(t, err)
		for _, u := range l.Msg.GetSources()[0].GetUsedBy() {
			list = append(list, u.GetAgentName())
		}
		for _, u := range g.Msg.GetSource().GetUsedBy() {
			get = append(get, u.GetAgentName())
		}
		return list, get
	}
	scopes := []string{scope.KnowledgeRead, scope.AgentRead}

	list, get := names(session.Principal{Source: session.SourceAPIKey, ActiveOrgID: e.org, ProjectIDs: []uuid.UUID{e.prj}, Scopes: scopes})
	assert.Equal(t, []string{"Support", "Billing"}, list, "a project key reaches every agent")
	assert.Equal(t, []string{"Support", "Billing"}, get)

	limited := session.Principal{Source: session.SourceAPIKey, ActiveOrgID: e.org, ProjectIDs: []uuid.UUID{e.prj}, ResourceIDs: []uuid.UUID{src.ID, mine}, Scopes: scopes}
	list, get = names(limited)
	assert.Equal(t, []string{"Support"}, list, "list")
	assert.Equal(t, []string{"Support"}, get, "get")
}
