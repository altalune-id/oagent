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
	}
}

type endUserRow struct {
	ID        uuid.UUID `alias:"end_users.id"`
	PublicID  string    `alias:"end_users.public_id"`
	OrgID     uuid.UUID `alias:"end_users.org_id"`
	ProjectID uuid.UUID `alias:"end_users.project_id"`
	CreatedAt time.Time `alias:"end_users.created_at"`
	UpdatedAt time.Time `alias:"end_users.updated_at"`
}

func (r *endUserRow) toEndUser() *EndUser {
	return &EndUser{
		ID: r.ID, PublicID: r.PublicID, OrgID: r.OrgID, ProjectID: r.ProjectID,
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

type identityRow struct {
	Kind      string    `alias:"end_user_identities.kind"`
	Value     string    `alias:"end_user_identities.value"`
	EndUserID uuid.UUID `alias:"end_user_identities.end_user_id"`
}
