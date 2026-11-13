//go:build integration

package knowledge_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"

	"altalune.id/oagent/apperror"
	knowledgev1 "altalune.id/oagent/gen/go/knowledge/v1"
	"altalune.id/oagent/gen/go/knowledge/v1/knowledgev1connect"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/testutil/fakes"
)

type cp struct {
	client                 knowledgev1connect.KnowledgeServiceHandler
	orgPub, prjPub, prvPub string
	otherPrjPub            string
	refs                   *fakes.PublicIDs
}

func newCP(t *testing.T, e *env, svc *knowledge.Service) cp {
	t.Helper()
	refs := fakes.NewPublicIDs()
	c := cp{orgPub: refs.Org(e.org), prjPub: refs.Project(e.prj, e.org), refs: refs}
	c.otherPrjPub = refs.Project(e.otherPrj, e.otherOrg)
	c.prvPub = refs.Provider(e.provider(t), e.prj)
	c.client = knowledge.NewControlPlane(svc, refs)
	return c
}

func TestKnowledgeService_SourcesUploadsAndModel(t *testing.T) {
	e := newEnv(t)
	svc := e.withEmbedding(1536, nil, fakeModels{"text-embedding-3-small": embeddingModels["text-embedding-3-small"]})
	c := newCP(t, e, svc)
	ctx := fakes.OperatorContext(context.Background())

	up, err := c.client.CreateUploadSource(ctx, connect.NewRequest(&knowledgev1.CreateUploadSourceRequest{OrgId: c.orgPub, ProjectId: c.prjPub, Name: "Policies"}))
	require.NoError(t, err)
	src := up.Msg.GetSource()
	assert.Len(t, src.GetId(), 21)
	assert.Equal(t, knowledgev1.KnowledgeBackend_KNOWLEDGE_BACKEND_PGVECTOR, src.GetBackend(), "unset picks the only enabled backend")
	assert.Equal(t, c.prjPub, src.GetProjectId())

	begun, err := c.client.BeginUpload(ctx, connect.NewRequest(&knowledgev1.BeginUploadRequest{
		SourceId: src.GetId(), Filename: "a.pdf", ContentType: knowledge.TypePDF, SizeBytes: int64(len(pdf)),
	}))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"Content-Type": knowledge.TypePDF}, begun.Msg.GetUpload().GetHeaders())
	doc := begun.Msg.GetDocument()
	assert.Equal(t, knowledgev1.KnowledgeDocumentStatus_KNOWLEDGE_DOCUMENT_STATUS_UPLOADING, doc.GetStatus())

	_, err = c.client.CompleteUpload(ctx, connect.NewRequest(&knowledgev1.CompleteUploadRequest{DocumentId: doc.GetId()}))
	assert.Equal(t, apperror.CodeKnowledgeUploadRejected, codeOf(err), "nothing was uploaded yet")

	set, err := c.client.SetEmbeddingModel(ctx, connect.NewRequest(&knowledgev1.SetEmbeddingModelRequest{ProjectId: c.prjPub, ProviderId: c.prvPub, Model: "text-embedding-3-small"}))
	require.NoError(t, err)
	assert.Equal(t, int32(1536), set.Msg.GetEmbeddingModel().GetDims())

	list, err := c.client.ListKnowledgeSources(ctx, connect.NewRequest(&knowledgev1.ListKnowledgeSourcesRequest{ProjectId: c.prjPub}))
	require.NoError(t, err)
	assert.True(t, list.Msg.GetStorageConfigured())
	assert.Equal(t, []knowledgev1.KnowledgeBackend{knowledgev1.KnowledgeBackend_KNOWLEDGE_BACKEND_PGVECTOR}, list.Msg.GetEnabledBackends())
	assert.Equal(t, c.prvPub, list.Msg.GetEmbeddingModel().GetProviderId())
	require.Len(t, list.Msg.GetSources(), 1)
	assert.Equal(t, int32(1), list.Msg.GetSources()[0].GetDocumentCount())

	docs, err := c.client.ListKnowledgeDocuments(ctx, connect.NewRequest(&knowledgev1.ListKnowledgeDocumentsRequest{SourceId: src.GetId()}))
	require.NoError(t, err)
	require.Len(t, docs.Msg.GetDocuments(), 1)
	got, err := c.client.GetKnowledgeDocument(ctx, connect.NewRequest(&knowledgev1.GetKnowledgeDocumentRequest{Id: doc.GetId()}))
	require.NoError(t, err)
	assert.Equal(t, src.GetId(), got.Msg.GetDocument().GetSourceId())
	_, err = c.client.DeleteKnowledgeDocument(ctx, connect.NewRequest(&knowledgev1.DeleteKnowledgeDocumentRequest{Id: doc.GetId()}))
	require.NoError(t, err)

	gotSrc, err := c.client.GetKnowledgeSource(ctx, connect.NewRequest(&knowledgev1.GetKnowledgeSourceRequest{Id: src.GetId()}))
	require.NoError(t, err)
	assert.Equal(t, "Policies", gotSrc.Msg.GetSource().GetName())
	_, err = c.client.DeleteKnowledgeSource(ctx, connect.NewRequest(&knowledgev1.DeleteKnowledgeSourceRequest{Id: src.GetId()}))
	require.NoError(t, err)
	_, err = c.client.GetKnowledgeSource(ctx, connect.NewRequest(&knowledgev1.GetKnowledgeSourceRequest{Id: src.GetId()}))
	assert.Equal(t, apperror.CodeKnowledgeSourceNotFound, codeOf(err))
}

