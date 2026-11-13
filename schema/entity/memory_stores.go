package entity

import "github.com/go-jet/jet/v2/postgres"

// MemoryStores is the jet binding for the memory_stores table.
type MemoryStores struct {
	postgres.Table

	ID               postgres.ColumnString
	PublicID         postgres.ColumnString
	OrgID            postgres.ColumnString
	ProjectID        postgres.ColumnString
	Name             postgres.ColumnString
	Backend          postgres.ColumnString
	RememberAPIUsers postgres.ColumnBool
	CreatedAt        postgres.ColumnTimestampz
	UpdatedAt        postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewMemoryStores builds the memory_stores binding.
func NewMemoryStores(schema, tablePrefix string) *MemoryStores {
	if schema == "" {
		schema = "public"
	}
	var (
		id        = postgres.StringColumn("id")
		publicID  = postgres.StringColumn("public_id")
		orgID     = postgres.StringColumn("org_id")
		projectID = postgres.StringColumn("project_id")
		name      = postgres.StringColumn("name")
		backend   = postgres.StringColumn("backend")
		apiUsers  = postgres.BoolColumn("remember_api_users")
		createdAt = postgres.TimestampzColumn("created_at")
		updatedAt = postgres.TimestampzColumn("updated_at")
		all       = postgres.ColumnList{id, publicID, orgID, projectID, name, backend, apiUsers, createdAt, updatedAt}
	)
	return &MemoryStores{
		Table:            postgres.NewTable(schema, tablePrefix+"memory_stores", "memory_stores", all...),
		ID:               id,
		PublicID:         publicID,
		OrgID:            orgID,
		ProjectID:        projectID,
		Name:             name,
		Backend:          backend,
		RememberAPIUsers: apiUsers,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
		AllColumns:       all,
	}
}
