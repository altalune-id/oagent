package entity

import "github.com/go-jet/jet/v2/postgres"

// LegalDocuments is the jet binding for the legal_documents table.
type LegalDocuments struct {
	postgres.Table

	ID          postgres.ColumnString
	Slug        postgres.ColumnString
	Version     postgres.ColumnString
	Title       postgres.ColumnString
	Body        postgres.ColumnString
	Sha256      postgres.ColumnString
	PublishedAt postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewLegalDocuments builds the legal_documents binding.
func NewLegalDocuments(schema, tablePrefix string) *LegalDocuments {
	if schema == "" {
		schema = "public"
	}
	var (
		id          = postgres.StringColumn("id")
		slug        = postgres.StringColumn("slug")
		version     = postgres.StringColumn("version")
		title       = postgres.StringColumn("title")
		body        = postgres.StringColumn("body")
		sha256      = postgres.StringColumn("sha256")
		publishedAt = postgres.TimestampzColumn("published_at")
		all         = postgres.ColumnList{id, slug, version, title, body, sha256, publishedAt}
	)
	return &LegalDocuments{
		Table:       postgres.NewTable(schema, tablePrefix+"legal_documents", "legal_documents", all...),
		ID:          id,
		Slug:        slug,
		Version:     version,
		Title:       title,
		Body:        body,
		Sha256:      sha256,
		PublishedAt: publishedAt,
		AllColumns:  all,
	}
}
