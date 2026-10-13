package entity

import "github.com/go-jet/jet/v2/postgres"

// Invites is the jet binding for the invites table.
type Invites struct {
	postgres.Table

	ID         postgres.ColumnString
	PublicID   postgres.ColumnString
	OrgID      postgres.ColumnString
	Email      postgres.ColumnString
	Role       postgres.ColumnString
	TokenHash  postgres.ColumnString
	Draft      postgres.ColumnBool
	InvitedBy  postgres.ColumnString
	ExpiresAt  postgres.ColumnTimestampz
	AcceptedAt postgres.ColumnTimestampz
	AcceptedBy postgres.ColumnString
	RevokedAt  postgres.ColumnTimestampz
	CreatedAt  postgres.ColumnTimestampz
	UpdatedAt  postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewInvites builds the invites binding.
func NewInvites(schema, tablePrefix string) *Invites {
	if schema == "" {
		schema = "public"
	}
	var (
		id         = postgres.StringColumn("id")
		publicID   = postgres.StringColumn("public_id")
		orgID      = postgres.StringColumn("org_id")
		email      = postgres.StringColumn("email")
		role       = postgres.StringColumn("role")
		tokenHash  = postgres.StringColumn("token_hash")
		draft      = postgres.BoolColumn("draft")
		invitedBy  = postgres.StringColumn("invited_by")
		expiresAt  = postgres.TimestampzColumn("expires_at")
		acceptedAt = postgres.TimestampzColumn("accepted_at")
		acceptedBy = postgres.StringColumn("accepted_by")
		revokedAt  = postgres.TimestampzColumn("revoked_at")
		createdAt  = postgres.TimestampzColumn("created_at")
		updatedAt  = postgres.TimestampzColumn("updated_at")
		all        = postgres.ColumnList{
			id, publicID, orgID, email, role, tokenHash, draft, invitedBy,
			expiresAt, acceptedAt, acceptedBy, revokedAt, createdAt, updatedAt,
		}
	)
	return &Invites{
		Table:      postgres.NewTable(schema, tablePrefix+"invites", "invites", all...),
		ID:         id,
		PublicID:   publicID,
		OrgID:      orgID,
		Email:      email,
		Role:       role,
		TokenHash:  tokenHash,
		Draft:      draft,
		InvitedBy:  invitedBy,
		ExpiresAt:  expiresAt,
		AcceptedAt: acceptedAt,
		AcceptedBy: acceptedBy,
		RevokedAt:  revokedAt,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
		AllColumns: all,
	}
}
