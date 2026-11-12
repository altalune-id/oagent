package knowledge

import (
	"context"
	"fmt"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
)

// SourceByID loads a source of the org on ctx.
func (s *postgresStore) SourceByID(ctx context.Context, id uuid.UUID) (*Source, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.sources
	stmt := postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).LIMIT(1)
	var row sourceRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &SourceNotFoundError{}
		}
		return nil, fmt.Errorf("knowledge.postgres: SourceByID: %w", err)
	}
	return row.toSource(), nil
}

// ListSources returns a project's sources, oldest first.
func (s *postgresStore) ListSources(ctx context.Context, projectID uuid.UUID) ([]*Source, error) {
	t := s.sources
	stmt := postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.ProjectID.EQ(postgres.UUID(projectID))).
		ORDER_BY(t.CreatedAt.ASC(), t.ID.ASC())
	var rows []sourceRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("knowledge.postgres: ListSources: %w", err)
	}
	out := make([]*Source, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toSource())
	}
	return out, nil
}

// DocumentCounts sums each source's documents in a project.
func (s *postgresStore) DocumentCounts(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID]DocumentCounts, error) {
	t := s.documents
	ready := postgres.COUNT(postgres.CASE().WHEN(t.Status.EQ(postgres.String(string(DocumentReady)))).THEN(t.ID))
	failed := postgres.COUNT(postgres.CASE().WHEN(t.Status.EQ(postgres.String(string(DocumentFailed)))).THEN(t.ID))
	stmt := postgres.SELECT(
		t.SourceID.AS("counts.source_id"), postgres.COUNT(t.ID).AS("counts.total"), ready.AS("counts.ready"), failed.AS("counts.failed"),
	).
		FROM(t).
		WHERE(t.ProjectID.EQ(postgres.UUID(projectID))).
		GROUP_BY(t.SourceID)
	var rows []struct {
		SourceID uuid.UUID `alias:"counts.source_id"`
		Total    int64     `alias:"counts.total"`
		Ready    int64     `alias:"counts.ready"`
		Failed   int64     `alias:"counts.failed"`
	}
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("knowledge.postgres: DocumentCounts: %w", err)
	}
	out := make(map[uuid.UUID]DocumentCounts, len(rows))
	for _, r := range rows {
		out[r.SourceID] = DocumentCounts{Total: int(r.Total), Ready: int(r.Ready), Failed: int(r.Failed)}
	}
	return out, nil
}

// ResolveSource names publicID's source and its org through a definer, so a caller with no active org can check reach and then scope.
func (s *postgresStore) ResolveSource(ctx context.Context, publicID string) (SourceRef, error) {
	var row struct {
		ID        uuid.UUID `alias:"knowledge_sources.id"`
		OrgID     uuid.UUID `alias:"knowledge_sources.org_id"`
		ProjectID uuid.UUID `alias:"knowledge_sources.project_id"`
	}
	stmt := postgres.RawStatement(s.resolveSource, postgres.RawArgs{"#publicID": publicID})
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return SourceRef{}, &SourceNotFoundError{}
		}
		return SourceRef{}, fmt.Errorf("knowledge.postgres: ResolveSource: %w", err)
	}
	return SourceRef(row), nil
}

// DocumentByID loads a document of the org on ctx.
func (s *postgresStore) DocumentByID(ctx context.Context, id uuid.UUID) (*Document, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	t := s.documents
	stmt := postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.ID.EQ(postgres.UUID(id)).AND(t.OrgID.EQ(postgres.UUID(tc.OrgID)))).LIMIT(1)
	var row documentRow
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &DocumentNotFoundError{}
		}
		return nil, fmt.Errorf("knowledge.postgres: DocumentByID: %w", err)
	}
	return row.toDocument(), nil
}

// ListDocuments returns a source's documents, oldest first.
func (s *postgresStore) ListDocuments(ctx context.Context, sourceID uuid.UUID) ([]*Document, error) {
	t := s.documents
	stmt := postgres.SELECT(t.AllColumns).FROM(t).
		WHERE(t.SourceID.EQ(postgres.UUID(sourceID))).
		ORDER_BY(t.CreatedAt.ASC(), t.ID.ASC())
	var rows []documentRow
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("knowledge.postgres: ListDocuments: %w", err)
	}
	out := make([]*Document, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].toDocument())
	}
	return out, nil
}

