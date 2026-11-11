package outbox

import (
	"context"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool    db.Pool
	entries *entity.OutboxEntries
	wake    func()
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	return &postgresStore{pool: pool, entries: entity.NewOutboxEntries(schema, tablePrefix)}
}

// OnEnqueue sets fn to run after each enqueue or requeue commits; call it before the store is shared.
func (s *postgresStore) OnEnqueue(fn func()) { s.wake = fn }

func (s *postgresStore) wakeAfterCommit(ctx context.Context) {
	if s.wake != nil {
		db.AfterCommit(ctx, s.wake)
	}
}
