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
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/knowledge"
	"altalune.id/oagent/internal/testutil/fakes"
)

const unknownID = "unknownid000000000001"

type reachFixture struct {
	c        cp
	src      *knowledge.Source
	doc      *knowledge.Document
	otherPrv string
}

func newReachFixture(t *testing.T) reachFixture {
	t.Helper()
	e := newEnv(t)
	svc := e.svc.WithRetrievers(map[knowledge.Backend]knowledge.Retriever{knowledge.BackendPgvector: fakeRetriever{}})
	c := newCP(t, e, svc)
	src := e.uploadSource(t, "Policies")
	doc, _ := e.begin(t, src)
	return reachFixture{c: c, src: src, doc: doc, otherPrv: c.refs.Provider(uuid.New(), e.otherPrj)}
}

// SECURITY: err must be exactly the error an unknown id gets, so a caller learns nothing about the row.
func sameAsUnknown(t *testing.T, unknown, err error, code apperror.Code, what string) {
	t.Helper()
	require.Error(t, err, what)
	assert.Equal(t, code, codeOf(err), "%s: %v", what, err)
	assert.Equal(t, unknown.Error(), err.Error(), what)
}

func TestKnowledgeReach_IDRPCsAnswerLikeUnknown(t *testing.T) {
	f := newReachFixture(t)
	org, prj := f.src.OrgID, f.src.ProjectID
	for name, p := range map[string]session.Principal{
		"member of another org":  {Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: uuid.New(), Scopes: []string{"*"}},
		"key of another project": {Source: session.SourceAPIKey, ActiveOrgID: org, ProjectIDs: []uuid.UUID{uuid.New()}},
		"key of another source":  {Source: session.SourceAPIKey, ActiveOrgID: org, ProjectIDs: []uuid.UUID{prj}, ResourceIDs: []uuid.UUID{uuid.New()}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := session.PrincipalInto(context.Background(), p)
			c := f.c.client
			for _, call := range []struct {
				name string
				run  func(id string) error
			}{
				{"GetKnowledgeSource", func(id string) error {
					_, err := c.GetKnowledgeSource(ctx, connect.NewRequest(&knowledgev1.GetKnowledgeSourceRequest{Id: id}))
					return err
				}},
				{"RefreshKnowledgeSource", func(id string) error {
					_, err := c.RefreshKnowledgeSource(ctx, connect.NewRequest(&knowledgev1.RefreshKnowledgeSourceRequest{Id: id}))
					return err
				}},
				{"DeleteKnowledgeSource", func(id string) error {
					_, err := c.DeleteKnowledgeSource(ctx, connect.NewRequest(&knowledgev1.DeleteKnowledgeSourceRequest{Id: id}))
					return err
				}},
				{"ListKnowledgeDocuments", func(id string) error {
					_, err := c.ListKnowledgeDocuments(ctx, connect.NewRequest(&knowledgev1.ListKnowledgeDocumentsRequest{SourceId: id}))
					return err
				}},
				{"BeginUpload", func(id string) error {
					_, err := c.BeginUpload(ctx, connect.NewRequest(&knowledgev1.BeginUploadRequest{
						SourceId: id, Filename: "a.pdf", ContentType: knowledge.TypePDF, SizeBytes: int64(len(pdf)),
					}))
					return err
				}},
				{"TestSearch", func(id string) error {
					_, err := c.TestSearch(ctx, connect.NewRequest(&knowledgev1.TestSearchRequest{SourceId: id, Query: "refunds"}))
					return err
				}},
			} {
				sameAsUnknown(t, call.run(unknownID), call.run(f.src.PublicID), apperror.CodeKnowledgeSourceNotFound, call.name)
			}
			for _, call := range []struct {
				name string
				run  func(id string) error
			}{
				{"GetKnowledgeDocument", func(id string) error {
					_, err := c.GetKnowledgeDocument(ctx, connect.NewRequest(&knowledgev1.GetKnowledgeDocumentRequest{Id: id}))
					return err
				}},
				{"CompleteUpload", func(id string) error {
					_, err := c.CompleteUpload(ctx, connect.NewRequest(&knowledgev1.CompleteUploadRequest{DocumentId: id}))
					return err
				}},
				{"DeleteKnowledgeDocument", func(id string) error {
					_, err := c.DeleteKnowledgeDocument(ctx, connect.NewRequest(&knowledgev1.DeleteKnowledgeDocumentRequest{Id: id}))
					return err
				}},
			} {
				sameAsUnknown(t, call.run(unknownID), call.run(f.doc.PublicID), apperror.CodeKnowledgeDocumentNotFound, call.name)
			}
		})
	}

	got, err := f.c.client.GetKnowledgeDocument(fakes.OperatorContext(context.Background()),
		connect.NewRequest(&knowledgev1.GetKnowledgeDocumentRequest{Id: f.doc.PublicID}))
	require.NoError(t, err, "nothing was deleted")
	assert.Equal(t, f.src.PublicID, got.Msg.GetDocument().GetSourceId())
}

