package org

import (
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

// NOTE: definers are set-returning functions in FROM position, which go-jet cannot build; the aliases mirror jet's so orgRow maps.
const orgFuncSelect = `SELECT o.id AS "orgs.id", o.public_id AS "orgs.public_id", o.slug AS "orgs.slug", o.name AS "orgs.name", ` +
	`o.created_by AS "orgs.created_by", o.created_at AS "orgs.created_at", o.updated_at AS "orgs.updated_at", ` +
	`o.is_system AS "orgs.is_system" FROM `

type postgresStore struct {
	pool                 db.Pool
	orgs                 *entity.Orgs
	resolveBySlugStmt    string
	resolveByPublicStmt  string
	resolveSystemOrgStmt string
	listIDsStmt          string
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	if schema == "" {
		schema = "public"
	}
	fn := schema + "." + tablePrefix
	return &postgresStore{
		pool:                 pool,
		orgs:                 entity.NewOrgs(schema, tablePrefix),
		resolveBySlugStmt:    orgFuncSelect + fn + "resolve_org_by_slug(#slug) AS o",
		resolveByPublicStmt:  orgFuncSelect + fn + "resolve_org_by_public_id(#publicID) AS o",
		resolveSystemOrgStmt: orgFuncSelect + fn + "resolve_system_org() AS o",
		listIDsStmt:          `SELECT o.id AS "orgs.id" FROM ` + fn + "list_org_ids() AS o ORDER BY o.created_at ASC, o.id ASC",
	}
}
