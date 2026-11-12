package entity

import "github.com/go-jet/jet/v2/postgres"

// KnowledgeSources is the jet binding for the knowledge_sources table.
type KnowledgeSources struct {
	postgres.Table

	ID               postgres.ColumnString
	PublicID         postgres.ColumnString
	OrgID            postgres.ColumnString
	ProjectID        postgres.ColumnString
	Kind             postgres.ColumnString
	Backend          postgres.ColumnString
	Name             postgres.ColumnString
	SeedURL          postgres.ColumnString
	Status           postgres.ColumnString
	ErrorCode        postgres.ColumnString
	ErrorReason      postgres.ColumnString
	RefreshRequested postgres.ColumnBool
	RefreshedAt      postgres.ColumnTimestampz
	LeaseOwner       postgres.ColumnString
	LeaseUntil       postgres.ColumnTimestampz
	CreatedAt        postgres.ColumnTimestampz
	UpdatedAt        postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewKnowledgeSources builds the knowledge_sources binding.
func NewKnowledgeSources(schema, tablePrefix string) *KnowledgeSources {
	if schema == "" {
		schema = "public"
	}
	var (
		id               = postgres.StringColumn("id")
		publicID         = postgres.StringColumn("public_id")
		orgID            = postgres.StringColumn("org_id")
		projectID        = postgres.StringColumn("project_id")
		kind             = postgres.StringColumn("kind")
		backend          = postgres.StringColumn("backend")
		name             = postgres.StringColumn("name")
		seedURL          = postgres.StringColumn("seed_url")
		status           = postgres.StringColumn("status")
		errorCode        = postgres.StringColumn("error_code")
		errorReason      = postgres.StringColumn("error_reason")
		refreshRequested = postgres.BoolColumn("refresh_requested")
		refreshedAt      = postgres.TimestampzColumn("refreshed_at")
		leaseOwner       = postgres.StringColumn("lease_owner")
		leaseUntil       = postgres.TimestampzColumn("lease_until")
		createdAt        = postgres.TimestampzColumn("created_at")
		updatedAt        = postgres.TimestampzColumn("updated_at")
		all              = postgres.ColumnList{
			id, publicID, orgID, projectID, kind, backend, name, seedURL, status, errorCode, errorReason,
			refreshRequested, refreshedAt, leaseOwner, leaseUntil, createdAt, updatedAt,
		}
	)
	return &KnowledgeSources{
		Table:            postgres.NewTable(schema, tablePrefix+"knowledge_sources", "knowledge_sources", all...),
		ID:               id,
		PublicID:         publicID,
		OrgID:            orgID,
		ProjectID:        projectID,
		Kind:             kind,
		Backend:          backend,
		Name:             name,
		SeedURL:          seedURL,
		Status:           status,
		ErrorCode:        errorCode,
		ErrorReason:      errorReason,
		RefreshRequested: refreshRequested,
		RefreshedAt:      refreshedAt,
		LeaseOwner:       leaseOwner,
		LeaseUntil:       leaseUntil,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
		AllColumns:       all,
	}
}
