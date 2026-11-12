package knowledge

import (
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

type storeSQL struct {
	claimable, leaseHeld, otherChunks, deleteChunks, insertChunks, upsertPage, chunkTexts string
	sourceCounts, documentStats                                                           string
}

func newStoreSQL(fn string) storeSQL {
	docs, chunks, sources, settings := fn+"knowledge_documents", fn+"knowledge_chunks", fn+"knowledge_sources", fn+"knowledge_settings"
	return storeSQL{
		claimable: `SELECT c.id FROM (SELECT d.id, d.project_id, d.updated_at,
				row_number() OVER (PARTITION BY d.project_id ORDER BY d.updated_at, d.id) AS rn
			FROM ` + docs + ` d JOIN ` + settings + ` ks ON ks.org_id = d.org_id AND ks.project_id = d.project_id
			WHERE d.org_id = $1 AND (d.status = 'pending' OR (d.status = 'processing' AND d.lease_until <= $2))) c
			LEFT JOIN (SELECT project_id, count(*) AS n FROM ` + docs + `
				WHERE org_id = $1 AND status = 'processing' AND lease_until > $2 GROUP BY project_id) b ON b.project_id = c.project_id
			WHERE c.rn <= $3 - COALESCE(b.n, 0) ORDER BY c.rn, c.updated_at, c.id LIMIT $4`,
		leaseHeld: `SELECT count(*) FROM (SELECT id FROM ` + docs +
			` WHERE id = $1 AND org_id = $2 AND lease_owner = $3 AND status = 'processing' FOR UPDATE) l`,
		otherChunks:  `SELECT count(*) FROM ` + chunks + ` WHERE org_id = $1 AND project_id = $2 AND document_id <> $3`,
		deleteChunks: `DELETE FROM ` + chunks + ` WHERE org_id = $1 AND document_id = $2`,
		insertChunks: `INSERT INTO ` + chunks + ` (id, org_id, project_id, source_id, document_id, ordinal, heading_path, page, url, text,
			embedding, dims, model_key, created_at)
			SELECT x.id::uuid, $1, $2, $3, $4, x.ordinal, x.heading, NULLIF(x.page, 0), NULLIF(x.url, ''), x.text, x.vec::public.vector, $5, $6, $7
			FROM unnest($8::text[], $9::int[], $10::text[], $11::int[], $12::text[], $13::text[], $14::text[])
			AS x(id, ordinal, heading, page, url, text, vec)`,
		upsertPage: `INSERT INTO ` + docs + ` AS d (id, public_id, org_id, project_id, source_id, title, url, content, content_hash, status,
			embed_tokens, embed_cost_micros, created_at, updated_at)
			SELECT $1, $2, s.org_id, s.project_id, s.id, $3, $4, $5::jsonb, $9, 'pending', 0, 0, $6, $6 FROM ` + sources + ` s
			WHERE s.id = $7 AND s.org_id = $8
			ON CONFLICT (source_id, url) WHERE url IS NOT NULL DO UPDATE SET title = EXCLUDED.title, content = EXCLUDED.content,
			content_hash = EXCLUDED.content_hash, status = 'pending', error_code = NULL, error_reason = NULL, attempts = 0,
			lease_owner = NULL, lease_until = NULL, updated_at = EXCLUDED.updated_at
			WHERE d.status = 'failed' OR d.content_hash IS DISTINCT FROM EXCLUDED.content_hash`,
		chunkTexts: `SELECT ordinal, heading_path, COALESCE(page, 0), COALESCE(url, ''), text FROM ` + chunks +
			` WHERE org_id = $2 AND document_id = $1 ORDER BY ordinal`,
		sourceCounts: `SELECT count(*), count(*) FILTER (WHERE status = 'ready'), count(*) FILTER (WHERE status = 'failed') FROM ` + docs +
			` WHERE org_id = $1 AND source_id = $2`,
		documentStats: `SELECT d.id, COALESCE(f.size_bytes, 0),
			(SELECT count(*) FROM ` + chunks + ` c WHERE c.org_id = d.org_id AND c.document_id = d.id)
			FROM ` + docs + ` d LEFT JOIN ` + fn + `files f ON f.org_id = d.org_id AND f.id = d.file_id
			WHERE d.org_id = $1 AND d.id = ANY($2::uuid[])`,
	}
}

type postgresStore struct {
	sql             storeSQL
	pool            db.Pool
	sources         *entity.KnowledgeSources
	documents       *entity.KnowledgeDocuments
	settings        *entity.KnowledgeSettings
	resolveSource   string
	resolveDocument string
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	if schema == "" {
		schema = "public"
	}
	fn := schema + "." + tablePrefix
	return &postgresStore{
		pool:      pool,
		sources:   entity.NewKnowledgeSources(schema, tablePrefix),
		documents: entity.NewKnowledgeDocuments(schema, tablePrefix),
		settings:  entity.NewKnowledgeSettings(schema, tablePrefix),
		sql:       newStoreSQL(fn),
		// NOTE: RawStatement because go-jet has no builder for a set-returning function in FROM position.
		resolveSource: `SELECT x.id AS "knowledge_sources.id", x.org_id AS "knowledge_sources.org_id", ` +
			`x.project_id AS "knowledge_sources.project_id" FROM ` + fn + "resolve_knowledge_source_by_public_id(#publicID) x",
		resolveDocument: `SELECT x.id AS "knowledge_documents.id", x.org_id AS "knowledge_documents.org_id", ` +
			`x.project_id AS "knowledge_documents.project_id", x.source_id AS "knowledge_documents.source_id" FROM ` +
			fn + "resolve_knowledge_document_by_public_id(#publicID) x",
	}
}

var _ Store = (*postgresStore)(nil)

type sourceRow struct {
	ID          uuid.UUID `alias:"knowledge_sources.id"`
	PublicID    string    `alias:"knowledge_sources.public_id"`
	OrgID       uuid.UUID `alias:"knowledge_sources.org_id"`
	ProjectID   uuid.UUID `alias:"knowledge_sources.project_id"`
	Kind        string    `alias:"knowledge_sources.kind"`
	Backend     string    `alias:"knowledge_sources.backend"`
	Name        string    `alias:"knowledge_sources.name"`
	SeedURL     *string   `alias:"knowledge_sources.seed_url"`
	Status      string    `alias:"knowledge_sources.status"`
	ErrorCode   *string   `alias:"knowledge_sources.error_code"`
	ErrorReason *string   `alias:"knowledge_sources.error_reason"`
	CreatedAt   time.Time `alias:"knowledge_sources.created_at"`
	UpdatedAt   time.Time `alias:"knowledge_sources.updated_at"`
}

func (r *sourceRow) toSource() *Source {
	s := &Source{
		ID: r.ID, PublicID: r.PublicID, OrgID: r.OrgID, ProjectID: r.ProjectID, Kind: Kind(r.Kind), Backend: Backend(r.Backend),
		Name: r.Name, Status: SourceStatus(r.Status), Failure: failure(r.ErrorCode, r.ErrorReason),
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if r.SeedURL != nil {
		s.SeedURL = *r.SeedURL
	}
	return s
}

type documentRow struct {
	ID              uuid.UUID  `alias:"knowledge_documents.id"`
	PublicID        string     `alias:"knowledge_documents.public_id"`
	OrgID           uuid.UUID  `alias:"knowledge_documents.org_id"`
	ProjectID       uuid.UUID  `alias:"knowledge_documents.project_id"`
	SourceID        uuid.UUID  `alias:"knowledge_documents.source_id"`
	Title           string     `alias:"knowledge_documents.title"`
	FileID          *uuid.UUID `alias:"knowledge_documents.file_id"`
	URL             *string    `alias:"knowledge_documents.url"`
	Status          string     `alias:"knowledge_documents.status"`
	ErrorCode       *string    `alias:"knowledge_documents.error_code"`
	ErrorReason     *string    `alias:"knowledge_documents.error_reason"`
	PageCount       *int32     `alias:"knowledge_documents.page_count"`
	EmbedTokens     int64      `alias:"knowledge_documents.embed_tokens"`
	EmbedCostMicros int64      `alias:"knowledge_documents.embed_cost_micros"`
	Attempts        int32      `alias:"knowledge_documents.attempts"`
	CreatedAt       time.Time  `alias:"knowledge_documents.created_at"`
	UpdatedAt       time.Time  `alias:"knowledge_documents.updated_at"`
}

func (r *documentRow) toDocument() *Document {
	d := &Document{
		ID: r.ID, PublicID: r.PublicID, OrgID: r.OrgID, ProjectID: r.ProjectID, SourceID: r.SourceID, Title: r.Title,
		FileID: r.FileID, Status: DocumentStatus(r.Status), Failure: failure(r.ErrorCode, r.ErrorReason),
		EmbedTokens: r.EmbedTokens, EmbedCostMicros: r.EmbedCostMicros, Attempts: int(r.Attempts),
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if r.URL != nil {
		d.URL = *r.URL
	}
	if r.PageCount != nil {
		d.PageCount = int(*r.PageCount)
	}
	return d
}

func failure(code, reason *string) *Failure {
	if code == nil {
		return nil
	}
	f := &Failure{Code: apperror.Code(*code)}
	if reason != nil {
		f.Reason = *reason
	}
	return f
}

type settingsRow struct {
	ProjectID  uuid.UUID `alias:"knowledge_settings.project_id"`
	OrgID      uuid.UUID `alias:"knowledge_settings.org_id"`
	ProviderID uuid.UUID `alias:"knowledge_settings.provider_id"`
	Model      string    `alias:"knowledge_settings.model"`
	Dims       int32     `alias:"knowledge_settings.dims"`
	UpdatedAt  time.Time `alias:"knowledge_settings.updated_at"`
}

func (r *settingsRow) toSettings() *Settings {
	return &Settings{
		OrgID: r.OrgID, ProjectID: r.ProjectID, ProviderID: r.ProviderID, Model: r.Model, Dims: int(r.Dims), UpdatedAt: r.UpdatedAt.UTC(),
	}
}
