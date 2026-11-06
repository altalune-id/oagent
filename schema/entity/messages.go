package entity

import "github.com/go-jet/jet/v2/postgres"

// Messages is the jet binding for the messages table.
type Messages struct {
	postgres.Table

	ID                postgres.ColumnString
	OrgID             postgres.ColumnString
	ProjectID         postgres.ColumnString
	ThreadID          postgres.ColumnString
	Seq               postgres.ColumnInteger
	Role              postgres.ColumnString
	Body              postgres.ColumnString
	SpeakerID         postgres.ColumnString
	Status            postgres.ColumnString
	ExternalMessageID postgres.ColumnString
	RunID             postgres.ColumnString
	ToolCallID        postgres.ColumnString
	CreatedAt         postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewMessages builds the messages binding.
func NewMessages(schema, tablePrefix string) *Messages {
	if schema == "" {
		schema = "public"
	}
	var (
		id                = postgres.StringColumn("id")
		orgID             = postgres.StringColumn("org_id")
		projectID         = postgres.StringColumn("project_id")
		threadID          = postgres.StringColumn("thread_id")
		seq               = postgres.IntegerColumn("seq")
		role              = postgres.StringColumn("role")
		body              = postgres.StringColumn("body")
		speakerID         = postgres.StringColumn("speaker_id")
		status            = postgres.StringColumn("status")
		externalMessageID = postgres.StringColumn("external_message_id")
		runID             = postgres.StringColumn("run_id")
		toolCallID        = postgres.StringColumn("tool_call_id")
		createdAt         = postgres.TimestampzColumn("created_at")
		all               = postgres.ColumnList{
			id, orgID, projectID, threadID, seq, role, body, speakerID, status, externalMessageID, runID, toolCallID,
			createdAt,
		}
	)
	return &Messages{
		Table:             postgres.NewTable(schema, tablePrefix+"messages", "messages", all...),
		ID:                id,
		OrgID:             orgID,
		ProjectID:         projectID,
		ThreadID:          threadID,
		Seq:               seq,
		Role:              role,
		Body:              body,
		SpeakerID:         speakerID,
		Status:            status,
		ExternalMessageID: externalMessageID,
		RunID:             runID,
		ToolCallID:        toolCallID,
		CreatedAt:         createdAt,
		AllColumns:        all,
	}
}
