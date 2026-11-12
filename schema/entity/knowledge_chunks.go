package entity

import "github.com/go-jet/jet/v2/postgres"

// KnowledgeChunks is the jet binding for the knowledge_chunks table.
type KnowledgeChunks struct {
	postgres.Table

	ID          postgres.ColumnString
	OrgID       postgres.ColumnString
	ProjectID   postgres.ColumnString
	SourceID    postgres.ColumnString
	DocumentID  postgres.ColumnString
	Ordinal     postgres.ColumnInteger
	HeadingPath postgres.ColumnString
	Page        postgres.ColumnInteger
	URL         postgres.ColumnString
	Text        postgres.ColumnString
	// Embedding is a pgvector vector, bound as its text form.
	Embedding postgres.ColumnString
	Dims      postgres.ColumnInteger
	ModelKey  postgres.ColumnString
	CreatedAt postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewKnowledgeChunks builds the knowledge_chunks binding.
func NewKnowledgeChunks(schema, tablePrefix string) *KnowledgeChunks {
	if schema == "" {
		schema = "public"
	}
	var (
		id          = postgres.StringColumn("id")
		orgID       = postgres.StringColumn("org_id")
		projectID   = postgres.StringColumn("project_id")
		sourceID    = postgres.StringColumn("source_id")
		documentID  = postgres.StringColumn("document_id")
		ordinal     = postgres.IntegerColumn("ordinal")
		headingPath = postgres.StringColumn("heading_path")
		page        = postgres.IntegerColumn("page")
		url         = postgres.StringColumn("url")
		text        = postgres.StringColumn("text")
		embedding   = postgres.StringColumn("embedding")
		dims        = postgres.IntegerColumn("dims")
		modelKey    = postgres.StringColumn("model_key")
		createdAt   = postgres.TimestampzColumn("created_at")
		all         = postgres.ColumnList{
			id, orgID, projectID, sourceID, documentID, ordinal, headingPath, page, url, text, embedding, dims, modelKey,
			createdAt,
		}
	)
	return &KnowledgeChunks{
		Table:       postgres.NewTable(schema, tablePrefix+"knowledge_chunks", "knowledge_chunks", all...),
		ID:          id,
		OrgID:       orgID,
		ProjectID:   projectID,
		SourceID:    sourceID,
		DocumentID:  documentID,
		Ordinal:     ordinal,
		HeadingPath: headingPath,
		Page:        page,
		URL:         url,
		Text:        text,
		Embedding:   embedding,
		Dims:        dims,
		ModelKey:    modelKey,
		CreatedAt:   createdAt,
		AllColumns:  all,
	}
}
