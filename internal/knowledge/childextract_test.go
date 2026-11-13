package knowledge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/knowledge/extract"
)

// SECURITY: every uploaded format is parsed in the limited child process, never in the server.
func TestChildExtractor_ParsesEveryFormatInTheChild(t *testing.T) {
	broken := ChildExtractor{Child: extract.Child{Path: "/nonexistent/oagent"}}
	for kind, data := range map[extract.Kind][]byte{
		extract.KindPDF: readDoc(t, "text_3pages.pdf"), extract.KindDOCX: readDoc(t, "headings.docx"),
		extract.KindMarkdown: []byte("# Refunds\n\nWe refund."), extract.KindText: []byte("We refund."),
	} {
		_, err := broken.Extract(context.Background(), kind, data)
		assert.True(t, extract.IsUnreadableError(err), "%s went around the child: %v", kind, err)
	}
}
