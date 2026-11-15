package webtool

import (
	"testing"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	mcpv1 "altalune.id/oagent/gen/go/mcp/v1"
	webtoolv1 "altalune.id/oagent/gen/go/webtool/v1"
	"altalune.id/oagent/internal/kernel/webpolicy"
)

func TestHeaderName_ProtoPatternMatches(t *testing.T) {
	field := webtoolv1.File_webtool_v1_webtool_proto.Messages().ByName("SetHTTPSecretRequest").Fields().ByName("header")
	require.NotNil(t, field)
	rules, ok := proto.GetExtension(field.Options(), validate.E_Field).(*validate.FieldRules)
	require.True(t, ok)
	assert.Equal(t, webpolicy.HeaderNamePattern, rules.GetString().GetPattern())
}

func TestWebToolService_HasNoMCPTools(t *testing.T) {
	methods := webtoolv1.File_webtool_v1_webtool_proto.Services().ByName("WebToolService").Methods()
	require.Positive(t, methods.Len())
	for i := range methods.Len() {
		m := methods.Get(i)
		assert.False(t, proto.HasExtension(m.Options(), mcpv1.E_Tool), "%s takes or guards a key and must have no MCP tool", m.Name())
	}
}
