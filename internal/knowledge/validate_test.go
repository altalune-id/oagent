package knowledge_test

import (
	"testing"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"buf.build/go/protovalidate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	knowledgev1 "altalune.id/oagent/gen/go/knowledge/v1"
	"altalune.id/oagent/internal/knowledge"
)

func TestBeginUploadRequest_FilenameRules(t *testing.T) {
	v, err := protovalidate.New()
	require.NoError(t, err)
	req := func(name string) *knowledgev1.BeginUploadRequest {
		return &knowledgev1.BeginUploadRequest{SourceId: "abcdefghjkmnpqrstuvwx", Filename: name, ContentType: "application/pdf", SizeBytes: 10}
	}
	require.NoError(t, v.Validate(req("Handbook 2026 (final).pdf")))
	require.NoError(t, v.Validate(req("Café menu.pdf")))
	for name, filename := range map[string]string{
		"spaces":     "   ",
		"tab":        "\t",
		"nbsp":       " ",
		"newline":    "a\nb.pdf",
		"nul":        "a\x00.pdf",
		"escape":     "a\x1b[31m.pdf",
		"delete":     "a\x7f.pdf",
		"c1 control": "a\u0085.pdf",
	} {
		assert.Error(t, v.Validate(req(filename)), name)
	}
}

// protovalidate cannot read Go, so this pins BeginUpload's size and type rules to the Go upload purpose.
func TestBeginUploadRequest_RulesMatchTheUploadPurpose(t *testing.T) {
	fields := (&knowledgev1.BeginUploadRequest{}).ProtoReflect().Descriptor().Fields()
	rules := func(name string) *validate.FieldRules {
		r, ok := proto.GetExtension(fields.ByName(protoreflect.Name(name)).Options(), validate.E_Field).(*validate.FieldRules)
		require.True(t, ok, name)
		return r
	}
	assert.Equal(t, knowledge.UploadPurpose.MaxBytes, rules("size_bytes").GetInt64().GetLte())
	assert.Equal(t, knowledge.UploadPurpose.Types, rules("content_type").GetString().GetIn())
}
