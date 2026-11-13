package entity

import "github.com/go-jet/jet/v2/postgres"

// MemoryFacts is the jet binding for the memory_facts table.
type MemoryFacts struct {
	postgres.Table

	ID          postgres.ColumnString
	PublicID    postgres.ColumnString
	OrgID       postgres.ColumnString
	ProjectID   postgres.ColumnString
	StoreID     postgres.ColumnString
	AgentID     postgres.ColumnString
	EndUserID   postgres.ColumnString
	Text        postgres.ColumnString
	TextHash    postgres.ColumnBytea
	SourceRunID postgres.ColumnString
	CreatedAt   postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewMemoryFacts builds the memory_facts binding.
func NewMemoryFacts(schema, tablePrefix string) *MemoryFacts {
	if schema == "" {
		schema = "public"
	}
	var (
		id          = postgres.StringColumn("id")
		publicID    = postgres.StringColumn("public_id")
		orgID       = postgres.StringColumn("org_id")
		projectID   = postgres.StringColumn("project_id")
		storeID     = postgres.StringColumn("store_id")
		agentID     = postgres.StringColumn("agent_id")
		endUserID   = postgres.StringColumn("end_user_id")
		text        = postgres.StringColumn("text")
		textHash    = postgres.ByteaColumn("text_hash")
		sourceRunID = postgres.StringColumn("source_run_id")
		createdAt   = postgres.TimestampzColumn("created_at")
		all         = postgres.ColumnList{id, publicID, orgID, projectID, storeID, agentID, endUserID, text, textHash, sourceRunID, createdAt}
	)
	return &MemoryFacts{
		Table:       postgres.NewTable(schema, tablePrefix+"memory_facts", "memory_facts", all...),
		ID:          id,
		PublicID:    publicID,
		OrgID:       orgID,
		ProjectID:   projectID,
		StoreID:     storeID,
		AgentID:     agentID,
		EndUserID:   endUserID,
		Text:        text,
		TextHash:    textHash,
		SourceRunID: sourceRunID,
		CreatedAt:   createdAt,
		AllColumns:  all,
	}
}
