package agent

import (
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool    db.Pool
	agents  *entity.Agents
	resolve string
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	if schema == "" {
		schema = "public"
	}
	return &postgresStore{
		pool: pool, agents: entity.NewAgents(schema, tablePrefix),
		// NOTE: RawStatement because go-jet has no builder for a set-returning function in FROM position.
		resolve: `SELECT x.id AS "agents.id", x.org_id AS "agents.org_id", x.project_id AS "agents.project_id" FROM ` +
			schema + "." + tablePrefix + "resolve_agent_by_public_id(#publicID) x",
	}
}
