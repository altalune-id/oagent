package entity

import "github.com/go-jet/jet/v2/postgres"

// ThreadSnapshots is the jet binding for the thread_snapshots table.
type ThreadSnapshots struct {
	postgres.Table

	ThreadID       postgres.ColumnString
	OrgID          postgres.ColumnString
	ProjectID      postgres.ColumnString
	AgentVersionID postgres.ColumnString
	SDKVersion     postgres.ColumnString
	Blob           postgres.ColumnBytea
	UpdatedAt      postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewThreadSnapshots builds the thread_snapshots binding.
func NewThreadSnapshots(schema, tablePrefix string) *ThreadSnapshots {
	if schema == "" {
		schema = "public"
	}
	var (
		threadID       = postgres.StringColumn("thread_id")
		orgID          = postgres.StringColumn("org_id")
		projectID      = postgres.StringColumn("project_id")
		agentVersionID = postgres.StringColumn("agent_version_id")
		sdkVersion     = postgres.StringColumn("sdk_version")
		blob           = postgres.ByteaColumn("blob")
		updatedAt      = postgres.TimestampzColumn("updated_at")
		all            = postgres.ColumnList{threadID, orgID, projectID, agentVersionID, sdkVersion, blob, updatedAt}
	)
	return &ThreadSnapshots{
		Table:          postgres.NewTable(schema, tablePrefix+"thread_snapshots", "thread_snapshots", all...),
		ThreadID:       threadID,
		OrgID:          orgID,
		ProjectID:      projectID,
		AgentVersionID: agentVersionID,
		SDKVersion:     sdkVersion,
		Blob:           blob,
		UpdatedAt:      updatedAt,
		AllColumns:     all,
	}
}