func TestKnowledgeReach_ProjectRPCsAnswerLikeUnknown(t *testing.T) {
	f := newReachFixture(t)
	org := f.src.OrgID
	for name, p := range map[string]session.Principal{
		"member of another org":  {Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: uuid.New(), Scopes: []string{"*"}},
		"key of another project": {Source: session.SourceAPIKey, ActiveOrgID: org, ProjectIDs: []uuid.UUID{uuid.New()}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := session.PrincipalInto(context.Background(), p)
			list := func(project string) error {
				_, err := f.c.client.ListKnowledgeSources(ctx, connect.NewRequest(&knowledgev1.ListKnowledgeSourcesRequest{ProjectId: project}))
				return err
			}
			sameAsUnknown(t, list(unknownID), list(f.c.prjPub), apperror.CodeKnowledgeSourceNotFound, "ListKnowledgeSources")
			set := func(project string) error {
				_, err := f.c.client.SetEmbeddingModel(ctx, connect.NewRequest(&knowledgev1.SetEmbeddingModelRequest{
					ProjectId: project, ProviderId: f.c.prvPub, Model: "text-embedding-3-small",
				}))
				return err
			}
			sameAsUnknown(t, set(unknownID), set(f.c.prjPub), apperror.CodeKnowledgeSourceNotFound, "SetEmbeddingModel")
		})
	}
}

func TestKnowledgeReach_ListFiltersToReach(t *testing.T) {
	f := newReachFixture(t)
	org, prj := f.src.OrgID, f.src.ProjectID
	other, err := f.c.client.CreateUploadSource(fakes.OperatorContext(context.Background()), connect.NewRequest(&knowledgev1.CreateUploadSourceRequest{
		OrgId: f.c.orgPub, ProjectId: f.c.prjPub, Name: "Other",
	}))
	require.NoError(t, err)
	list := func(p session.Principal) []string {
		resp, err := f.c.client.ListKnowledgeSources(session.PrincipalInto(context.Background(), p),
			connect.NewRequest(&knowledgev1.ListKnowledgeSourcesRequest{ProjectId: f.c.prjPub}))
		require.NoError(t, err)
		var out []string
		for _, s := range resp.Msg.GetSources() {
			out = append(out, s.GetId())
		}
		return out
	}
	assert.ElementsMatch(t, []string{f.src.PublicID, other.Msg.GetSource().GetId()},
		list(session.Principal{Source: session.SourceAPIKey, ActiveOrgID: org, ProjectIDs: []uuid.UUID{prj}}))
	assert.Equal(t, []string{f.src.PublicID},
		list(session.Principal{Source: session.SourceAPIKey, ActiveOrgID: org, ProjectIDs: []uuid.UUID{prj}, ResourceIDs: []uuid.UUID{f.src.ID}}))
	assert.Empty(t, list(session.Principal{Source: session.SourceAPIKey, ActiveOrgID: org, ProjectIDs: []uuid.UUID{prj}, ResourceIDs: []uuid.UUID{uuid.New()}}))
}

// SECURITY: another project's provider answers exactly like a provider id that does not exist.
func TestKnowledgeReach_EmbeddingModelOfAnotherProject(t *testing.T) {
	f := newReachFixture(t)
	ctx := fakes.OperatorContext(context.Background())
	set := func(provider string) error {
		_, err := f.c.client.SetEmbeddingModel(ctx, connect.NewRequest(&knowledgev1.SetEmbeddingModelRequest{
			ProjectId: f.c.prjPub, ProviderId: provider, Model: "text-embedding-3-small",
		}))
		return err
	}
	sameAsUnknown(t, set(unknownID), set(f.otherPrv), apperror.CodeKnowledgeEmbeddingModelInvalid, "SetEmbeddingModel")
}
