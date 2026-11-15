package entity

import "github.com/go-jet/jet/v2/postgres"

// WebSearchProviders is the jet binding for the web_search_providers table.
type WebSearchProviders struct {
	postgres.Table

	ProjectID          postgres.ColumnString
	OrgID              postgres.ColumnString
	Kind               postgres.ColumnString
	APIKeySealed       postgres.ColumnBytea
	PricePerCallMicros postgres.ColumnInteger
	CreatedAt          postgres.ColumnTimestampz
	UpdatedAt          postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewWebSearchProviders builds the web_search_providers binding.
func NewWebSearchProviders(schema, tablePrefix string) *WebSearchProviders {
	if schema == "" {
		schema = "public"
	}
	var (
		projectID          = postgres.StringColumn("project_id")
		orgID              = postgres.StringColumn("org_id")
		kind               = postgres.StringColumn("kind")
		apiKeySealed       = postgres.ByteaColumn("api_key_sealed")
		pricePerCallMicros = postgres.IntegerColumn("price_per_call_micros")
		createdAt          = postgres.TimestampzColumn("created_at")
		updatedAt          = postgres.TimestampzColumn("updated_at")
		all                = postgres.ColumnList{projectID, orgID, kind, apiKeySealed, pricePerCallMicros, createdAt, updatedAt}
	)
	return &WebSearchProviders{
		Table:              postgres.NewTable(schema, tablePrefix+"web_search_providers", "web_search_providers", all...),
		ProjectID:          projectID,
		OrgID:              orgID,
		Kind:               kind,
		APIKeySealed:       apiKeySealed,
		PricePerCallMicros: pricePerCallMicros,
		CreatedAt:          createdAt,
		UpdatedAt:          updatedAt,
		AllColumns:         all,
	}
}
