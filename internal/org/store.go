package org

import (
	"context"

	"github.com/google/uuid"
)

// Store persists Org aggregates.
type Store interface {
	Reader
	Writer
}

// Reader reads Org aggregates; implementations may use a read replica.
type Reader interface {
	ByID(ctx context.Context, id uuid.UUID) (*Org, error)
	BySlug(ctx context.Context, slug string) (*Org, error)
	ByPublicID(ctx context.Context, publicID string) (*Org, error)
	List(ctx context.Context) ([]*Org, error)
	System(ctx context.Context) (*Org, error)
}

// Writer writes Org aggregates.
type Writer interface {
	Save(ctx context.Context, o *Org) error
}
