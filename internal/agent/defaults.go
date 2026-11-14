package agent

import (
	"time"

	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/run"
)

// The agent config ranges and recommended values; a zero MaxSteps runs with RecommendedMaxSteps.
const (
	RecommendedMaxSteps = 20
	MaxSteps            = 100
	MaxTemperature      = 2.0
	MaxCostCents        = 1000000
)

// EffectiveMaxSteps is the step cap a run uses: MaxSteps, or RecommendedMaxSteps when unset.
func (c Config) EffectiveMaxSteps() int32 {
	if c.MaxSteps > 0 {
		return c.MaxSteps
	}
	return RecommendedMaxSteps
}

func defaultsToProto() *agentv1.AgentDefaults {
	return &agentv1.AgentDefaults{
		MaxSteps: RecommendedMaxSteps, MaxMaxSteps: MaxSteps, MaxTemperature: MaxTemperature, MaxCostCents: MaxCostCents,
		VoiceNoteMaxSeconds: int32(run.MaxVoiceLength / time.Second), VoiceNoteMaxBytes: run.MaxVoiceBytes,
	}
}

func effectiveToProto(c Config) *agentv1.AgentEffective {
	k := c.KnowledgeTuning
	//nolint:gosec // G115: SearchK and MaxTokensOf clamp to MaxTopK and MaxMaxTokens.
	return &agentv1.AgentEffective{
		MaxSteps:           c.EffectiveMaxSteps(),
		KnowledgeTopK:      int32(knowledge.SearchK(int(k.TopK))),
		KnowledgeMinScore:  knowledge.MinScoreOf(knowledge.Query{MinScore: k.MinScore}),
		KnowledgeMaxTokens: int32(knowledge.MaxTokensOf(int(k.MaxTokens))),
	}
}
