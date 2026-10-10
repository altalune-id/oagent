package member

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
)

// Service is the membership application service.
type Service struct {
	store      Store
	unexpected apperror.UnexpectedFunc
}

// NewService builds a Service over store.
func NewService(store Store, unexpected apperror.UnexpectedFunc) *Service {
	return &Service{store: store, unexpected: unexpected}
}

// Add makes userID a member of orgID with role.
func (s *Service) Add(ctx context.Context, orgID, userID uuid.UUID, role Role) (*Membership, error) {
	m, err := New(orgID, userID, role)
	if err != nil {
		return nil, err
	}
	if err := s.store.Add(ctx, m); err != nil {
		return nil, s.errOrReport(ctx, "member: add", err)
	}
	return m, nil
}

// Find returns the membership of userID in orgID.
func (s *Service) Find(ctx context.Context, orgID, userID uuid.UUID) (*Membership, error) {
	m, err := s.store.Find(ctx, orgID, userID)
	if err != nil {
		return nil, s.errOrReport(ctx, "member: find", err)
	}
	return m, nil
}

// ListByUser returns userID's memberships, oldest first.
func (s *Service) ListByUser(ctx context.Context, userID uuid.UUID) ([]*Membership, error) {
	ms, err := s.store.ListByUser(ctx, userID)
	if err != nil {
		return nil, s.errOrReport(ctx, "member: list by user", err)
	}
	return ms, nil
}

// CountOwners returns how many owners orgID has.
func (s *Service) CountOwners(ctx context.Context, orgID uuid.UUID) (int, error) {
	n, err := s.store.CountOwners(ctx, orgID)
	if err != nil {
		return 0, s.errOrReport(ctx, "member: count owners", err)
	}
	return n, nil
}

func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if IsExpectedError(err) {
		return err
	}
	return s.unexpected(ctx, message, err)
}
