package user

import (
	"context"

	"github.com/google/uuid"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/password"
)

// Service is the user application service.
type Service struct {
	store      Store
	unexpected apperror.UnexpectedFunc
}

// NewService builds a Service over store.
func NewService(store Store, unexpected apperror.UnexpectedFunc) *Service {
	return &Service{store: store, unexpected: unexpected}
}

// CreateRequest is the input to Create.
type CreateRequest struct {
	Email    string
	Name     string
	Password string
}

// Create validates, hashes the password and persists a new local user.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*User, error) {
	if err := password.Check(req.Password, password.Hints{Email: req.Email, Name: req.Name}); err != nil {
		return nil, err
	}
	u, err := New(req.Email, req.Name)
	if err != nil {
		return nil, err
	}
	if u.PasswordHash, err = password.Hash(req.Password); err != nil {
		return nil, s.unexpected(ctx, "user: hash password", err)
	}
	if err := s.store.Save(ctx, u); err != nil {
		return nil, s.errOrReport(ctx, "user: create", err)
	}
	return u, nil
}

// ByID returns the user with id.
func (s *Service) ByID(ctx context.Context, id uuid.UUID) (*User, error) {
	u, err := s.store.ByID(ctx, id)
	if err != nil {
		return nil, s.errOrReport(ctx, "user: by id", err)
	}
	return u, nil
}

// ByPublicID returns the user with publicID.
func (s *Service) ByPublicID(ctx context.Context, publicID string) (*User, error) {
	u, err := s.store.ByPublicID(ctx, publicID)
	if err != nil {
		return nil, s.errOrReport(ctx, "user: by public id", err)
	}
	return u, nil
}

// ByEmail returns the user with email, compared after normalization.
func (s *Service) ByEmail(ctx context.Context, email string) (*User, error) {
	u, err := s.store.ByEmail(ctx, email)
	if err != nil {
		return nil, s.errOrReport(ctx, "user: by email", err)
	}
	return u, nil
}

// FindByEmail returns the user with the address an operator typed; an unknown address is an UnknownEmailError naming it.
func (s *Service) FindByEmail(ctx context.Context, raw string) (*User, error) {
	email, err := NormalizeEmail(raw)
	if err != nil {
		return nil, err
	}
	u, err := s.ByEmail(ctx, email)
	if IsNotFoundError(err) {
		return nil, &UnknownEmailError{Email: email}
	}
	return u, err
}

// SetPassword replaces the user's password after a policy check.
func (s *Service) SetPassword(ctx context.Context, id uuid.UUID, plain string) error {
	u, err := s.ByID(ctx, id)
	if err != nil {
		return err
	}
	if err := password.Check(plain, password.Hints{Email: u.Email, Name: u.Name}); err != nil {
		return err
	}
	if u.PasswordHash, err = password.Hash(plain); err != nil {
		return s.unexpected(ctx, "user: hash password", err)
	}
	if err := s.store.Save(ctx, u); err != nil {
		return s.errOrReport(ctx, "user: set password", err)
	}
	return nil
}

// SetName replaces the user's display name, trimmed; an empty name is allowed.
func (s *Service) SetName(ctx context.Context, id uuid.UUID, name string) (*User, error) {
	u, err := s.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := u.rename(name); err != nil {
		return nil, err
	}
	saved, err := s.store.SetName(ctx, u.ID, u.Name)
	if err != nil {
		return nil, s.errOrReport(ctx, "user: set name", err)
	}
	return saved, nil
}

// AcceptTerms records that the user accepted the terms and returns the updated user.
func (s *Service) AcceptTerms(ctx context.Context, id uuid.UUID) (*User, error) {
	u, err := s.store.AcceptTerms(ctx, id)
	if err != nil {
		return nil, s.errOrReport(ctx, "user: accept terms", err)
	}
	return u, nil
}

func (s *Service) errOrReport(ctx context.Context, message string, err error) error {
	if IsExpectedError(err) {
		return err
	}
	return s.unexpected(ctx, message, err)
}
