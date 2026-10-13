package webhook

import (
	"altalune.id/oagent/internal/kernel/db"
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
