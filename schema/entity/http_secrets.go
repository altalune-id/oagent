package entity

import "github.com/go-jet/jet/v2/postgres"

// HTTPSecrets is the jet binding for the http_secrets table.
type HTTPSecrets struct {
	postgres.Table

	ID          postgres.ColumnString
	PublicID    postgres.ColumnString
	OrgID       postgres.ColumnString
	ProjectID   postgres.ColumnString
	Host        postgres.ColumnString
	Header      postgres.ColumnString
	ValueSealed postgres.ColumnBytea
	CreatedAt   postgres.ColumnTimestampz
	UpdatedAt   postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewHTTPSecrets builds the http_secrets binding.
func NewHTTPSecrets(schema, tablePrefix string) *HTTPSecrets {
	if schema == "" {
		schema = "public"
	}
	var (
		id          = postgres.StringColumn("id")
		publicID    = postgres.StringColumn("public_id")
		orgID       = postgres.StringColumn("org_id")
		projectID   = postgres.StringColumn("project_id")
		host        = postgres.StringColumn("host")
		header      = postgres.StringColumn("header")
		valueSealed = postgres.ByteaColumn("value_sealed")
		createdAt   = postgres.TimestampzColumn("created_at")
		updatedAt   = postgres.TimestampzColumn("updated_at")
		all         = postgres.ColumnList{id, publicID, orgID, projectID, host, header, valueSealed, createdAt, updatedAt}
	)
	return &HTTPSecrets{
		Table:       postgres.NewTable(schema, tablePrefix+"http_secrets", "http_secrets", all...),
		ID:          id,
		PublicID:    publicID,
		OrgID:       orgID,
		ProjectID:   projectID,
		Host:        host,
		Header:      header,
		ValueSealed: valueSealed,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		AllColumns:  all,
	}
}
