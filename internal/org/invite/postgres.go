package invite

import (
	"context"
	"errors"

	"github.com/go-jet/jet/v2/qrm"

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

func (s *postgresStore) writer(ctx context.Context) qrm.DB {
	if tx, ok := db.CurrentTx(ctx); ok {
		return tx
	}
	return s.pool.W
}
