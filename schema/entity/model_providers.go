package entity

import "github.com/go-jet/jet/v2/postgres"

// ModelProviders is the jet binding for the model_providers table.
type ModelProviders struct {
	postgres.Table

	ID           postgres.ColumnString
	PublicID     postgres.ColumnString
	OrgID        postgres.ColumnString
	ProjectID    postgres.ColumnString
	Name         postgres.ColumnString
	Kind         postgres.ColumnString
	BaseURL      postgres.ColumnString
	APIKeySealed postgres.ColumnBytea
	// Models is JSONB, bound as a string.
	Models    postgres.ColumnString
	CreatedAt postgres.ColumnTimestampz
	UpdatedAt postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewModelProviders builds the model_providers binding.
func NewModelProviders(schema, tablePrefix string) *ModelProviders {
	if schema == "" {
		schema = "public"
	}
	var (
		id           = postgres.StringColumn("id")
		publicID     = postgres.StringColumn("public_id")
		orgID        = postgres.StringColumn("org_id")
		projectID    = postgres.StringColumn("project_id")
		name         = postgres.StringColumn("name")
		kind         = postgres.StringColumn("kind")
		baseURL      = postgres.StringColumn("base_url")
		apiKeySealed = postgres.ByteaColumn("api_key_sealed")
		models       = postgres.StringColumn("models")
		createdAt    = postgres.TimestampzColumn("created_at")
		updatedAt    = postgres.TimestampzColumn("updated_at")
		all          = postgres.ColumnList{id, publicID, orgID, projectID, name, kind, baseURL, apiKeySealed, models, createdAt, updatedAt}
	)
	return &ModelProviders{
		Table:        postgres.NewTable(schema, tablePrefix+"model_providers", "model_providers", all...),
		ID:           id,
		PublicID:     publicID,
		OrgID:        orgID,
		ProjectID:    projectID,
		Name:         name,
		Kind:         kind,
		BaseURL:      baseURL,
		APIKeySealed: apiKeySealed,
		Models:       models,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		AllColumns:   all,
	}
}
