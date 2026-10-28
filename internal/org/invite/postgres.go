package invite

import (
	"errors"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

// NOTE: definers are set-returning functions in FROM position, which go-jet cannot build; the aliases mirror jet's so inviteRow maps.
const inviteFuncSelect = `SELECT i.id AS "invites.id", i.public_id AS "invites.public_id", i.org_id AS "invites.org_id", ` +
	`i.email AS "invites.email", i.role AS "invites.role", i.draft AS "invites.draft", i.invited_by AS "invites.invited_by", ` +
	`i.expires_at AS "invites.expires_at", i.accepted_at AS "invites.accepted_at", i.accepted_by AS "invites.accepted_by", ` +
	`i.revoked_at AS "invites.revoked_at", i.created_at AS "invites.created_at", i.updated_at AS "invites.updated_at" FROM `

var errLockOutsideTx = errors.New("invite.postgres: a row lock needs a transaction")

// NOTE: reads use the writer; a lagging replica could show a used invite as usable.
type postgresStore struct {
	pool        db.Pool
	invites     *entity.Invites
	users       *entity.Users
	byTokenHash string
	byPublicID  string
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	if schema == "" {
		schema = "public"
	}
	return &postgresStore{
		pool: pool, invites: entity.NewInvites(schema, tablePrefix), users: entity.NewUsers(schema, tablePrefix),
		byTokenHash: inviteFuncSelect + schema + "." + tablePrefix + "resolve_invite_by_token_hash(#hash) i",
		byPublicID:  inviteFuncSelect + schema + "." + tablePrefix + "resolve_invite_by_public_id(#publicID) i",
	}
}
