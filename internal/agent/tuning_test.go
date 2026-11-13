package agent

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"buf.build/go/protovalidate"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	"altalune.id/oagent/internal/knowledge"
)

func fieldOf(t *testing.T, err error) string {
	t.Helper()
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok, "want a validation error, got %v", err)
	return ae.Meta()["field"]
}

func TestConfig_ValidateKnowledgeTuning(t *testing.T) {
	score := func(v float64) *float64 { return &v }
	for name, tc := range map[string]struct {
		tuning KnowledgeTuning
		field  string
	}{
		"recommended":      {KnowledgeTuning{}, ""},
		"lowest":           {KnowledgeTuning{TopK: knowledge.MinTopK, MinScore: score(knowledge.MinMinScore), MaxTokens: knowledge.MinMaxTokens}, ""},
		"highest":          {KnowledgeTuning{TopK: knowledge.MaxTopK, MinScore: score(knowledge.MaxMinScore), MaxTokens: knowledge.MaxMaxTokens}, ""},
		"top k too many":   {KnowledgeTuning{TopK: knowledge.MaxTopK + 1}, "config.knowledge_tuning.top_k"},
		"top k negative":   {KnowledgeTuning{TopK: -1}, "config.knowledge_tuning.top_k"},
		"score above one":  {KnowledgeTuning{MinScore: score(1.01)}, "config.knowledge_tuning.min_score"},
		"score negative":   {KnowledgeTuning{MinScore: score(-0.01)}, "config.knowledge_tuning.min_score"},
		"score NaN":        {KnowledgeTuning{MinScore: score(math.NaN())}, "config.knowledge_tuning.min_score"},
		"budget too small": {KnowledgeTuning{MaxTokens: knowledge.MinMaxTokens - 1}, "config.knowledge_tuning.max_tokens"},
		"budget too big":   {KnowledgeTuning{MaxTokens: knowledge.MaxMaxTokens + 1}, "config.knowledge_tuning.max_tokens"},
		"budget negative":  {KnowledgeTuning{MaxTokens: -1}, "config.knowledge_tuning.max_tokens"},
	} {
		t.Run(name, func(t *testing.T) {
			c := validConfig()
			c.KnowledgeTuning = tc.tuning
			err := c.Validate()
			if tc.field == "" {
				assert.NoError(t, err)
				return
			}
			assert.Equal(t, tc.field, fieldOf(t, err))
		})
	}
}

func TestConfig_EqualKnowledgeTuning(t *testing.T) {
	half, other := 0.5, 0.5
	base := Config{KnowledgeTuning: KnowledgeTuning{TopK: 3, MinScore: &half, MaxTokens: 1000}}
	same := Config{KnowledgeTuning: KnowledgeTuning{TopK: 3, MinScore: &other, MaxTokens: 1000}}
	assert.True(t, base.Equal(same), "equal scores at different addresses")
	zero := 0.0
	for name, c := range map[string]Config{
		"top k":       {KnowledgeTuning: KnowledgeTuning{TopK: 4, MinScore: &half, MaxTokens: 1000}},
		"score":       {KnowledgeTuning: KnowledgeTuning{TopK: 3, MinScore: &zero, MaxTokens: 1000}},
		"score unset": {KnowledgeTuning: KnowledgeTuning{TopK: 3, MaxTokens: 1000}},
		"budget":      {KnowledgeTuning: KnowledgeTuning{TopK: 3, MinScore: &half, MaxTokens: 2000}},
	} {
		assert.False(t, base.Equal(c), name)
	}
	assert.False(t, Config{}.Equal(Config{KnowledgeTuning: KnowledgeTuning{MinScore: &zero}}), "0 is a set minimum, not the recommended one")
}