func TestKnowledgeService_URLSourcesAndRefusals(t *testing.T) {
	e := newEnv(t)
	c := newCP(t, e, e.svc)
	ctx := fakes.OperatorContext(context.Background())

	made, err := c.client.CreateURLSource(ctx, connect.NewRequest(&knowledgev1.CreateURLSourceRequest{
		OrgId: c.orgPub, ProjectId: c.prjPub, Name: "Help", SeedUrl: "https://help.example.com",
	}))
	require.NoError(t, err)
	assert.Equal(t, knowledgev1.KnowledgeSourceStatus_KNOWLEDGE_SOURCE_STATUS_CRAWLING, made.Msg.GetSource().GetStatus())
	refreshed, err := c.client.RefreshKnowledgeSource(ctx, connect.NewRequest(&knowledgev1.RefreshKnowledgeSourceRequest{Id: made.Msg.GetSource().GetId()}))
	require.NoError(t, err)
	assert.Equal(t, knowledgev1.KnowledgeSourceKind_KNOWLEDGE_SOURCE_KIND_URL, refreshed.Msg.GetSource().GetKind())

	_, err = c.client.CreateURLSource(ctx, connect.NewRequest(&knowledgev1.CreateURLSourceRequest{
		OrgId: c.orgPub, ProjectId: c.prjPub, Name: "Help", SeedUrl: "https://other.example.com",
	}))
	assert.Equal(t, apperror.CodeKnowledgeSourceAlreadyExists, codeOf(err))

	_, err = c.client.CreateUploadSource(ctx, connect.NewRequest(&knowledgev1.CreateUploadSourceRequest{OrgId: c.orgPub, ProjectId: c.otherPrjPub, Name: "X"}))
	assert.Equal(t, apperror.CodeKnowledgeSourceNotFound, codeOf(err), "a project of another org answers like an unknown one")

	_, err = c.client.TestSearch(ctx, connect.NewRequest(&knowledgev1.TestSearchRequest{SourceId: made.Msg.GetSource().GetId(), Query: "refunds"}))
	assert.Equal(t, apperror.CodeKnowledgeSourceInvalid, codeOf(err), "no retriever for the backend")

	_, err = c.client.SetEmbeddingModel(ctx, connect.NewRequest(&knowledgev1.SetEmbeddingModelRequest{ProjectId: c.prjPub, ProviderId: "unknownprovider000001", Model: "m"}))
	assert.Equal(t, apperror.CodeKnowledgeEmbeddingModelInvalid, codeOf(err))
	for _, id := range []string{"unknownsource00000001", uuid.NewString()} {
		_, err = c.client.GetKnowledgeSource(ctx, connect.NewRequest(&knowledgev1.GetKnowledgeSourceRequest{Id: id}))
		assert.Error(t, err, id)
		_, err = c.client.GetKnowledgeDocument(ctx, connect.NewRequest(&knowledgev1.GetKnowledgeDocumentRequest{Id: id}))
		assert.Error(t, err, id)
	}
}

// SECURITY: file bytes go from the browser straight to storage; no request message of the service carries bytes.
func TestNoRPCAcceptsFileBytes(t *testing.T) {
	svcs := knowledgev1.File_knowledge_v1_knowledge_proto.Services()
	for i := range svcs.Len() {
		methods := svcs.Get(i).Methods()
		for j := range methods.Len() {
			assertNoBytes(t, methods.Get(j).Input(), map[protoreflect.FullName]bool{})
		}
	}
}

func assertNoBytes(t *testing.T, m protoreflect.MessageDescriptor, seen map[protoreflect.FullName]bool) {
	t.Helper()
	if seen[m.FullName()] {
		return
	}
	seen[m.FullName()] = true
	fields := m.Fields()
	for i := range fields.Len() {
		f := fields.Get(i)
		assert.NotEqual(t, protoreflect.BytesKind, f.Kind(), "%s.%s carries bytes", m.FullName(), f.Name())
		if f.Message() != nil {
			assertNoBytes(t, f.Message(), seen)
		}
	}
}

type fakeRetriever struct{ hits []knowledge.Hit }

func (f fakeRetriever) Search(_ context.Context, q knowledge.Query) ([]knowledge.Hit, knowledge.Usage, error) {
	if len(q.Sources) != 1 || q.K != knowledge.RecommendedTopK {
		return nil, knowledge.Usage{}, apperror.New(apperror.CodeUnexpected, "bad query")
	}
	return f.hits, knowledge.Usage{RetrieveCalls: 1}, nil
}

func TestKnowledgeService_TestSearchUsesTheSourceBackend(t *testing.T) {
	e := newEnv(t)
	svc := e.svc.WithRetrievers(map[knowledge.Backend]knowledge.Retriever{
		knowledge.BackendPgvector: fakeRetriever{hits: []knowledge.Hit{{Text: "We refund within 30 days.", Title: "FAQ", Heading: "Refunds", Page: 2, Score: 0.8}}},
	})
	c := newCP(t, e, svc)
	ctx := fakes.OperatorContext(context.Background())
	made, err := c.client.CreateUploadSource(ctx, connect.NewRequest(&knowledgev1.CreateUploadSourceRequest{
		OrgId: c.orgPub, ProjectId: c.prjPub, Name: "FAQ", Backend: knowledgev1.KnowledgeBackend_KNOWLEDGE_BACKEND_PGVECTOR,
	}))
	require.NoError(t, err)

	got, err := c.client.TestSearch(ctx, connect.NewRequest(&knowledgev1.TestSearchRequest{SourceId: made.Msg.GetSource().GetId(), Query: "refunds"}))
	require.NoError(t, err)
	require.Len(t, got.Msg.GetHits(), 1)
	assert.Equal(t, "Refunds", got.Msg.GetHits()[0].GetHeading())
	assert.Equal(t, int32(2), got.Msg.GetHits()[0].GetPage())
}
