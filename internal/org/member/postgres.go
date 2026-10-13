package member

import (
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

// NOTE: every read goes to the writer: memberships decide authorization, so a lagging replica must never grant or deny.
type postgresStore struct {
	pool        db.Pool
	memberships *entity.Memberships
	users       *entity.Users
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	return &postgresStore{pool: pool, memberships: entity.NewMemberships(schema, tablePrefix), users: entity.NewUsers(schema, tablePrefix)}
}