func TestPublish_TuningOnlyChangeIsANewVersion(t *testing.T) {
	a, err := New(uuid.New(), uuid.New(), "bot", "Bot", "", uuid.Nil)
	require.NoError(t, err)
	cfg := validConfig()
	require.NoError(t, a.SetDraft(cfg, time.Now()))
	first, created, err := a.Publish(nil, uuid.Nil, time.Now())
	require.NoError(t, err)
	require.True(t, created)

	cfg.KnowledgeTuning = KnowledgeTuning{TopK: 8}
	require.NoError(t, a.SetDraft(cfg, time.Now()))
	second, created, err := a.Publish(first, uuid.Nil, time.Now())
	require.NoError(t, err)
	assert.True(t, created, "a tuning-only change publishes a new version")
	assert.Equal(t, first.Version+1, second.Version)
	assert.Equal(t, int32(8), second.Config.KnowledgeTuning.TopK)
}

func TestConfig_KnowledgeTuningJSON(t *testing.T) {
	raw, err := json.Marshal(Config{Model: "m"})
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "knowledge_tuning", "recommended values store nothing")

	zero := 0.0
	raw, err = json.Marshal(Config{KnowledgeTuning: KnowledgeTuning{TopK: 3, MinScore: &zero, MaxTokens: 600}})
	require.NoError(t, err)
	var back Config
	require.NoError(t, json.Unmarshal(raw, &back))
	require.NotNil(t, back.KnowledgeTuning.MinScore, "a set 0 survives storage")
	assert.Zero(t, *back.KnowledgeTuning.MinScore)
	assert.Equal(t, int32(3), back.KnowledgeTuning.TopK)
	assert.Equal(t, int32(600), back.KnowledgeTuning.MaxTokens)
}

func TestKnowledgeTuning_ProtoRoundTrip(t *testing.T) {
	zero := 0.0
	for name, k := range map[string]KnowledgeTuning{
		"recommended": {},
		"set":         {TopK: 3, MinScore: &zero, MaxTokens: 600},
		"score only":  {MinScore: &zero},
	} {
		cfg := Config{Model: "m", KnowledgeTuning: k}
		back := configFromProto(configToProto(cfg))
		assert.True(t, cfg.Equal(back), "%s: %+v", name, back.KnowledgeTuning)
	}
	pb := configToProto(Config{})
	assert.Zero(t, pb.GetKnowledgeTuning().GetTopK(), "recommended values are sent as 0 or unset")
	assert.Nil(t, pb.GetKnowledgeTuning().MinScore)
	assert.Zero(t, pb.GetKnowledgeTuning().GetMaxTokens())
	assert.Nil(t, configFromProto(&agentv1.AgentConfig{}).KnowledgeTuning.MinScore, "an unset tuning is the recommended one")
}

// SECURITY: the proto rules and Validate share the knowledge ranges, so an API call and a stored draft agree.
func TestKnowledgeTuning_ProtoRulesMatchTheRanges(t *testing.T) {
	v, err := protovalidate.New()
	require.NoError(t, err)
	check := func(k *agentv1.KnowledgeTuning) error { return v.Validate(&agentv1.AgentConfig{KnowledgeTuning: k}) }
	i32 := func(n int) int32 { return int32(n) }
	f64 := func(n float64) *float64 { return &n }

	assert.NoError(t, check(&agentv1.KnowledgeTuning{}), "0 and unset mean recommended")
	assert.NoError(t, check(&agentv1.KnowledgeTuning{TopK: i32(knowledge.MinTopK), MinScore: f64(knowledge.MinMinScore), MaxTokens: i32(knowledge.MinMaxTokens)}))
	assert.NoError(t, check(&agentv1.KnowledgeTuning{TopK: i32(knowledge.MaxTopK), MinScore: f64(knowledge.MaxMinScore), MaxTokens: i32(knowledge.MaxMaxTokens)}))
	for name, k := range map[string]*agentv1.KnowledgeTuning{
		"top k low":   {TopK: -1},
		"top k high":  {TopK: i32(knowledge.MaxTopK + 1)},
		"score low":   {MinScore: f64(knowledge.MinMinScore - 0.01)},
		"score high":  {MinScore: f64(knowledge.MaxMinScore + 0.01)},
		"budget low":  {MaxTokens: i32(knowledge.MinMaxTokens - 1)},
		"budget high": {MaxTokens: i32(knowledge.MaxMaxTokens + 1)},
	} {
		assert.Error(t, check(k), name)
	}
}
