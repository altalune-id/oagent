package enduser

import (
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool       db.Pool
	users      *entity.EndUsers
	identities *entity.EndUserIdentities
	resolve    string
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	if schema == "" {
		schema = "public"
	}
	return &postgresStore{
		pool:       pool,
		users:      entity.NewEndUsers(schema, tablePrefix),
		identities: entity.NewEndUserIdentities(schema, tablePrefix),
		// NOTE: RawStatement because go-jet has no builder for a set-returning function in FROM position.
		resolve: `SELECT x.id AS "ref.id", x.org_id AS "ref.org_id", x.project_id AS "ref.project_id" FROM ` +
			schema + "." + tablePrefix + "resolve_end_user_by_public_id(#publicID) x",
	}
}

type endUserRow struct {
	ID          uuid.UUID `alias:"end_users.id"`
	PublicID    string    `alias:"end_users.public_id"`
	OrgID       uuid.UUID `alias:"end_users.org_id"`
	ProjectID   uuid.UUID `alias:"end_users.project_id"`
	DisplayName *string   `alias:"end_users.display_name"`
	CreatedAt   time.Time `alias:"end_users.created_at"`
	UpdatedAt   time.Time `alias:"end_users.updated_at"`
}

func (r *endUserRow) toEndUser() *EndUser {
	e := &EndUser{
		ID: r.ID, PublicID: r.PublicID, OrgID: r.OrgID, ProjectID: r.ProjectID,
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if r.DisplayName != nil {
		e.DisplayName = *r.DisplayName
	}
	return e
}

type identityRow struct {
	Kind      string    `alias:"end_user_identities.kind"`
	Value     string    `alias:"end_user_identities.value"`
	EndUserID uuid.UUID `alias:"end_user_identities.end_user_id"`
}
