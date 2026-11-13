package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	"altalune.id/oagent/internal/knowledge"
)

func TestEffectiveConfig_ResolvesUnsetToTheCurrentRecommended(t *testing.T) {
	unset := effectiveToProto(Config{})
	assert.Equal(t, &agentv1.AgentEffective{
		MaxSteps: RecommendedMaxSteps, KnowledgeTopK: knowledge.RecommendedTopK,
		KnowledgeMinScore: knowledge.RecommendedMinScore, KnowledgeMaxTokens: knowledge.RecommendedMaxTokens,
	}, unset)

	zero := 0.0
	set := effectiveToProto(Config{MaxSteps: 7, KnowledgeTuning: KnowledgeTuning{TopK: 3, MinScore: &zero, MaxTokens: 600}})
	assert.Equal(t, &agentv1.AgentEffective{MaxSteps: 7, KnowledgeTopK: 3, KnowledgeMinScore: 0, KnowledgeMaxTokens: 600}, set, "a set 0 minimum stays 0")

	over, under := 5.0, -1.0
	clamped := effectiveToProto(Config{KnowledgeTuning: KnowledgeTuning{TopK: 50, MinScore: &over, MaxTokens: 99999}})
	assert.Equal(t, &agentv1.AgentEffective{
		MaxSteps: RecommendedMaxSteps, KnowledgeTopK: knowledge.MaxTopK, KnowledgeMinScore: 1, KnowledgeMaxTokens: knowledge.MaxMaxTokens,
	}, clamped, "the effective view clamps exactly as the run does")
	assert.Zero(t, effectiveToProto(Config{KnowledgeTuning: KnowledgeTuning{MinScore: &under}}).GetKnowledgeMinScore())

	a := &Agent{}
	assert.Equal(t, unset, agentToProto(a, "o", "p").GetEffectiveConfig(), "the agent carries its draft resolved")
	v := &Version{Config: Config{MaxSteps: 7}}
	assert.Equal(t, int32(7), versionToProto(v, "a", "").GetEffectiveConfig().GetMaxSteps(), "a version carries its config resolved")
}

// SECURITY: effective values are output only; no request may carry them back, so a client can't store resolved defaults as its own.
func TestNoRequestCarriesEffectiveConfig(t *testing.T) {
	effective := (&agentv1.AgentEffective{}).ProtoReflect().Descriptor().FullName()
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		svcs := fd.Services()
		for i := range svcs.Len() {
			methods := svcs.Get(i).Methods()
			for j := range methods.Len() {
				m := methods.Get(j)
				assert.False(t, carries(m.Input(), effective, map[protoreflect.FullName]bool{}), "%s takes %s", m.FullName(), effective)
			}
		}
		return true
	})
}

func carries(m protoreflect.MessageDescriptor, target protoreflect.FullName, seen map[protoreflect.FullName]bool) bool {
	if m.FullName() == target {
		return true
	}
	if seen[m.FullName()] {
		return false
	}
	seen[m.FullName()] = true
	fields := m.Fields()
	for i := range fields.Len() {
		if f := fields.Get(i).Message(); f != nil && carries(f, target, seen) {
			return true
		}
	}
	return false
}
