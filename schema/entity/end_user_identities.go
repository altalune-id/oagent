package entity

import "github.com/go-jet/jet/v2/postgres"

// EndUserIdentities is the jet binding for the end_user_identities table.
type EndUserIdentities struct {
	postgres.Table

	ID        postgres.ColumnString
	OrgID     postgres.ColumnString
	ProjectID postgres.ColumnString
	EndUserID postgres.ColumnString
	Kind      postgres.ColumnString
	Value     postgres.ColumnString
	CreatedAt postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewEndUserIdentities builds the end_user_identities binding.
func NewEndUserIdentities(schema, tablePrefix string) *EndUserIdentities {
	if schema == "" {
		schema = "public"
	}
	var (
		id        = postgres.StringColumn("id")
		orgID     = postgres.StringColumn("org_id")
		projectID = postgres.StringColumn("project_id")
		endUserID = postgres.StringColumn("end_user_id")
		kind      = postgres.StringColumn("kind")
		value     = postgres.StringColumn("value")
		createdAt = postgres.TimestampzColumn("created_at")
		all       = postgres.ColumnList{id, orgID, projectID, endUserID, kind, value, createdAt}
	)
	return &EndUserIdentities{
		Table:      postgres.NewTable(schema, tablePrefix+"end_user_identities", "end_user_identities", all...),
		ID:         id,
		OrgID:      orgID,
		ProjectID:  projectID,
		EndUserID:  endUserID,
		Kind:       kind,
		Value:      value,
		CreatedAt:  createdAt,
		AllColumns: all,
	}
}
