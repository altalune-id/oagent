package entity

import "github.com/go-jet/jet/v2/postgres"

// KnowledgeSettings is the jet binding for the knowledge_settings table.
type KnowledgeSettings struct {
	postgres.Table

	ProjectID  postgres.ColumnString
	OrgID      postgres.ColumnString
	ProviderID postgres.ColumnString
	Model      postgres.ColumnString
	Dims       postgres.ColumnInteger
	UpdatedAt  postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewKnowledgeSettings builds the knowledge_settings binding.
func NewKnowledgeSettings(schema, tablePrefix string) *KnowledgeSettings {
	if schema == "" {
		schema = "public"
	}
	var (
		projectID  = postgres.StringColumn("project_id")
		orgID      = postgres.StringColumn("org_id")
		providerID = postgres.StringColumn("provider_id")
		model      = postgres.StringColumn("model")
		dims       = postgres.IntegerColumn("dims")
		updatedAt  = postgres.TimestampzColumn("updated_at")
		all        = postgres.ColumnList{projectID, orgID, providerID, model, dims, updatedAt}
	)
	return &KnowledgeSettings{
		Table:      postgres.NewTable(schema, tablePrefix+"knowledge_settings", "knowledge_settings", all...),
		ProjectID:  projectID,
		OrgID:      orgID,
		ProviderID: providerID,
		Model:      model,
		Dims:       dims,
		UpdatedAt:  updatedAt,
		AllColumns: all,
	}
}
