//go:build integration

package knowledge_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	knowledgev1 "altalune.id/oagent/gen/go/knowledge/v1"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/testutil/fakes"
)

func TestKnowledgeService_DocumentsShowSizeAndPassages(t *testing.T) {
	e := newEnv(t)
	svc, _ := e.ingesting(t, 1536)
	src := e.uploadSource(t, "Policies")
	doc := e.uploaded(t, src, faq)
	waiting, _ := e.begin(t, src)
	e.ingestRound(t, svc)
	chunks, _ := e.chunkRows(t, doc.ID)
	require.Positive(t, chunks)
	c := newCP(t, e, svc)
	ctx := fakes.OperatorContext(context.Background())

	list, err := c.client.ListKnowledgeDocuments(ctx, connect.NewRequest(&knowledgev1.ListKnowledgeDocumentsRequest{SourceId: src.PublicID}))
	require.NoError(t, err)
	got := map[string]*knowledgev1.KnowledgeDocument{}
	for _, d := range list.Msg.GetDocuments() {
		got[d.GetId()] = d
	}
	require.Len(t, got, 2)
	assert.Equal(t, int64(len(faq)), got[doc.PublicID].GetSizeBytes())
	assert.Equal(t, int32(chunks), got[doc.PublicID].GetChunkCount()) //nolint:gosec // G115: a handful of chunks.
	assert.Equal(t, int64(len(pdf)), got[waiting.PublicID].GetSizeBytes(), "the declared size of a file still uploading")
	assert.Zero(t, got[waiting.PublicID].GetChunkCount())

	one, err := c.client.GetKnowledgeDocument(ctx, connect.NewRequest(&knowledgev1.GetKnowledgeDocumentRequest{Id: doc.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, int64(len(faq)), one.Msg.GetDocument().GetSizeBytes())
	assert.Equal(t, int32(chunks), one.Msg.GetDocument().GetChunkCount()) //nolint:gosec // G115: a handful of chunks.
}

func TestKnowledgeService_GetSourceCountsOnlyItsOwnDocuments(t *testing.T) {
	e := newEnv(t)
	full, empty := e.uploadSource(t, "Policies"), e.uploadSource(t, "Empty")
	e.begin(t, full)
	e.begin(t, full)
	used := []knowledge.UsedBy{{AgentPublicID: "agentpublicid00000001", AgentName: "Support"}}
	c := newCP(t, e, e.svc.WithBindings(fakeBindings{used: map[uuid.UUID][]knowledge.UsedBy{full.ID: used}}))
	ctx := fakes.OperatorContext(context.Background())
	get := func(id string) *knowledgev1.KnowledgeSource {
		resp, err := c.client.GetKnowledgeSource(ctx, connect.NewRequest(&knowledgev1.GetKnowledgeSourceRequest{Id: id}))
		require.NoError(t, err)
		return resp.Msg.GetSource()
	}
	f := get(full.PublicID)
	assert.Equal(t, int32(2), f.GetDocumentCount())
	require.Len(t, f.GetUsedBy(), 1)
	assert.Equal(t, "Support", f.GetUsedBy()[0].GetAgentName())
	g := get(empty.PublicID)
	assert.Zero(t, g.GetDocumentCount())
	assert.Empty(t, g.GetUsedBy())
}

func TestKnowledgeService_UploadToAWebsiteSourceSaysKind(t *testing.T) {
	e := newEnv(t)
	src := e.urlSource(t, e.svc)
	c := newCP(t, e, e.svc)
	_, err := c.client.BeginUpload(fakes.OperatorContext(context.Background()), connect.NewRequest(&knowledgev1.BeginUploadRequest{
		SourceId: src.PublicID, Filename: "a.pdf", ContentType: knowledge.TypePDF, SizeBytes: int64(len(pdf)),
	}))
	require.Equal(t, apperror.CodeKnowledgeSourceInvalid, codeOf(err), "%v", err)
	ae, _ := apperror.AsAppError(err)
	assert.Equal(t, "kind", ae.Meta()["reason"])
}

func TestKnowledgeService_RefreshAnUploadSourceSaysKind(t *testing.T) {
	e := newEnv(t)
	src := e.uploadSource(t, "Policies")
	c := newCP(t, e, e.svc)
	_, err := c.client.RefreshKnowledgeSource(fakes.OperatorContext(context.Background()),
		connect.NewRequest(&knowledgev1.RefreshKnowledgeSourceRequest{Id: src.PublicID}))
	require.Equal(t, apperror.CodeKnowledgeSourceInvalid, codeOf(err), "%v", err)
	ae, _ := apperror.AsAppError(err)
	assert.Equal(t, "kind", ae.Meta()["reason"])
}
