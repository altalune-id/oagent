package legal

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Service serves the legal documents and records acceptance of the terms.
type Service struct {
	users    TermsAcceptor
	external *time.Time
}

// NewService builds a Service; users may be nil when acceptance is never asked for.
func NewService(users TermsAcceptor) *Service {
	return &Service{users: users}
}

// WithExternalTerms says the terms live at an external URL, last changed at updated; a zero updated never asks again.
func (s *Service) WithExternalTerms(updated time.Time) *Service {
	s.external = &updated
	return s
}

// TermsUpdatedAt is when the terms or the privacy policy last changed.
func (s *Service) TermsUpdatedAt() time.Time {
	if s.external != nil {
		return *s.external
	}
	m, err := docs()
	if err != nil {
		return time.Time{}
	}
	return latest(m)
}

// NeedsAcceptance reports whether a person who accepted the terms at acceptedAt must accept them again.
func (s *Service) NeedsAcceptance(acceptedAt time.Time) bool {
	return needsAcceptance(acceptedAt, s.TermsUpdatedAt())
}

func needsAcceptance(acceptedAt, termsUpdated time.Time) bool {
	return acceptedAt.IsZero() || acceptedAt.Before(termsUpdated)
}

// Document returns the document for slug.
func (s *Service) Document(slug string) (*Document, error) {
	return BySlug(slug)
}

// CanAccept reports whether the service can record acceptance.
func (s *Service) CanAccept() bool { return s.users != nil }

// Accept records that userID accepted the terms and returns when.
func (s *Service) Accept(ctx context.Context, userID uuid.UUID) (time.Time, error) {
	u, err := s.users.AcceptTerms(ctx, userID)
	if err != nil {
		return time.Time{}, err
	}
	return *u.TermsAcceptedAt, nil
}
