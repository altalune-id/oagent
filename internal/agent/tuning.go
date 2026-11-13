package agent

import (
	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/knowledge"
)

func (k KnowledgeTuning) validate(invalid func(string, apperror.FieldReason) error) error {
	switch {
	case k.TopK != 0 && (k.TopK < knowledge.MinTopK || k.TopK > knowledge.MaxTopK):
		return invalid("knowledge_tuning.top_k", apperror.FieldInvalid)
	case k.MinScore != nil && !(*k.MinScore >= knowledge.MinMinScore && *k.MinScore <= knowledge.MaxMinScore):
		return invalid("knowledge_tuning.min_score", apperror.FieldInvalid)
	case k.MaxTokens != 0 && (k.MaxTokens < knowledge.MinMaxTokens || k.MaxTokens > knowledge.MaxMaxTokens):
		return invalid("knowledge_tuning.max_tokens", apperror.FieldInvalid)
	}
	return nil
}

func (k KnowledgeTuning) equal(o KnowledgeTuning) bool {
	sameScore := (k.MinScore == nil) == (o.MinScore == nil) && (k.MinScore == nil || *k.MinScore == *o.MinScore)
	return k.TopK == o.TopK && k.MaxTokens == o.MaxTokens && sameScore
}
