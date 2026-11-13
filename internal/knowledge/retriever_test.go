package knowledge_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/knowledge"
)

func TestSearchK_ClampsToOneToTen(t *testing.T) {
	assert.Equal(t, 10, knowledge.MaxTopK)
	assert.Equal(t, knowledge.RecommendedTopK, knowledge.SearchK(0), "unset uses the recommended count")
	assert.Equal(t, 3, knowledge.SearchK(3))
	assert.Equal(t, 10, knowledge.SearchK(50))
}

func TestMinScoreOf_NilIsRecommendedAndZeroIsKept(t *testing.T) {
	zero, high, over := 0.0, 0.35, 1.5
	assert.InDelta(t, knowledge.RecommendedMinScore, knowledge.MinScoreOf(knowledge.Query{}), 1e-9)
	assert.Zero(t, knowledge.MinScoreOf(knowledge.Query{MinScore: &zero}), "0 is a valid minimum")
	assert.InDelta(t, 0.35, knowledge.MinScoreOf(knowledge.Query{MinScore: &high}), 1e-9)
	assert.InDelta(t, 1.0, knowledge.MinScoreOf(knowledge.Query{MinScore: &over}), 1e-9, "clamped to 0..1")
}
