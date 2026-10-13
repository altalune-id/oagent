package legal

import (
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
