package agent

import (
	"context"

	"github.com/go-jet/jet/v2/qrm"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool   db.Pool
	agents *entity.Agents
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	return &postgresStore{pool: pool, agents: entity.NewAgents(schema, tablePrefix)}
}

func (s *postgresStore) reader(ctx context.Context) qrm.DB {
	if tx, ok := db.CurrentTx(ctx); ok {
		return tx
	}
	return s.pool.R
}

func (s *postgresStore) writer(ctx context.Context) qrm.DB {
	if tx, ok := db.CurrentTx(ctx); ok {
		return tx
	}
	return s.pool.W
}
