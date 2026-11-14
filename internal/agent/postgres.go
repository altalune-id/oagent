package agent

import (
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool     db.Pool
	agents   *entity.Agents
	versions *entity.AgentVersions
	resolve  string
	// NOTE: RawStatement because go-jet has no builder for the jsonb @> containment operator.
	providerUse string
	sttUse      string
	serverUse   string
	sourceUse   string
	projectUse  string
	storeUse    string
	memoryUse   string
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	if schema == "" {
		schema = "public"
	}
	return &postgresStore{
		pool: pool, agents: entity.NewAgents(schema, tablePrefix), versions: entity.NewAgentVersions(schema, tablePrefix),
		// NOTE: RawStatement because go-jet has no builder for a set-returning function in FROM position.
		resolve: `SELECT x.id AS "agents.id", x.org_id AS "agents.org_id", x.project_id AS "agents.project_id" FROM ` +
			schema + "." + tablePrefix + "resolve_agent_by_public_id(#publicID) x",
		providerUse: latestVersions(schema, tablePrefix, "jsonb_build_object('provider_id', #id::text)"),
		sttUse: latestVersions(schema, tablePrefix,
			"jsonb_build_object('transcription_model', jsonb_build_object('provider_id', #id::text))"),
		sourceUse:  latestVersions(schema, tablePrefix, "jsonb_build_object('knowledge_source_ids', jsonb_build_array(#id::text))"),
		projectUse: projectVersions(schema, tablePrefix, "knowledge_source_ids"),
		storeUse:   latestVersions(schema, tablePrefix, "jsonb_build_object('memory_store_id', #id::text)"),
		memoryUse:  projectVersions(schema, tablePrefix, "memory_store_id"),
		serverUse: latestVersions(schema, tablePrefix,
			"jsonb_build_object('mcp_servers', jsonb_build_array(jsonb_build_object('server_id', #id::text)))"),
	}
}

func projectVersions(schema, tablePrefix, key string) string {
	return `SELECT a.id AS "agents.id", a.public_id AS "agents.public_id", a.name AS "agents.name", ` +
		`v.config AS "agent_versions.config" FROM ` +
		schema + "." + tablePrefix + `agent_versions v JOIN ` + schema + "." + tablePrefix +
		`agents a ON a.id = v.agent_id AND a.published_version = v.version ` +
		`WHERE v.org_id = #org AND a.project_id = #project AND v.kind = 'published' AND v.config ? '` + key + `'`
}

func latestVersions(schema, tablePrefix, contains string) string {
	return `SELECT v.config AS "agent_versions.config" FROM ` + schema + "." + tablePrefix + `agent_versions v JOIN ` +
		schema + "." + tablePrefix + `agents a ON a.id = v.agent_id AND a.published_version = v.version ` +
		`WHERE v.org_id = #org AND v.kind = 'published' AND v.config @> ` + contains
}
