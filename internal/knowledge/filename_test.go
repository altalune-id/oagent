package knowledge_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/knowledge"
)

func TestNewUploadDocument_RefusesControlCharacters(t *testing.T) {
	src, err := knowledge.NewSource(uuid.New(), uuid.New(), knowledge.SourceInput{Kind: knowledge.KindUpload, Backend: knowledge.BackendPgvector, Name: "Policies"}, pgvectorOn, now)
	require.NoError(t, err)
	doc, err := knowledge.NewUploadDocument(src, " Café menu (2026).pdf ", uuid.New(), now)
	require.NoError(t, err)
	assert.Equal(t, "Café menu (2026).pdf", doc.Title)
	for name, filename := range map[string]string{
		"newline": "a\nb.pdf", "nul": "a\x00.pdf", "escape": "a\x1b[31m.pdf", "delete": "a\x7f.pdf", "c1 control": "a\u0085.pdf",
	} {
		_, err := knowledge.NewUploadDocument(src, filename, uuid.New(), now)
		assert.Equal(t, apperror.CodeValidation, codeOf(err), "%s: %v", name, err)
	}
}