// ResolveDocument names publicID's document and where it lives through a definer.
func (s *postgresStore) ResolveDocument(ctx context.Context, publicID string) (DocumentRef, error) {
	var row struct {
		ID        uuid.UUID `alias:"knowledge_documents.id"`
		OrgID     uuid.UUID `alias:"knowledge_documents.org_id"`
		ProjectID uuid.UUID `alias:"knowledge_documents.project_id"`
		SourceID  uuid.UUID `alias:"knowledge_documents.source_id"`
	}
	stmt := postgres.RawStatement(s.resolveDocument, postgres.RawArgs{"#publicID": publicID})
	if err := db.Read(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return DocumentRef{}, &DocumentNotFoundError{}
		}
		return DocumentRef{}, fmt.Errorf("knowledge.postgres: ResolveDocument: %w", err)
	}
	return DocumentRef(row), nil
}

// Settings loads a project's embedding model, or *EmbeddingModelNotSetError.
func (s *postgresStore) Settings(ctx context.Context, projectID uuid.UUID) (*Settings, error) {
	t := s.settings
	stmt := postgres.SELECT(t.AllColumns).FROM(t).WHERE(t.ProjectID.EQ(postgres.UUID(projectID))).LIMIT(1)
	var row settingsRow
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &EmbeddingModelNotSetError{}
		}
		return nil, fmt.Errorf("knowledge.postgres: Settings: %w", err)
	}
	return row.toSettings(), nil
}

// EmbeddingModelOf names the model a project of the org on ctx embeds with through providerID, else "".
func (s *postgresStore) EmbeddingModelOf(ctx context.Context, providerID uuid.UUID) (string, error) {
	t := s.settings
	stmt := postgres.SELECT(t.Model).FROM(t).WHERE(t.ProviderID.EQ(postgres.UUID(providerID))).LIMIT(1)
	var row struct {
		Model string `alias:"knowledge_settings.model"`
	}
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return "", nil
		}
		return "", fmt.Errorf("knowledge.postgres: EmbeddingModelOf: %w", err)
	}
	return row.Model, nil
}

// ProjectChunkCount counts the project's chunks outside exceptDocument.
func (s *postgresStore) ProjectChunkCount(ctx context.Context, projectID, exceptDocument uuid.UUID) (int, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return 0, err
	}
	var n int
	err = db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error {
		rows, err := q.QueryContext(ctx, s.sql.otherChunks, tc.OrgID, projectID, exceptDocument)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		if rows.Next() {
			if err := rows.Scan(&n); err != nil {
				return err
			}
		}
		return rows.Err()
	})
	if err != nil {
		return 0, fmt.Errorf("knowledge.postgres: ProjectChunkCount: %w", err)
	}
	return n, nil
}

// ChunkTexts returns a document's chunks without their vectors, in order.
func (s *postgresStore) ChunkTexts(ctx context.Context, documentID uuid.UUID) ([]StoredChunk, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, err
	}
	var out []StoredChunk
	err = db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error {
		rows, err := q.QueryContext(ctx, s.sql.chunkTexts, documentID, tc.OrgID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var c StoredChunk
			if err := rows.Scan(&c.Ordinal, &c.HeadingPath, &c.Page, &c.URL, &c.Text); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("knowledge.postgres: ChunkTexts: %w", err)
	}
	return out, nil
}

// DocumentContent returns a crawled page's stored sections as JSON, or nil.
func (s *postgresStore) DocumentContent(ctx context.Context, documentID uuid.UUID) ([]byte, error) {
	t := s.documents
	stmt := postgres.SELECT(t.Content).FROM(t).WHERE(t.ID.EQ(postgres.UUID(documentID)))
	var row struct {
		Content *string `alias:"knowledge_documents.content"`
	}
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &row) }); err != nil {
		if db.IsNoRows(err) {
			return nil, &DocumentNotFoundError{}
		}
		return nil, fmt.Errorf("knowledge.postgres: DocumentContent: %w", err)
	}
	if row.Content == nil {
		return nil, nil
	}
	return []byte(*row.Content), nil
}

// PageURLs lists the URLs of a source's crawled pages.
func (s *postgresStore) PageURLs(ctx context.Context, sourceID uuid.UUID) ([]string, error) {
	t := s.documents
	stmt := postgres.SELECT(t.URL).FROM(t).WHERE(t.SourceID.EQ(postgres.UUID(sourceID)).AND(t.URL.IS_NOT_NULL())).ORDER_BY(t.URL.ASC())
	var rows []struct {
		URL string `alias:"knowledge_documents.url"`
	}
	if err := db.ReadPrimary(ctx, s.pool, func(q qrm.DB) error { return stmt.QueryContext(ctx, q, &rows) }); err != nil && !db.IsNoRows(err) {
		return nil, fmt.Errorf("knowledge.postgres: PageURLs: %w", err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.URL
	}
	return out, nil
}
