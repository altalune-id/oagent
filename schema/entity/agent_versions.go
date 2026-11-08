package entity

import "github.com/go-jet/jet/v2/postgres"

// AgentVersions is the jet binding for the agent_versions table.
type AgentVersions struct {
	postgres.Table

	ID        postgres.ColumnString
	PublicID  postgres.ColumnString
	OrgID     postgres.ColumnString
	ProjectID postgres.ColumnString
	AgentID   postgres.ColumnString
	Version   postgres.ColumnInteger
	// Config is JSONB, bound as a string.
	Config    postgres.ColumnString
	CreatedBy postgres.ColumnString
	CreatedAt postgres.ColumnTimestampz
	// Kind is published or draft; a draft has no Version and has a ConfigHash.
	Kind       postgres.ColumnString
	ConfigHash postgres.ColumnBytea

	AllColumns postgres.ColumnList
}

// NewAgentVersions builds the agent_versions binding.
func NewAgentVersions(schema, tablePrefix string) *AgentVersions {
	if schema == "" {
		schema = "public"
	}
	var (
		id         = postgres.StringColumn("id")
		publicID   = postgres.StringColumn("public_id")
		orgID      = postgres.StringColumn("org_id")
		projectID  = postgres.StringColumn("project_id")
		agentID    = postgres.StringColumn("agent_id")
		version    = postgres.IntegerColumn("version")
		config     = postgres.StringColumn("config")
		createdBy  = postgres.StringColumn("created_by")
		createdAt  = postgres.TimestampzColumn("created_at")
		kind       = postgres.StringColumn("kind")
		configHash = postgres.ByteaColumn("config_hash")
		all        = postgres.ColumnList{id, publicID, orgID, projectID, agentID, version, config, createdBy, createdAt, kind, configHash}
	)
	return &AgentVersions{
		Table:      postgres.NewTable(schema, tablePrefix+"agent_versions", "agent_versions", all...),
		ID:         id,
		PublicID:   publicID,
		OrgID:      orgID,
		ProjectID:  projectID,
		AgentID:    agentID,
		Version:    version,
		Config:     config,
		CreatedBy:  createdBy,
		CreatedAt:  createdAt,
		Kind:       kind,
		ConfigHash: configHash,
		AllColumns: all,
	}
}
