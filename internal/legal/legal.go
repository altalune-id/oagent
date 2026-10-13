// Package legal ships the Terms of Service and Privacy Policy and records that a person accepted them.
package legal

import (
	"time"

	"github.com/google/uuid"
)

// The document slugs, also their URL paths.
const (
	TermsSlug   = "terms"
	PrivacySlug = "privacy"
)

// Document is a rendered legal document.
type Document struct {
	Slug      string
	Title     string
	Version   string
	Source    string
	SHA256    string
	HTML      string
	UpdatedAt time.Time
}

type snapshotRow struct {
	ID          uuid.UUID `alias:"legal_documents.id"`
	Slug        string    `alias:"legal_documents.slug"`
	Version     string    `alias:"legal_documents.version"`
	Title       string    `alias:"legal_documents.title"`
	Body        string    `alias:"legal_documents.body"`
	SHA256      string    `alias:"legal_documents.sha256"`
	PublishedAt time.Time `alias:"legal_documents.published_at"`
}
