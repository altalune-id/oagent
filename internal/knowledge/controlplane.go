package knowledge

import (
	"context"
	"slices"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	knowledgev1 "altalune.id/oagent/gen/go/knowledge/v1"
	"altalune.id/oagent/gen/go/knowledge/v1/knowledgev1connect"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/scope"
)

// Refs turns org, project and model provider public ids into ids and back for the handler; ok is false for an unknown id.
type Refs interface {
	OrgID(ctx context.Context, publicID string) (id uuid.UUID, ok bool, err error)
	OrgPublicID(ctx context.Context, id uuid.UUID) (string, error)
	ProjectID(ctx context.Context, publicID string) (id, orgID uuid.UUID, ok bool, err error)
	ProjectPublicID(ctx context.Context, id uuid.UUID) (string, error)
	ProviderID(ctx context.Context, publicID string) (id, projectID uuid.UUID, ok bool, err error)
	ProviderPublicID(ctx context.Context, id uuid.UUID) (publicID string, ok bool, err error)
}

// KnowledgeService implements knowledge.v1.KnowledgeService; every id it takes and returns is a public id.
type KnowledgeService struct {
	svc  *Service
	refs Refs
}

// NewControlPlane binds the handler to the knowledge service and the public id lookups.
func NewControlPlane(svc *Service, refs Refs) *KnowledgeService {
	return &KnowledgeService{svc: svc, refs: refs}
}

var _ knowledgev1connect.KnowledgeServiceHandler = (*KnowledgeService)(nil)

// CreateURLSource creates a website source in a project; its first crawl starts at once.
func (s *KnowledgeService) CreateURLSource(
	ctx context.Context, req *connect.Request[knowledgev1.CreateURLSourceRequest],
) (*connect.Response[knowledgev1.CreateURLSourceResponse], error) {
	m := req.Msg
	out, err := s.create(ctx, m.GetOrgId(), m.GetProjectId(), SourceInput{
		Kind: KindURL, Backend: s.backendOf(m.GetBackend()), Name: m.GetName(), SeedURL: m.GetSeedUrl(),
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&knowledgev1.CreateURLSourceResponse{Source: out}), nil
}

// CreateUploadSource creates a source that takes uploaded files.
func (s *KnowledgeService) CreateUploadSource(
	ctx context.Context, req *connect.Request[knowledgev1.CreateUploadSourceRequest],
) (*connect.Response[knowledgev1.CreateUploadSourceResponse], error) {
	m := req.Msg
	in := SourceInput{Kind: KindUpload, Backend: s.backendOf(m.GetBackend()), Name: m.GetName()}
	out, err := s.create(ctx, m.GetOrgId(), m.GetProjectId(), in)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&knowledgev1.CreateUploadSourceResponse{Source: out}), nil
}

func (s *KnowledgeService) create(ctx context.Context, orgPub, projectPub string, in SourceInput) (*knowledgev1.KnowledgeSource, error) {
	orgID, ok, err := s.refs.OrgID(ctx, orgPub)
	if err != nil {
		return nil, err
	}
	ctx, projectID, err := s.project(ctx, projectPub, authz.ReachWholeProject)
	if err != nil {
		return nil, err
	}
	// SECURITY: a project of another org answers the same not-found as an unknown one.
	if tc, terr := tenant.From(ctx); !ok || terr != nil || tc.OrgID != orgID {
		return nil, &SourceNotFoundError{}
	}
	src, err := s.svc.CreateSource(ctx, projectID, in)
	if err != nil {
		return nil, err
	}
	return sourceToProto(SourceView{Source: src}, orgPub, projectPub, nil), nil
}

func (s *KnowledgeService) backendOf(b knowledgev1.KnowledgeBackend) Backend {
	if b == knowledgev1.KnowledgeBackend_KNOWLEDGE_BACKEND_PGVECTOR {
		return BackendPgvector
	}
	if enabled := s.svc.Backends(); len(enabled) == 1 {
		return enabled[0]
	}
	return ""
}

