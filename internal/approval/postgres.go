package approval

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool      db.Pool
	approvals *entity.Approvals
	resolve   string
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	if schema == "" {
		schema = "public"
	}
	return &postgresStore{
		pool:      pool,
		approvals: entity.NewApprovals(schema, tablePrefix),
		// NOTE: RawStatement because go-jet has no builder for a set-returning function in FROM position.
		resolve: `SELECT x.id AS "ref.id", x.org_id AS "ref.org_id", x.project_id AS "ref.project_id", x.agent_id AS "ref.agent_id" FROM ` +
			schema + "." + tablePrefix + "resolve_approval_by_public_id(#publicID) x",
	}
}

var _ Store = (*postgresStore)(nil)

type approvalRow struct {
	ID                 uuid.UUID     `alias:"approvals.id"`
	PublicID           string        `alias:"approvals.public_id"`
	OrgID              uuid.UUID     `alias:"approvals.org_id"`
	ProjectID          uuid.UUID     `alias:"approvals.project_id"`
	AgentID            uuid.UUID     `alias:"approvals.agent_id"`
	RunID              uuid.UUID     `alias:"approvals.run_id"`
	ThreadID           uuid.UUID     `alias:"approvals.thread_id"`
	Calls              string        `alias:"approvals.calls"`
	Preview            string        `alias:"approvals.preview"`
	Channel            string        `alias:"approvals.channel"`
	RequesterEndUserID uuid.NullUUID `alias:"approvals.requester_end_user_id"`
	PromptExternalID   *string       `alias:"approvals.prompt_external_id"`
	PromptAfterSeq     *int64        `alias:"approvals.prompt_after_seq"`
	RaisedSnapshotHash []byte        `alias:"approvals.raised_snapshot_hash"`
	Status             string        `alias:"approvals.status"`
	DecidedByKind      *string       `alias:"approvals.decided_by_kind"`
	DecidedByID        *string       `alias:"approvals.decided_by_id"`
	DecidedVia         *string       `alias:"approvals.decided_via"`
	DecisionText       *string       `alias:"approvals.decision_text"`
	ExpiresAt          time.Time     `alias:"approvals.expires_at"`
	CreatedAt          time.Time     `alias:"approvals.created_at"`
	DecidedAt          *time.Time    `alias:"approvals.decided_at"`
}

func (r *approvalRow) toApproval() (*Approval, error) {
	a := &Approval{
		ID: r.ID, PublicID: r.PublicID, OrgID: r.OrgID, ProjectID: r.ProjectID, AgentID: r.AgentID, RunID: r.RunID, ThreadID: r.ThreadID,
		Preview: r.Preview, Channel: Channel(r.Channel), Requester: r.RequesterEndUserID, PromptExternalID: deref(r.PromptExternalID),
		PromptAfterSeq:     r.PromptAfterSeq,
		RaisedSnapshotHash: r.RaisedSnapshotHash, Status: Status(r.Status), DecidedByKind: DeciderKind(deref(r.DecidedByKind)),
		DecidedByID: deref(r.DecidedByID), DecidedVia: Via(deref(r.DecidedVia)), DecisionText: deref(r.DecisionText),
		ExpiresAt: r.ExpiresAt.UTC(), CreatedAt: r.CreatedAt.UTC(),
	}
	if r.DecidedAt != nil {
		at := r.DecidedAt.UTC()
		a.DecidedAt = &at
	}
	if err := json.Unmarshal([]byte(r.Calls), &a.Calls); err != nil {
		return nil, fmt.Errorf("approval.postgres: calls: %w", err)
	}
	return a, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
