package agent_test

import (
	"testing"

	"buf.build/go/protovalidate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	"altalune.id/oagent/internal/agent"
)

func TestDefaults_Published(t *testing.T) {
	assert.Equal(t, 20, agent.RecommendedMaxSteps)
	assert.Equal(t, 100, agent.MaxSteps)
	assert.Equal(t, 2.0, agent.MaxTemperature)
	assert.Equal(t, 1000000, agent.MaxCostCents)
	assert.LessOrEqual(t, agent.RecommendedMaxSteps, agent.MaxSteps)
}

func TestConfig_EffectiveMaxSteps(t *testing.T) {
	assert.Equal(t, int32(agent.RecommendedMaxSteps), agent.Config{}.EffectiveMaxSteps(), "0 runs with the recommended value")
	assert.Equal(t, int32(7), agent.Config{MaxSteps: 7}.EffectiveMaxSteps())
	assert.Equal(t, int32(agent.MaxSteps), agent.Config{MaxSteps: agent.MaxSteps}.EffectiveMaxSteps())
}

// SECURITY: the proto rules repeat the agent ranges; they must match the Go constants Validate uses, edge for edge.
func TestDefaults_ProtoRulesMatchTheRanges(t *testing.T) {
	v, err := protovalidate.New()
	require.NoError(t, err)
	check := func(c *agentv1.AgentConfig) error { return v.Validate(c) }
	assert.NoError(t, check(&agentv1.AgentConfig{MaxSteps: agent.MaxSteps, Temperature: agent.MaxTemperature, MaxCostCents: agent.MaxCostCents}))
	assert.NoError(t, check(&agentv1.AgentConfig{}))
	for name, c := range map[string]*agentv1.AgentConfig{
		"steps over":       {MaxSteps: agent.MaxSteps + 1},
		"steps negative":   {MaxSteps: -1},
		"temperature over": {Temperature: agent.MaxTemperature + 0.01},
		"temperature low":  {Temperature: -0.01},
		"cost over":        {MaxCostCents: agent.MaxCostCents + 1},
		"cost negative":    {MaxCostCents: -1},
	} {
		assert.Error(t, check(c), name)
		assert.Error(t, agent.Config{MaxSteps: c.GetMaxSteps(), Temperature: c.GetTemperature(), MaxCostCents: c.GetMaxCostCents()}.Validate(), "Validate agrees: %s", name)
	}
}
