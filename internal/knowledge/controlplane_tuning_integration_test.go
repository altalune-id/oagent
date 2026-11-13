//go:build integration

package knowledge_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	knowledgev1 "altalune.id/oagent/gen/go/knowledge/v1"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/testutil/fakes"
)

func TestKnowledgeService_ListAnswersTheRetrievalDefaults(t *testing.T) {
	e := newEnv(t)
	c := newCP(t, e, e.svc)
	list, err := c.client.ListKnowledgeSources(fakes.OperatorContext(context.Background()),
		connect.NewRequest(&knowledgev1.ListKnowledgeSourcesRequest{ProjectId: c.prjPub}))
	require.NoError(t, err)

	d := list.Msg.GetRetrievalDefaults()
	require.NotNil(t, d, "the console reads the tuning defaults from the server, even with no source yet")
	assert.Equal(t, int32(knowledge.RecommendedTopK), d.GetTopK())
	assert.InDelta(t, knowledge.RecommendedMinScore, d.GetMinScore(), 1e-9)
	assert.Equal(t, int32(knowledge.RecommendedMaxTokens), d.GetMaxTokens())
	assert.Equal(t, []int32{knowledge.MinTopK, knowledge.MaxTopK, knowledge.MinMaxTokens, knowledge.MaxMaxTokens},
		[]int32{d.GetMinTopK(), d.GetMaxTopK(), d.GetMinMaxTokens(), d.GetMaxMaxTokens()})
	assert.InDelta(t, knowledge.MinMinScore, d.GetMinMinScore(), 1e-9)
	assert.InDelta(t, knowledge.MaxMinScore, d.GetMaxMinScore(), 1e-9)
	presets := knowledge.Presets()
	require.Len(t, d.GetPresets(), len(presets))
	for i, p := range presets {
		assert.Equal(t, p.Name, d.GetPresets()[i].GetName())
		assert.Equal(t, int32(p.TopK), d.GetPresets()[i].GetTopK())
		assert.InDelta(t, p.MinScore, d.GetPresets()[i].GetMinScore(), 1e-9)
	}
}
