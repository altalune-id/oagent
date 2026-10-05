package org

import (
	"context"

	"github.com/go-jet/jet/v2/qrm"

	"altalune.id/auth-server/internal/kernel/db"
	"altalune.id/auth-server/schema/entity"
)

type postgresStore struct {
	pool db.Pool
	orgs *entity.Orgs
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	return &postgresStore{pool: pool, orgs: entity.NewOrgs(schema, tablePrefix)}
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
