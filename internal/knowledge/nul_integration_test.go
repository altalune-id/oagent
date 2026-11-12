//go:build integration

package knowledge_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/knowledge"
)

// Postgres text cannot hold U+0000; a NUL past the sniffed prefix must not fail the document.
func TestIngest_TextWithANULIngestsCleanly(t *testing.T) {
	e := newEnv(t)
	svc, _ := e.ingesting(t, 1536)
	body := "Refund policy ok\x00 we refund within 30 days."
	doc, _, err := e.svc.BeginUpload(e.ctx, e.uploadSource(t, "Policies").ID,
		knowledge.UploadInput{Filename: "policy.txt", ContentType: knowledge.TypeText, Size: int64(len(body))})
	require.NoError(t, err)
	e.driver.Put(e.fileKey(t, *doc.FileID), knowledge.TypeText, []byte(body))
	_, err = e.svc.CompleteUpload(e.ctx, doc.ID)
	require.NoError(t, err)

	runRound(t, knowledge.NewIngestWorker(svc, oneTenant{e.ctx}, slog.New(slog.DiscardHandler)))

	got, err := svc.GetDocument(e.ctx, doc.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.DocumentReady, got.Status, "failure: %+v", got.Failure)
	n, _ := e.chunkRows(t, doc.ID)
	assert.Positive(t, n)
}
