package invite

import (
	"errors"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

var errLockOutsideTx = errors.New("invite.postgres: a row lock needs a transaction")

// NOTE: reads use the writer; a lagging replica could show a used invite as usable.
type postgresStore struct {
	pool    db.Pool
	invites *entity.Invites
	users   *entity.Users
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	return &postgresStore{pool: pool, invites: entity.NewInvites(schema, tablePrefix), users: entity.NewUsers(schema, tablePrefix)}
}
