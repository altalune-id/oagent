package entity

import "github.com/go-jet/jet/v2/postgres"

// Agents is the jet binding for the agents table.
type Agents struct {
	postgres.Table

	ID          postgres.ColumnString
	PublicID    postgres.ColumnString
	OrgID       postgres.ColumnString
	ProjectID   postgres.ColumnString
	Slug        postgres.ColumnString
	Name        postgres.ColumnString
	Description postgres.ColumnString
	CreatedBy   postgres.ColumnString
	CreatedAt   postgres.ColumnTimestampz
	UpdatedAt   postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewAgents builds the agents binding.
func NewAgents(schema, tablePrefix string) *Agents {
	if schema == "" {
		schema = "public"
	}
	var (
		id          = postgres.StringColumn("id")
		publicID    = postgres.StringColumn("public_id")
		orgID       = postgres.StringColumn("org_id")
		projectID   = postgres.StringColumn("project_id")
		slug        = postgres.StringColumn("slug")
		name        = postgres.StringColumn("name")
		description = postgres.StringColumn("description")
		createdBy   = postgres.StringColumn("created_by")
		createdAt   = postgres.TimestampzColumn("created_at")
		updatedAt   = postgres.TimestampzColumn("updated_at")
		all         = postgres.ColumnList{id, publicID, orgID, projectID, slug, name, description, createdBy, createdAt, updatedAt}
	)
	return &Agents{
		Table:       postgres.NewTable(schema, tablePrefix+"agents", "agents", all...),
		ID:          id,
		PublicID:    publicID,
		OrgID:       orgID,
		ProjectID:   projectID,
		Slug:        slug,
		Name:        name,
		Description: description,
		CreatedBy:   createdBy,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		AllColumns:  all,
	}
}
