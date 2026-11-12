package entity

import "github.com/go-jet/jet/v2/postgres"

// Approvals is the jet binding for the approvals table.
type Approvals struct {
	postgres.Table

	ID                 postgres.ColumnString
	PublicID           postgres.ColumnString
	OrgID              postgres.ColumnString
	ProjectID          postgres.ColumnString
	AgentID            postgres.ColumnString
	RunID              postgres.ColumnString
	ThreadID           postgres.ColumnString
	Calls              postgres.ColumnString
	Preview            postgres.ColumnString
	Channel            postgres.ColumnString
	RequesterEndUserID postgres.ColumnString
	PromptExternalID   postgres.ColumnString
	RaisedSnapshotHash postgres.ColumnBytea
	Status             postgres.ColumnString
	DecidedByKind      postgres.ColumnString
	DecidedByID        postgres.ColumnString
	DecidedVia         postgres.ColumnString
	DecisionText       postgres.ColumnString
	ExpiresAt          postgres.ColumnTimestampz
	CreatedAt          postgres.ColumnTimestampz
	DecidedAt          postgres.ColumnTimestampz
	PromptAfterSeq     postgres.ColumnInteger

	AllColumns postgres.ColumnList
}

// NewApprovals builds the approvals binding.
func NewApprovals(schema, tablePrefix string) *Approvals {
	if schema == "" {
		schema = "public"
	}
	var (
		id                 = postgres.StringColumn("id")
		publicID           = postgres.StringColumn("public_id")
		orgID              = postgres.StringColumn("org_id")
		projectID          = postgres.StringColumn("project_id")
		agentID            = postgres.StringColumn("agent_id")
		runID              = postgres.StringColumn("run_id")
		threadID           = postgres.StringColumn("thread_id")
		calls              = postgres.StringColumn("calls")
		preview            = postgres.StringColumn("preview")
		channel            = postgres.StringColumn("channel")
		requesterEndUserID = postgres.StringColumn("requester_end_user_id")
		promptExternalID   = postgres.StringColumn("prompt_external_id")
		raisedSnapshotHash = postgres.ByteaColumn("raised_snapshot_hash")
		status             = postgres.StringColumn("status")
		decidedByKind      = postgres.StringColumn("decided_by_kind")
		decidedByID        = postgres.StringColumn("decided_by_id")
		decidedVia         = postgres.StringColumn("decided_via")
		decisionText       = postgres.StringColumn("decision_text")
		expiresAt          = postgres.TimestampzColumn("expires_at")
		createdAt          = postgres.TimestampzColumn("created_at")
		decidedAt          = postgres.TimestampzColumn("decided_at")
		promptAfterSeq     = postgres.IntegerColumn("prompt_after_seq")
		all                = postgres.ColumnList{
			id, publicID, orgID, projectID, agentID, runID, threadID, calls, preview, channel, requesterEndUserID, promptExternalID,
			raisedSnapshotHash, status, decidedByKind, decidedByID, decidedVia, decisionText, expiresAt, createdAt, decidedAt, promptAfterSeq,
		}
	)
	return &Approvals{
		Table:              postgres.NewTable(schema, tablePrefix+"approvals", "approvals", all...),
		ID:                 id,
		PublicID:           publicID,
		OrgID:              orgID,
		ProjectID:          projectID,
		AgentID:            agentID,
		RunID:              runID,
		ThreadID:           threadID,
		Calls:              calls,
		Preview:            preview,
		Channel:            channel,
		RequesterEndUserID: requesterEndUserID,
		PromptExternalID:   promptExternalID,
		RaisedSnapshotHash: raisedSnapshotHash,
		Status:             status,
		DecidedByKind:      decidedByKind,
		DecidedByID:        decidedByID,
		DecidedVia:         decidedVia,
		DecisionText:       decisionText,
		ExpiresAt:          expiresAt,
		CreatedAt:          createdAt,
		DecidedAt:          decidedAt,
		PromptAfterSeq:     promptAfterSeq,
		AllColumns:         all,
	}
}
