package user

import (
	"context"

	"github.com/google/uuid"
)

// Store persists users.
type Store interface {
	Reader
	Writer
}

// Reader reads users.
type Reader interface {
	ByID(ctx context.Context, id uuid.UUID) (*User, error)
	ByEmail(ctx context.Context, email string) (*User, error)
	ByPublicID(ctx context.Context, publicID string) (*User, error)
	// ByIDP loads the user linked to the identity issuer and subject name.
	ByIDP(ctx context.Context, issuer, subject string) (*User, error)
}

// Writer writes users.
type Writer interface {
	Save(ctx context.Context, u *User) error
	// AcceptTerms sets the user's terms_accepted_at to the database time and returns the stored user.
	AcceptTerms(ctx context.Context, id uuid.UUID) (*User, error)
	// SetName sets only the user's name and returns the stored user.
	SetName(ctx context.Context, id uuid.UUID, name string) (*User, error)
	// SetPasswordHash sets only the user's password hash.
	SetPasswordHash(ctx context.Context, id uuid.UUID, hash string) error
	// LinkIDP links the user to issuer and subject only while it has no link, and reports whether it did.
	LinkIDP(ctx context.Context, id uuid.UUID, issuer, subject string) (bool, error)
	// Lock locks the user's row until the unit of work on ctx ends.
	Lock(ctx context.Context, id uuid.UUID) error
}
