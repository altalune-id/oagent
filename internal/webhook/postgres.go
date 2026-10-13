package webhook

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/go-jet/jet/v2/qrm"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool      db.Pool
	endpoints *entity.WebhookEndpoints
	attempts  *entity.WebhookDeliveries
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	return &postgresStore{
		pool:      pool,
		endpoints: entity.NewWebhookEndpoints(schema, tablePrefix),
		attempts:  entity.NewWebhookDeliveries(schema, tablePrefix),
	}
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
		return nil, false, tenant.Context{}, fmt.Errorf("webhook.postgres: begin: %w", err)
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
		return fmt.Errorf("webhook.postgres: commit: %w", cerr)
	}
	return nil
}
