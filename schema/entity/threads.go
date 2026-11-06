package entity

import "github.com/go-jet/jet/v2/postgres"

// Threads is the jet binding for the threads table.
type Threads struct {
	postgres.Table

	ID              postgres.ColumnString
	PublicID        postgres.ColumnString
	OrgID           postgres.ColumnString
	ProjectID       postgres.ColumnString
	AgentID         postgres.ColumnString
	Kind            postgres.ColumnString
	ExternalKey     postgres.ColumnString
	LeaseOwner      postgres.ColumnString
	LeaseUntil      postgres.ColumnTimestampz
	CancelRequested postgres.ColumnBool
	NextSeq         postgres.ColumnInteger
	CreatedAt       postgres.ColumnTimestampz
	UpdatedAt       postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewThreads builds the threads binding.
func NewThreads(schema, tablePrefix string) *Threads {
	if schema == "" {
		schema = "public"
	}
	var (
		id              = postgres.StringColumn("id")
		publicID        = postgres.StringColumn("public_id")
		orgID           = postgres.StringColumn("org_id")
		projectID       = postgres.StringColumn("project_id")
		agentID         = postgres.StringColumn("agent_id")
		kind            = postgres.StringColumn("kind")
		externalKey     = postgres.StringColumn("external_key")
		leaseOwner      = postgres.StringColumn("lease_owner")
		leaseUntil      = postgres.TimestampzColumn("lease_until")
		cancelRequested = postgres.BoolColumn("cancel_requested")
		nextSeq         = postgres.IntegerColumn("next_seq")
		createdAt       = postgres.TimestampzColumn("created_at")
		updatedAt       = postgres.TimestampzColumn("updated_at")
		all             = postgres.ColumnList{
			id, publicID, orgID, projectID, agentID, kind, externalKey, leaseOwner, leaseUntil, cancelRequested,
			nextSeq, createdAt, updatedAt,
		}
	)
	return &Threads{
		Table:           postgres.NewTable(schema, tablePrefix+"threads", "threads", all...),
		ID:              id,
		PublicID:        publicID,
		OrgID:           orgID,
		ProjectID:       projectID,
		AgentID:         agentID,
		Kind:            kind,
		ExternalKey:     externalKey,
		LeaseOwner:      leaseOwner,
		LeaseUntil:      leaseUntil,
		CancelRequested: cancelRequested,
		NextSeq:         nextSeq,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		AllColumns:      all,
	}
}
