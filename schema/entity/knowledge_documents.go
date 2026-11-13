package entity

import "github.com/go-jet/jet/v2/postgres"

// KnowledgeDocuments is the jet binding for the knowledge_documents table.
type KnowledgeDocuments struct {
	postgres.Table

	ID        postgres.ColumnString
	PublicID  postgres.ColumnString
	OrgID     postgres.ColumnString
	ProjectID postgres.ColumnString
	SourceID  postgres.ColumnString
	Title     postgres.ColumnString
	FileID    postgres.ColumnString
	URL       postgres.ColumnString
	// Content is JSONB, bound as a string.
	Content         postgres.ColumnString
	ContentHash     postgres.ColumnString
	Status          postgres.ColumnString
	ErrorCode       postgres.ColumnString
	ErrorReason     postgres.ColumnString
	PageCount       postgres.ColumnInteger
	EmbedTokens     postgres.ColumnInteger
	EmbedCostMicros postgres.ColumnInteger
	Attempts        postgres.ColumnInteger
	LeaseOwner      postgres.ColumnString
	LeaseUntil      postgres.ColumnTimestampz
	CreatedAt       postgres.ColumnTimestampz
	UpdatedAt       postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewKnowledgeDocuments builds the knowledge_documents binding.
func NewKnowledgeDocuments(schema, tablePrefix string) *KnowledgeDocuments {
	if schema == "" {
		schema = "public"
	}
	var (
		id              = postgres.StringColumn("id")
		publicID        = postgres.StringColumn("public_id")
		orgID           = postgres.StringColumn("org_id")
		projectID       = postgres.StringColumn("project_id")
		sourceID        = postgres.StringColumn("source_id")
		title           = postgres.StringColumn("title")
		fileID          = postgres.StringColumn("file_id")
		url             = postgres.StringColumn("url")
		content         = postgres.StringColumn("content")
		contentHash     = postgres.StringColumn("content_hash")
		status          = postgres.StringColumn("status")
		errorCode       = postgres.StringColumn("error_code")
		errorReason     = postgres.StringColumn("error_reason")
		pageCount       = postgres.IntegerColumn("page_count")
		embedTokens     = postgres.IntegerColumn("embed_tokens")
		embedCostMicros = postgres.IntegerColumn("embed_cost_micros")
		attempts        = postgres.IntegerColumn("attempts")
		leaseOwner      = postgres.StringColumn("lease_owner")
		leaseUntil      = postgres.TimestampzColumn("lease_until")
		createdAt       = postgres.TimestampzColumn("created_at")
		updatedAt       = postgres.TimestampzColumn("updated_at")
		all             = postgres.ColumnList{
			id, publicID, orgID, projectID, sourceID, title, fileID, url, content, contentHash, status, errorCode,
			errorReason, pageCount, embedTokens, embedCostMicros, attempts, leaseOwner, leaseUntil, createdAt, updatedAt,
		}
	)
	return &KnowledgeDocuments{
		Table:           postgres.NewTable(schema, tablePrefix+"knowledge_documents", "knowledge_documents", all...),
		ID:              id,
		PublicID:        publicID,
		OrgID:           orgID,
		ProjectID:       projectID,
		SourceID:        sourceID,
		Title:           title,
		FileID:          fileID,
		URL:             url,
		Content:         content,
		ContentHash:     contentHash,
		Status:          status,
		ErrorCode:       errorCode,
		ErrorReason:     errorReason,
		PageCount:       pageCount,
		EmbedTokens:     embedTokens,
		EmbedCostMicros: embedCostMicros,
		Attempts:        attempts,
		LeaseOwner:      leaseOwner,
		LeaseUntil:      leaseUntil,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		AllColumns:      all,
	}
}
