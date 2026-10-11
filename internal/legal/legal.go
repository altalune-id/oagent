// Package legal ships the Terms of Service and Privacy Policy.
package legal

import "time"

// The document slugs, also their URL paths.
const (
	TermsSlug   = "terms"
	PrivacySlug = "privacy"
)

// Document is a rendered legal document.
type Document struct {
	Slug      string
	Title     string
	HTML      string
	UpdatedAt time.Time
}
