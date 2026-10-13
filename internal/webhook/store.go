package webhook

import (
	"context"

	"github.com/google/uuid"
)

// Store is the driven port.
type Store interface {
	// Save upserts e, writing its secrets only on insert, and returns a *NotFoundError when the id belongs to another org.
	Save(ctx context.Context, e *Endpoint) error
	// SaveSecrets replaces the endpoint's secrets when the stored pair still equals expected.
	SaveSecrets(ctx context.Context, id uuid.UUID, expected, next SealedSecrets) error
	// ByID returns the endpoint in the caller's org, or a *NotFoundError.
	ByID(ctx context.Context, id uuid.UUID) (*Endpoint, error)
	// ByPublicID returns the endpoint with publicID in the caller's org, or a *NotFoundError.
	ByPublicID(ctx context.Context, publicID string) (*Endpoint, error)
	// List returns the project's endpoints, newest first.
	List(ctx context.Context, orgID, projectID uuid.UUID) ([]*Endpoint, error)
	// ListByOrg returns the endpoints of every project in orgID, oldest first.
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]*Endpoint, error)
	// Delete removes the endpoint and its attempts, or returns a *NotFoundError.
	Delete(ctx context.Context, id uuid.UUID) error
	// SaveAttempt appends a, returning an *InvalidAttemptError on a scope mismatch or a *NotFoundError when the endpoint is gone.
	SaveAttempt(ctx context.Context, a Attempt) error
	// ListAttempts returns the attempts of one delivery to one endpoint, newest first.
	ListAttempts(ctx context.Context, endpointID, deliveryID uuid.UUID) ([]Attempt, error)
}
