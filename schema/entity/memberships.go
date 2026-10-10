package entity

import "github.com/go-jet/jet/v2/postgres"

// Memberships is the jet binding for the memberships table.
type Memberships struct {
	postgres.Table

	ID        postgres.ColumnString
	PublicID  postgres.ColumnString
	OrgID     postgres.ColumnString
	UserID    postgres.ColumnString
	Role      postgres.ColumnString
	CreatedAt postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewMemberships builds the memberships binding.
func NewMemberships(schema, tablePrefix string) *Memberships {
	if schema == "" {
		schema = "public"
	}
	var (
		id        = postgres.StringColumn("id")
		publicID  = postgres.StringColumn("public_id")
		orgID     = postgres.StringColumn("org_id")
		userID    = postgres.StringColumn("user_id")
		role      = postgres.StringColumn("role")
		createdAt = postgres.TimestampzColumn("created_at")
		all       = postgres.ColumnList{id, publicID, orgID, userID, role, createdAt}
	)
	return &Memberships{
		Table:      postgres.NewTable(schema, tablePrefix+"memberships", "memberships", all...),
		ID:         id,
		PublicID:   publicID,
		OrgID:      orgID,
		UserID:     userID,
		Role:       role,
		CreatedAt:  createdAt,
		AllColumns: all,
	}
}
