package entity

import "github.com/go-jet/jet/v2/postgres"

// Users is the jet binding for the users table.
type Users struct {
	postgres.Table

	ID              postgres.ColumnString
	PublicID        postgres.ColumnString
	Email           postgres.ColumnString
	Name            postgres.ColumnString
	AvatarURL       postgres.ColumnString
	PasswordHash    postgres.ColumnString
	IDPIssuer       postgres.ColumnString
	IDPSubject      postgres.ColumnString
	Locale          postgres.ColumnString
	TermsAcceptedAt postgres.ColumnTimestampz
	CreatedAt       postgres.ColumnTimestampz
	UpdatedAt       postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewUsers builds the users binding.
func NewUsers(schema, tablePrefix string) *Users {
	if schema == "" {
		schema = "public"
	}
	var (
		id              = postgres.StringColumn("id")
		publicID        = postgres.StringColumn("public_id")
		email           = postgres.StringColumn("email")
		name            = postgres.StringColumn("name")
		avatarURL       = postgres.StringColumn("avatar_url")
		passwordHash    = postgres.StringColumn("password_hash")
		idpIssuer       = postgres.StringColumn("idp_issuer")
		idpSubject      = postgres.StringColumn("idp_subject")
		locale          = postgres.StringColumn("locale")
		termsAcceptedAt = postgres.TimestampzColumn("terms_accepted_at")
		createdAt       = postgres.TimestampzColumn("created_at")
		updatedAt       = postgres.TimestampzColumn("updated_at")
		all             = postgres.ColumnList{
			id, publicID, email, name, avatarURL, passwordHash, idpIssuer, idpSubject, locale, termsAcceptedAt, createdAt, updatedAt,
		}
	)
	return &Users{
		Table:           postgres.NewTable(schema, tablePrefix+"users", "users", all...),
		ID:              id,
		PublicID:        publicID,
		Email:           email,
		Name:            name,
		AvatarURL:       avatarURL,
		PasswordHash:    passwordHash,
		IDPIssuer:       idpIssuer,
		IDPSubject:      idpSubject,
		Locale:          locale,
		TermsAcceptedAt: termsAcceptedAt,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		AllColumns:      all,
	}
}
