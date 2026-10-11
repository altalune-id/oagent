package legal

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Service serves the legal documents and records acceptance of the terms.
type Service struct {
	users TermsAcceptor
}

// NewService builds a Service; users may be nil when acceptance is never asked for.
func NewService(users TermsAcceptor) *Service {
	return &Service{users: users}
}

// Document returns the document for slug.
func (s *Service) Document(slug string) (*Document, error) {
	return BySlug(slug)
}

// CanAccept reports whether the service can record acceptance.
func (s *Service) CanAccept() bool { return s.users != nil }

// Accept records that userID accepted the terms and returns when; the first acceptance is kept.
func (s *Service) Accept(ctx context.Context, userID uuid.UUID) (time.Time, error) {
	u, err := s.users.AcceptTerms(ctx, userID)
	if err != nil {
		return time.Time{}, err
	}
	return *u.TermsAcceptedAt, nil
}
