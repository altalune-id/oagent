package entity

import "github.com/go-jet/jet/v2/postgres"

// MCPServers is the jet binding for the mcp_servers table.
type MCPServers struct {
	postgres.Table

	ID         postgres.ColumnString
	PublicID   postgres.ColumnString
	OrgID      postgres.ColumnString
	ProjectID  postgres.ColumnString
	Name       postgres.ColumnString
	URL        postgres.ColumnString
	AuthKind   postgres.ColumnString
	AuthHeader postgres.ColumnString
	AuthSealed postgres.ColumnBytea
	TimeoutMS  postgres.ColumnInteger
	Trusted    postgres.ColumnBool
	CreatedAt  postgres.ColumnTimestampz
	UpdatedAt  postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewMCPServers builds the mcp_servers binding.
func NewMCPServers(schema, tablePrefix string) *MCPServers {
	if schema == "" {
		schema = "public"
	}
	var (
		id         = postgres.StringColumn("id")
		publicID   = postgres.StringColumn("public_id")
		orgID      = postgres.StringColumn("org_id")
		projectID  = postgres.StringColumn("project_id")
		name       = postgres.StringColumn("name")
		url        = postgres.StringColumn("url")
		authKind   = postgres.StringColumn("auth_kind")
		authHeader = postgres.StringColumn("auth_header")
		authSealed = postgres.ByteaColumn("auth_sealed")
		timeoutMS  = postgres.IntegerColumn("timeout_ms")
		trusted    = postgres.BoolColumn("trusted")
		createdAt  = postgres.TimestampzColumn("created_at")
		updatedAt  = postgres.TimestampzColumn("updated_at")
		all        = postgres.ColumnList{
			id, publicID, orgID, projectID, name, url, authKind, authHeader, authSealed, timeoutMS, trusted, createdAt, updatedAt,
		}
	)
	return &MCPServers{
		Table:      postgres.NewTable(schema, tablePrefix+"mcp_servers", "mcp_servers", all...),
		ID:         id,
		PublicID:   publicID,
		OrgID:      orgID,
		ProjectID:  projectID,
		Name:       name,
		URL:        url,
		AuthKind:   authKind,
		AuthHeader: authHeader,
		AuthSealed: authSealed,
		TimeoutMS:  timeoutMS,
		Trusted:    trusted,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
		AllColumns: all,
	}
}
