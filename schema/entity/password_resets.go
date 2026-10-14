package entity

import "github.com/go-jet/jet/v2/postgres"

// PasswordResets is the jet binding for the password_resets table.
type PasswordResets struct {
	postgres.Table

	ID        postgres.ColumnString
	PublicID  postgres.ColumnString
	UserID    postgres.ColumnString
	TokenHash postgres.ColumnString
	ExpiresAt postgres.ColumnTimestampz
	UsedAt    postgres.ColumnTimestampz
	CreatedAt postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewPasswordResets builds the password_resets binding.
func NewPasswordResets(schema, tablePrefix string) *PasswordResets {
	if schema == "" {
		schema = "public"
	}
	var (
		id        = postgres.StringColumn("id")
		publicID  = postgres.StringColumn("public_id")
		userID    = postgres.StringColumn("user_id")
		tokenHash = postgres.StringColumn("token_hash")
		expiresAt = postgres.TimestampzColumn("expires_at")
		usedAt    = postgres.TimestampzColumn("used_at")
		createdAt = postgres.TimestampzColumn("created_at")
		all       = postgres.ColumnList{id, publicID, userID, tokenHash, expiresAt, usedAt, createdAt}
	)
	return &PasswordResets{
		Table:      postgres.NewTable(schema, tablePrefix+"password_resets", "password_resets", all...),
		ID:         id,
		PublicID:   publicID,
		UserID:     userID,
		TokenHash:  tokenHash,
		ExpiresAt:  expiresAt,
		UsedAt:     usedAt,
		CreatedAt:  createdAt,
		AllColumns: all,
	}
}
