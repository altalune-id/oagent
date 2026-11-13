package knowledge_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/knowledge"
)

func TestPresets_InsideTheRangesWithBalancedRecommended(t *testing.T) {
	presets := knowledge.Presets()
	names := make([]string, len(presets))
	for i, p := range presets {
		names[i] = p.Name
		assert.GreaterOrEqual(t, p.TopK, knowledge.MinTopK, p.Name)
		assert.LessOrEqual(t, p.TopK, knowledge.MaxTopK, p.Name)
		assert.GreaterOrEqual(t, p.MinScore, knowledge.MinMinScore, p.Name)
		assert.LessOrEqual(t, p.MinScore, knowledge.MaxMinScore, p.Name)
	}
	assert.Equal(t, []string{"precise", "balanced", "broad"}, names)
	assert.Equal(t, knowledge.Preset{Name: "balanced", TopK: knowledge.RecommendedTopK, MinScore: knowledge.RecommendedMinScore}, presets[1])
	assert.Equal(t, 5, knowledge.RecommendedTopK)
	assert.InDelta(t, 0.20, knowledge.RecommendedMinScore, 1e-9)
	assert.Equal(t, 4000, knowledge.RecommendedMaxTokens)
}
