package entity

import "github.com/go-jet/jet/v2/postgres"

// Orgs is the jet binding for the orgs table.
type Orgs struct {
	postgres.Table

	ID        postgres.ColumnString
	PublicID  postgres.ColumnString
	Slug      postgres.ColumnString
	Name      postgres.ColumnString
	CreatedBy postgres.ColumnString
	CreatedAt postgres.ColumnTimestampz
	UpdatedAt postgres.ColumnTimestampz
	IsSystem  postgres.ColumnBool

	AllColumns postgres.ColumnList
}

// NewOrgs builds the orgs binding.
func NewOrgs(schema, tablePrefix string) *Orgs {
	if schema == "" {
		schema = "public"
	}
	var (
		id        = postgres.StringColumn("id")
		publicID  = postgres.StringColumn("public_id")
		slug      = postgres.StringColumn("slug")
		name      = postgres.StringColumn("name")
		createdBy = postgres.StringColumn("created_by")
		createdAt = postgres.TimestampzColumn("created_at")
		updatedAt = postgres.TimestampzColumn("updated_at")
		isSystem  = postgres.BoolColumn("is_system")
		all       = postgres.ColumnList{id, publicID, slug, name, createdBy, createdAt, updatedAt, isSystem}
	)
	return &Orgs{
		Table:      postgres.NewTable(schema, tablePrefix+"orgs", "orgs", all...),
		ID:         id,
		PublicID:   publicID,
		Slug:       slug,
		Name:       name,
		CreatedBy:  createdBy,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
		IsSystem:   isSystem,
		AllColumns: all,
	}
}
