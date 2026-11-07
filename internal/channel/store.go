package channel

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Store persists connections.
type Store interface {
	Reader
	Writer
}

// Reader reads connections; Resolve needs no tenant, the others read inside the bound one.
type Reader interface {
	Resolve(ctx context.Context, publicID string) (Ref, error)
	ByID(ctx context.Context, id uuid.UUID) (*Connection, error)
	List(ctx context.Context, projectID uuid.UUID) ([]*Connection, error)
}

// Writer writes connections.
type Writer interface {
	Insert(ctx context.Context, c *Connection) error
	Update(ctx context.Context, c *Connection) error
	Delete(ctx context.Context, id uuid.UUID) error
	InsertDelivery(ctx context.Context, c *Connection, deliveryID string, at time.Time) (inserted bool, err error)
	RecordResult(ctx context.Context, id uuid.UUID, result string, at time.Time) error
	RecordBadSignature(ctx context.Context, id uuid.UUID, at time.Time) error
	PruneDeliveries(ctx context.Context, before time.Time) error
}
