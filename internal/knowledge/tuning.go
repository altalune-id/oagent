package knowledge

import knowledgev1 "altalune.id/oagent/gen/go/knowledge/v1"

// The retrieval tuning an agent may set: the recommended values, the allowed ranges and the presets.
const (
	RecommendedTopK      = 5
	MinTopK              = 1
	MaxTopK              = 10
	RecommendedMinScore  = 0.20
	MinMinScore          = 0.0
	MaxMinScore          = 1.0
	RecommendedMaxTokens = 4000
	MinMaxTokens         = 500
	MaxMaxTokens         = 8000
)

// Preset is a named pair of passages per answer and minimum relevance.
type Preset struct {
	Name     string
	TopK     int
	MinScore float64
}

// Presets returns the tuning presets, narrowest first; balanced holds the recommended values.
func Presets() []Preset {
	return []Preset{
		{Name: "precise", TopK: 3, MinScore: 0.35},
		{Name: "balanced", TopK: RecommendedTopK, MinScore: RecommendedMinScore},
		{Name: "broad", TopK: 8, MinScore: 0.10},
	}
}

func retrievalDefaults() *knowledgev1.RetrievalDefaults {
	out := &knowledgev1.RetrievalDefaults{
		TopK: RecommendedTopK, MinScore: RecommendedMinScore, MaxTokens: RecommendedMaxTokens,
		MinTopK: MinTopK, MaxTopK: MaxTopK, MinMinScore: MinMinScore, MaxMinScore: MaxMinScore,
		MinMaxTokens: MinMaxTokens, MaxMaxTokens: MaxMaxTokens,
	}
	for _, p := range Presets() {
		//nolint:gosec // G115: preset TopK values are constants within MaxTopK.
		out.Presets = append(out.Presets, &knowledgev1.RetrievalPreset{Name: p.Name, TopK: int32(p.TopK), MinScore: p.MinScore})
	}
	return out
}
