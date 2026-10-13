package legal

import (
	"context"

	"github.com/go-jet/jet/v2/qrm"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool        db.Pool
	documents   *entity.LegalDocuments
	acceptances *entity.TermsAcceptances
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	return &postgresStore{
		pool: pool, documents: entity.NewLegalDocuments(schema, tablePrefix), acceptances: entity.NewTermsAcceptances(schema, tablePrefix),
	}
}

func (s *postgresStore) writer(ctx context.Context) qrm.DB {
	if tx, ok := db.CurrentTx(ctx); ok {
		return tx
	}
	return s.pool.W
}