// ListKnowledgeSources lists a project's sources with their counts and users, the embedding model and what the server supports.
func (s *KnowledgeService) ListKnowledgeSources(
	ctx context.Context, req *connect.Request[knowledgev1.ListKnowledgeSourcesRequest],
) (*connect.Response[knowledgev1.ListKnowledgeSourcesResponse], error) {
	ctx, projectID, err := s.project(ctx, req.Msg.GetProjectId(), authz.ReachProject)
	if err != nil {
		return nil, err
	}
	views, err := s.svc.ListSources(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := &knowledgev1.ListKnowledgeSourcesResponse{
		StorageConfigured: s.svc.StorageConfigured(),
		MaxUploadBytes:    UploadPurpose.MaxBytes, AllowedContentTypes: slices.Clone(UploadPurpose.Types),
		RetrievalDefaults: retrievalDefaults(),
	}
	for _, b := range s.svc.Backends() {
		out.EnabledBackends = append(out.EnabledBackends, backendToProto(b))
	}
	orgPub, agents := "", shownAgents(ctx)
	for _, v := range views {
		if !authz.ReachResource(ctx, v.Source.OrgID, v.Source.ProjectID, v.Source.ID) {
			continue
		}
		if orgPub == "" {
			if orgPub, err = s.refs.OrgPublicID(ctx, v.Source.OrgID); err != nil {
				return nil, err
			}
		}
		out.Sources = append(out.Sources, sourceToProto(v, orgPub, req.Msg.GetProjectId(), agents))
	}
	st, err := s.svc.Settings(ctx, projectID)
	switch {
	case IsEmbeddingModelNotSetError(err):
	case err != nil:
		return nil, err
	default:
		if out.EmbeddingModel, err = s.settingsToProto(ctx, st); err != nil {
			return nil, err
		}
	}
	return connect.NewResponse(out), nil
}

// GetKnowledgeSource loads a source by id with its document counts.
func (s *KnowledgeService) GetKnowledgeSource(
	ctx context.Context, req *connect.Request[knowledgev1.GetKnowledgeSourceRequest],
) (*connect.Response[knowledgev1.GetKnowledgeSourceResponse], error) {
	ctx, src, err := s.reachableSource(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	out, err := s.sourceOut(ctx, src)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&knowledgev1.GetKnowledgeSourceResponse{Source: out}), nil
}

// RefreshKnowledgeSource queues a website source for a new crawl.
func (s *KnowledgeService) RefreshKnowledgeSource(
	ctx context.Context, req *connect.Request[knowledgev1.RefreshKnowledgeSourceRequest],
) (*connect.Response[knowledgev1.RefreshKnowledgeSourceResponse], error) {
	ctx, src, err := s.reachableSource(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if src, err = s.svc.Refresh(ctx, src.ID); err != nil {
		return nil, err
	}
	out, err := s.sourceOut(ctx, src)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&knowledgev1.RefreshKnowledgeSourceResponse{Source: out}), nil
}

// DeleteKnowledgeSource deletes a source no published agent version searches.
func (s *KnowledgeService) DeleteKnowledgeSource(
	ctx context.Context, req *connect.Request[knowledgev1.DeleteKnowledgeSourceRequest],
) (*connect.Response[knowledgev1.DeleteKnowledgeSourceResponse], error) {
	ctx, src, err := s.reachableSource(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.DeleteSource(ctx, src.ID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&knowledgev1.DeleteKnowledgeSourceResponse{}), nil
}

// BeginUpload records an uploading document and signs the browser's PUT; the file bytes never pass through here.
func (s *KnowledgeService) BeginUpload(
	ctx context.Context, req *connect.Request[knowledgev1.BeginUploadRequest],
) (*connect.Response[knowledgev1.BeginUploadResponse], error) {
	m := req.Msg
	ctx, src, err := s.reachableSource(ctx, m.GetSourceId())
	if err != nil {
		return nil, err
	}
	in := UploadInput{Filename: m.GetFilename(), ContentType: m.GetContentType(), Size: m.GetSizeBytes()}
	doc, up, err := s.svc.BeginUpload(ctx, src.ID, in)
	if err != nil {
		return nil, err
	}
	out, err := s.documentOut(ctx, doc, src.PublicID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&knowledgev1.BeginUploadResponse{
		Document: out,
		Upload:   &knowledgev1.SignedUpload{Url: up.URL, Method: up.Method, Headers: up.Headers, ExpiresAt: timestamppb.New(up.ExpiresAt)},
	}), nil
}

// CompleteUpload verifies a document's file and queues it for ingestion.
func (s *KnowledgeService) CompleteUpload(
	ctx context.Context, req *connect.Request[knowledgev1.CompleteUploadRequest],
) (*connect.Response[knowledgev1.CompleteUploadResponse], error) {
	ctx, doc, src, err := s.reachableDocument(ctx, req.Msg.GetDocumentId())
	if err != nil {
		return nil, err
	}
	if doc, err = s.svc.CompleteUpload(ctx, doc.ID); err != nil {
		return nil, err
	}
	out, err := s.documentOut(ctx, doc, src.PublicID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&knowledgev1.CompleteUploadResponse{Document: out}), nil
}

// ListKnowledgeDocuments lists a source's documents, oldest first.
func (s *KnowledgeService) ListKnowledgeDocuments(
	ctx context.Context, req *connect.Request[knowledgev1.ListKnowledgeDocumentsRequest],
) (*connect.Response[knowledgev1.ListKnowledgeDocumentsResponse], error) {
	ctx, src, err := s.reachableSource(ctx, req.Msg.GetSourceId())
	if err != nil {
		return nil, err
	}
	docs, err := s.svc.ListDocuments(ctx, src.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(docs))
	for i, d := range docs {
		ids[i] = d.ID
	}
	stats, err := s.svc.DocumentStats(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]*knowledgev1.KnowledgeDocument, 0, len(docs))
	for _, d := range docs {
		out = append(out, documentToProto(d, src.PublicID, stats[d.ID]))
	}
	return connect.NewResponse(&knowledgev1.ListKnowledgeDocumentsResponse{Documents: out}), nil
}

// GetKnowledgeDocument loads a document by id.
func (s *KnowledgeService) GetKnowledgeDocument(
	ctx context.Context, req *connect.Request[knowledgev1.GetKnowledgeDocumentRequest],
) (*connect.Response[knowledgev1.GetKnowledgeDocumentResponse], error) {
	ctx, doc, src, err := s.reachableDocument(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	out, err := s.documentOut(ctx, doc, src.PublicID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&knowledgev1.GetKnowledgeDocumentResponse{Document: out}), nil
}

// DeleteKnowledgeDocument deletes a document and its passages; its file is swept.
func (s *KnowledgeService) DeleteKnowledgeDocument(
	ctx context.Context, req *connect.Request[knowledgev1.DeleteKnowledgeDocumentRequest],
) (*connect.Response[knowledgev1.DeleteKnowledgeDocumentResponse], error) {
	ctx, doc, _, err := s.reachableDocument(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.DeleteDocument(ctx, doc.ID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&knowledgev1.DeleteKnowledgeDocumentResponse{}), nil
}

// SetEmbeddingModel sets the project's embedding model; a new model re-embeds every document.
func (s *KnowledgeService) SetEmbeddingModel(
	ctx context.Context, req *connect.Request[knowledgev1.SetEmbeddingModelRequest],
) (*connect.Response[knowledgev1.SetEmbeddingModelResponse], error) {
	m := req.Msg
	ctx, projectID, err := s.project(ctx, m.GetProjectId(), authz.ReachWholeProject)
	if err != nil {
		return nil, err
	}
	providerID, owner, ok, err := s.refs.ProviderID(ctx, m.GetProviderId())
	if err != nil {
		return nil, err
	}
	// SECURITY: another project's provider answers like a model the provider does not have.
	if !ok || owner != projectID {
		return nil, &EmbeddingModelInvalidError{Reason: EmbeddingNotEmbedding}
	}
	st, err := s.svc.SetEmbeddingModel(ctx, projectID, providerID, m.GetModel())
	if err != nil {
		return nil, err
	}
	out, err := s.settingsToProto(ctx, st)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&knowledgev1.SetEmbeddingModelResponse{EmbeddingModel: out}), nil
}

// TestSearch searches one source the way a turn does.
func (s *KnowledgeService) TestSearch(
	ctx context.Context, req *connect.Request[knowledgev1.TestSearchRequest],
) (*connect.Response[knowledgev1.TestSearchResponse], error) {
	ctx, src, err := s.reachableSource(ctx, req.Msg.GetSourceId())
	if err != nil {
		return nil, err
	}
	hits, err := s.svc.TestSearch(ctx, src, req.Msg.GetQuery())
	if err != nil {
		return nil, err
	}
	out := make([]*knowledgev1.KnowledgeHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, &knowledgev1.KnowledgeHit{
			Text: h.Text, Title: h.Title, Heading: h.Heading, Page: count(h.Page), Url: h.URL, Score: h.Score,
		})
	}
	return connect.NewResponse(&knowledgev1.TestSearchResponse{Hits: out}), nil
}

// SECURITY: unknown and out-of-reach sources answer the same not-found; a reached source scopes the call to its org and project.
func (s *KnowledgeService) reachableSource(ctx context.Context, publicID string) (context.Context, *Source, error) {
	r, err := s.svc.ResolveSource(ctx, publicID)
	if IsSourceNotFoundError(err) {
		return ctx, nil, &SourceNotFoundError{}
	}
	if err != nil {
		return ctx, nil, err
	}
	if !authz.ReachResource(ctx, r.OrgID, r.ProjectID, r.ID) {
		return ctx, nil, &SourceNotFoundError{}
	}
	ctx = scoped(ctx, r.OrgID, r.ProjectID)
	src, err := s.svc.GetSource(ctx, r.ID)
	return ctx, src, err
}

// SECURITY: a document is reached through its source; unknown and out-of-reach answer the same not-found.
func (s *KnowledgeService) reachableDocument(ctx context.Context, publicID string) (context.Context, *Document, *Source, error) {
	r, err := s.svc.ResolveDocument(ctx, publicID)
	if IsDocumentNotFoundError(err) {
		return ctx, nil, nil, &DocumentNotFoundError{}
	}
	if err != nil {
		return ctx, nil, nil, err
	}
	if !authz.ReachResource(ctx, r.OrgID, r.ProjectID, r.SourceID) {
		return ctx, nil, nil, &DocumentNotFoundError{}
	}
	ctx = scoped(ctx, r.OrgID, r.ProjectID)
	src, err := s.svc.GetSource(ctx, r.SourceID)
	if err != nil {
		return ctx, nil, nil, err
	}
	doc, err := s.svc.GetDocument(ctx, r.ID)
	return ctx, doc, src, err
}

// SECURITY: an unknown or unreachable project is reported as an absent source; a reached project scopes the call to its org and itself.
func (s *KnowledgeService) project(
	ctx context.Context, publicID string, reach func(ctx context.Context, orgID, projectID uuid.UUID) bool,
) (context.Context, uuid.UUID, error) {
	id, orgID, ok, err := s.refs.ProjectID(ctx, publicID)
	if err != nil {
		return ctx, uuid.Nil, err
	}
	if !ok || !reach(ctx, orgID, id) {
		return ctx, uuid.Nil, &SourceNotFoundError{}
	}
	return scoped(ctx, orgID, id), id, nil
}

// SECURITY: a reached resource scopes the call to its own org and project, replacing the session's active project.
func scoped(ctx context.Context, orgID, projectID uuid.UUID) context.Context {
	return tenant.WithProject(tenant.WithOrg(ctx, orgID), projectID)
}

func (s *KnowledgeService) sourceOut(ctx context.Context, src *Source) (*knowledgev1.KnowledgeSource, error) {
	orgPub, err := s.refs.OrgPublicID(ctx, src.OrgID)
	if err != nil {
		return nil, err
	}
	projectPub, err := s.refs.ProjectPublicID(ctx, src.ProjectID)
	if err != nil {
		return nil, err
	}
	v, err := s.svc.SourceView(ctx, src)
	if err != nil {
		return nil, err
	}
	return sourceToProto(v, orgPub, projectPub, shownAgents(ctx)), nil
}

// SECURITY: a provider that no longer resolves renders empty and flagged, never as its UUID and never as an error.
func (s *KnowledgeService) settingsToProto(ctx context.Context, st *Settings) (*knowledgev1.EmbeddingModel, error) {
	pub, ok, err := s.refs.ProviderPublicID(ctx, st.ProviderID)
	if err != nil {
		return nil, err
	}
	return &knowledgev1.EmbeddingModel{
		ProviderId: pub, ProviderMissing: !ok, Model: st.Model, Dims: count(st.Dims), UpdatedAt: timestamppb.New(st.UpdatedAt),
	}, nil
}

// SECURITY: used_by names agents, so it shows only to a caller that may read agents, and only the agents that caller reaches.
func shownAgents(ctx context.Context) func(src *Source, u UsedBy) bool {
	if !authz.Allowed(session.PrincipalFrom(ctx), scope.AgentRead) {
		return nil
	}
	return func(src *Source, u UsedBy) bool { return authz.ReachResource(ctx, src.OrgID, src.ProjectID, u.AgentID) }
}

func sourceToProto(v SourceView, orgPub, projectPub string, shown func(*Source, UsedBy) bool) *knowledgev1.KnowledgeSource {
	src := v.Source
	out := &knowledgev1.KnowledgeSource{
		Id: src.PublicID, OrgId: orgPub, ProjectId: projectPub, Kind: kindToProto(src.Kind), Backend: backendToProto(src.Backend),
		Name: src.Name, SeedUrl: src.SeedURL, Status: sourceStatusToProto(src.Status), Failure: failureToProto(src.Failure),
		DocumentCount: count(v.Documents.Total), ReadyCount: count(v.Documents.Ready), FailedCount: count(v.Documents.Failed),
		CreatedAt: timestamppb.New(src.CreatedAt), UpdatedAt: timestamppb.New(src.UpdatedAt),
	}
	if shown == nil {
		return out
	}
	for _, u := range v.UsedBy {
		if !shown(src, u) {
			continue
		}
		out.UsedBy = append(out.UsedBy, &knowledgev1.KnowledgeUsedBy{AgentId: u.AgentPublicID, AgentName: u.AgentName})
	}
	return out
}

func (s *KnowledgeService) documentOut(ctx context.Context, d *Document, sourcePub string) (*knowledgev1.KnowledgeDocument, error) {
	stats, err := s.svc.DocumentStats(ctx, []uuid.UUID{d.ID})
	if err != nil {
		return nil, err
	}
	return documentToProto(d, sourcePub, stats[d.ID]), nil
}

func documentToProto(d *Document, sourcePub string, st DocumentStats) *knowledgev1.KnowledgeDocument {
	return &knowledgev1.KnowledgeDocument{
		Id: d.PublicID, SourceId: sourcePub, Title: d.Title, Url: d.URL, Status: documentStatusToProto(d.Status),
		Failure: failureToProto(d.Failure), PageCount: count(d.PageCount), EmbedTokens: d.EmbedTokens, EmbedCostMicros: d.EmbedCostMicros,
		SizeBytes: st.SizeBytes, ChunkCount: count(st.Chunks),
		CreatedAt: timestamppb.New(d.CreatedAt), UpdatedAt: timestamppb.New(d.UpdatedAt),
	}
}

func count(n int) int32 { return int32(min(n, 1<<30)) } //nolint:gosec // G115: clamped.

func failureToProto(f *Failure) *knowledgev1.KnowledgeFailure {
	if f == nil {
		return nil
	}
	return &knowledgev1.KnowledgeFailure{Code: string(f.Code), Reason: f.Reason}
}

func kindToProto(k Kind) knowledgev1.KnowledgeSourceKind {
	switch k {
	case KindUpload:
		return knowledgev1.KnowledgeSourceKind_KNOWLEDGE_SOURCE_KIND_UPLOAD
	case KindURL:
		return knowledgev1.KnowledgeSourceKind_KNOWLEDGE_SOURCE_KIND_URL
	}
	return knowledgev1.KnowledgeSourceKind_KNOWLEDGE_SOURCE_KIND_UNSPECIFIED
}

func backendToProto(b Backend) knowledgev1.KnowledgeBackend {
	if b == BackendPgvector {
		return knowledgev1.KnowledgeBackend_KNOWLEDGE_BACKEND_PGVECTOR
	}
	return knowledgev1.KnowledgeBackend_KNOWLEDGE_BACKEND_UNSPECIFIED
}

func sourceStatusToProto(s SourceStatus) knowledgev1.KnowledgeSourceStatus {
	switch s {
	case SourceIdle:
		return knowledgev1.KnowledgeSourceStatus_KNOWLEDGE_SOURCE_STATUS_IDLE
	case SourceCrawling:
		return knowledgev1.KnowledgeSourceStatus_KNOWLEDGE_SOURCE_STATUS_CRAWLING
	case SourceFailed:
		return knowledgev1.KnowledgeSourceStatus_KNOWLEDGE_SOURCE_STATUS_FAILED
	}
	return knowledgev1.KnowledgeSourceStatus_KNOWLEDGE_SOURCE_STATUS_UNSPECIFIED
}

func documentStatusToProto(s DocumentStatus) knowledgev1.KnowledgeDocumentStatus {
	switch s {
	case DocumentUploading:
		return knowledgev1.KnowledgeDocumentStatus_KNOWLEDGE_DOCUMENT_STATUS_UPLOADING
	case DocumentPending:
		return knowledgev1.KnowledgeDocumentStatus_KNOWLEDGE_DOCUMENT_STATUS_PENDING
	case DocumentProcessing:
		return knowledgev1.KnowledgeDocumentStatus_KNOWLEDGE_DOCUMENT_STATUS_PROCESSING
	case DocumentReady:
		return knowledgev1.KnowledgeDocumentStatus_KNOWLEDGE_DOCUMENT_STATUS_READY
	case DocumentFailed:
		return knowledgev1.KnowledgeDocumentStatus_KNOWLEDGE_DOCUMENT_STATUS_FAILED
	}
	return knowledgev1.KnowledgeDocumentStatus_KNOWLEDGE_DOCUMENT_STATUS_UNSPECIFIED
}
