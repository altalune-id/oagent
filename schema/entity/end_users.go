package entity

import "github.com/go-jet/jet/v2/postgres"

// EndUsers is the jet binding for the end_users table.
type EndUsers struct {
	postgres.Table

	ID          postgres.ColumnString
	PublicID    postgres.ColumnString
	OrgID       postgres.ColumnString
	ProjectID   postgres.ColumnString
	CreatedAt   postgres.ColumnTimestampz
	UpdatedAt   postgres.ColumnTimestampz
	DisplayName postgres.ColumnString

	AllColumns postgres.ColumnList
}

// NewEndUsers builds the end_users binding.
func NewEndUsers(schema, tablePrefix string) *EndUsers {
	if schema == "" {
		schema = "public"
	}
	var (
		id          = postgres.StringColumn("id")
		publicID    = postgres.StringColumn("public_id")
		orgID       = postgres.StringColumn("org_id")
		projectID   = postgres.StringColumn("project_id")
		createdAt   = postgres.TimestampzColumn("created_at")
		updatedAt   = postgres.TimestampzColumn("updated_at")
		displayName = postgres.StringColumn("display_name")
		all         = postgres.ColumnList{
			id, publicID, orgID, projectID, createdAt, updatedAt, displayName,
		}
	)
	return &EndUsers{
		Table:       postgres.NewTable(schema, tablePrefix+"end_users", "end_users", all...),
		ID:          id,
		PublicID:    publicID,
		OrgID:       orgID,
		ProjectID:   projectID,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		DisplayName: displayName,
		AllColumns:  all,
	}
}
