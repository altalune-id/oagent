//go:build integration

package knowledge_test

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	knowledgev1 "altalune.id/oagent/gen/go/knowledge/v1"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/knowledge"
)

// NOTE: it refuses a query whose project differs from the context's, as the pgvector retriever does.
type projectGuardRetriever struct{}

func (projectGuardRetriever) Search(ctx context.Context, q knowledge.Query) ([]knowledge.Hit, knowledge.Usage, error) {
	if tc := tenant.Peek(ctx); tc.ProjectID != uuid.Nil && tc.ProjectID != q.ProjectID {
		return nil, knowledge.Usage{}, errors.New("the query's project is not the context's project")
	}
	return []knowledge.Hit{{Text: "We refund within 30 days.", Title: "FAQ"}}, knowledge.Usage{}, nil
}

// A member's session carries an active project; a source of another project of the same org is still searchable.
func TestKnowledgeService_TestSearchScopesToTheSourcesProject(t *testing.T) {
	e := newEnv(t)
	src := e.uploadSource(t, "Policies")
	c := newCP(t, e, e.svc.WithRetrievers(map[knowledge.Backend]knowledge.Retriever{knowledge.BackendPgvector: projectGuardRetriever{}}))
	member := session.Principal{
		Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: e.org, ActiveProjectID: uuid.New(), Scopes: []string{"*"},
	}
	ctx := session.WithTenant(session.PrincipalInto(context.Background(), member), member)

	got, err := c.client.TestSearch(ctx, connect.NewRequest(&knowledgev1.TestSearchRequest{SourceId: src.PublicID, Query: "refunds"}))
	require.NoError(t, err)
	assert.Len(t, got.Msg.GetHits(), 1)
}
