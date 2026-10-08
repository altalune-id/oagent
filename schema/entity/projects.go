package entity

import "github.com/go-jet/jet/v2/postgres"

// Projects is the jet binding for the projects table.
type Projects struct {
	postgres.Table

	ID        postgres.ColumnString
	PublicID  postgres.ColumnString
	OrgID     postgres.ColumnString
	Slug      postgres.ColumnString
	Name      postgres.ColumnString
	CreatedBy postgres.ColumnString
	CreatedAt postgres.ColumnTimestampz
	UpdatedAt postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewProjects builds the projects binding.
func NewProjects(schema, tablePrefix string) *Projects {
	if schema == "" {
		schema = "public"
	}
	var (
		id        = postgres.StringColumn("id")
		publicID  = postgres.StringColumn("public_id")
		orgID     = postgres.StringColumn("org_id")
		slug      = postgres.StringColumn("slug")
		name      = postgres.StringColumn("name")
		createdBy = postgres.StringColumn("created_by")
		createdAt = postgres.TimestampzColumn("created_at")
		updatedAt = postgres.TimestampzColumn("updated_at")
		all       = postgres.ColumnList{id, publicID, orgID, slug, name, createdBy, createdAt, updatedAt}
	)
	return &Projects{
		Table:      postgres.NewTable(schema, tablePrefix+"projects", "projects", all...),
		ID:         id,
		PublicID:   publicID,
		OrgID:      orgID,
		Slug:       slug,
		Name:       name,
		CreatedBy:  createdBy,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
		AllColumns: all,
	}
}
