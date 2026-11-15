package webtool

import (
	"context"

	"github.com/google/uuid"
)

// Ref names a secret's row and the org and project that own it.
type Ref struct {
	ID, OrgID, ProjectID uuid.UUID
}

// Store persists search providers and HTTP secrets.
type Store interface {
	Reader
	Writer
}

// Reader reads the org on ctx's providers and secrets; implementations may use a read replica.
type Reader interface {
	Provider(ctx context.Context, projectID uuid.UUID) (*Provider, error)
	Secrets(ctx context.Context, projectID uuid.UUID) ([]*HTTPSecret, error)
	SecretsForHosts(ctx context.Context, projectID uuid.UUID, hosts []string) ([]*HTTPSecret, error)
	SecretCount(ctx context.Context, projectID uuid.UUID, host, header string) (total int, exists bool, err error)
	ResolveSecret(ctx context.Context, publicID string) (Ref, error)
}

// Writer writes the org on ctx's providers and secrets.
type Writer interface {
	SaveProvider(ctx context.Context, p *Provider) error
	DeleteProvider(ctx context.Context, projectID uuid.UUID) error
	SaveSecret(ctx context.Context, s *HTTPSecret) (*HTTPSecret, error)
	DeleteSecret(ctx context.Context, id uuid.UUID) error
}
