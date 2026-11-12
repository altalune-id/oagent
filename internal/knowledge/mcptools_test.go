package knowledge_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"altalune.id/oagent/gen/go/knowledge/v1/knowledgev1mcp"
	"altalune.id/oagent/mcp"
)

// TestSearch spends embedding tokens on the project's provider, so MCP clients must treat it as a mutation.
func TestKnowledgeMCPTools_TestSearchIsAMutation(t *testing.T) {
	reg := mcp.NewRegistry()
	knowledgev1mcp.RegisterKnowledgeServiceTools(reg, nil, func(string) []string { return nil }, func(proto.Message) error { return nil })
	spec, ok := reg.Spec(knowledgev1mcp.KnowledgeTestSearchToolName)
	require.True(t, ok)
	assert.True(t, spec.Mutation)
	assert.False(t, spec.Destructive)
}
