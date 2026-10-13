package entity

import "github.com/go-jet/jet/v2/postgres"

// TermsAcceptances is the jet binding for the terms_acceptances table.
type TermsAcceptances struct {
	postgres.Table

	ID                postgres.ColumnString
	PublicID          postgres.ColumnString
	UserID            postgres.ColumnString
	TermsDocumentID   postgres.ColumnString
	PrivacyDocumentID postgres.ColumnString
	Source            postgres.ColumnString
	AcceptedAt        postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewTermsAcceptances builds the terms_acceptances binding.
func NewTermsAcceptances(schema, tablePrefix string) *TermsAcceptances {
	if schema == "" {
		schema = "public"
	}
	var (
		id                = postgres.StringColumn("id")
		publicID          = postgres.StringColumn("public_id")
		userID            = postgres.StringColumn("user_id")
		termsDocumentID   = postgres.StringColumn("terms_document_id")
		privacyDocumentID = postgres.StringColumn("privacy_document_id")
		source            = postgres.StringColumn("source")
		acceptedAt        = postgres.TimestampzColumn("accepted_at")
		all               = postgres.ColumnList{id, publicID, userID, termsDocumentID, privacyDocumentID, source, acceptedAt}
	)
	return &TermsAcceptances{
		Table:             postgres.NewTable(schema, tablePrefix+"terms_acceptances", "terms_acceptances", all...),
		ID:                id,
		PublicID:          publicID,
		UserID:            userID,
		TermsDocumentID:   termsDocumentID,
		PrivacyDocumentID: privacyDocumentID,
		Source:            source,
		AcceptedAt:        acceptedAt,
		AllColumns:        all,
	}
}
