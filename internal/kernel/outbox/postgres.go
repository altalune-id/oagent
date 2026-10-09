package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool    db.Pool
	entries *entity.OutboxEntries
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	return &postgresStore{pool: pool, entries: entity.NewOutboxEntries(schema, tablePrefix)}
}

func (s *postgresStore) reader(ctx context.Context) qrm.DB {
	if tx, ok := db.CurrentTx(ctx); ok {
		return tx
	}
	return s.pool.R
}

// txAcquire returns the ambient transaction, or begins one and reports that it owns it.
func (s *postgresStore) txAcquire(ctx context.Context) (*sql.Tx, bool, tenant.Context, error) {
	tc, err := tenant.From(ctx)
	if err != nil {
		return nil, false, tenant.Context{}, err
	}
	if tx, ok := db.CurrentTx(ctx); ok {
		return tx, false, tc, nil
	}
	tx, err := s.pool.W.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, tenant.Context{}, fmt.Errorf("outbox.postgres: begin: %w", err)
	}
	return tx, true, tc, nil
}

// endTx commits or rolls back a transaction txAcquire started; it is a no-op for an ambient one.
func (s *postgresStore) endTx(tx *sql.Tx, owned bool, err error) error {
	if !owned {
		return err
	}
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if cerr := tx.Commit(); cerr != nil {
		return fmt.Errorf("outbox.postgres: commit: %w", cerr)
	}
	return nil
}

type pgEntryRow struct {
	ID            uuid.UUID `alias:"outbox_entries.id"`
	EventID       uuid.UUID `alias:"outbox_entries.event_id"`
	OrgID         uuid.UUID `alias:"outbox_entries.org_id"`
	ProjectID     uuid.UUID `alias:"outbox_entries.project_id"`
	Target        string    `alias:"outbox_entries.target"`
	Payload       []byte    `alias:"outbox_entries.payload"`
	Attempt       int32     `alias:"outbox_entries.attempt"`
	NextAttemptAt time.Time `alias:"outbox_entries.next_attempt_at"`
	Status        string    `alias:"outbox_entries.status"`
	LastError     string    `alias:"outbox_entries.last_error"`
	CreatedAt     time.Time `alias:"outbox_entries.created_at"`
}

func (r *pgEntryRow) toEntry() Entry {
	return Entry{
		ID:            r.ID,
		EventID:       r.EventID,
		OrgID:         r.OrgID,
		ProjectID:     r.ProjectID,
		Target:        r.Target,
		Payload:       r.Payload,
		Attempt:       int(r.Attempt),
		NextAttemptAt: r.NextAttemptAt.UTC(),
		Status:        Status(r.Status),
		LastError:     r.LastError,
		CreatedAt:     r.CreatedAt.UTC(),
	}
}
