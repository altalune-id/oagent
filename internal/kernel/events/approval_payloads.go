package events

// ApprovalV1 is an approval as its events carry it; ids are public ids, with no arguments, decision text or phone.
type ApprovalV1 struct {
	ID        string   `json:"id"`
	ProjectID string   `json:"project_id"`
	AgentID   string   `json:"agent_id"`
	ThreadID  string   `json:"thread_id"`
	RunID     string   `json:"run_id"`
	Tools     []string `json:"tools"`
	Status    string   `json:"status"`
	Channel   string   `json:"channel"`
}

// ApprovalRequestedV1 is the payload for ApprovalRequested.
type ApprovalRequestedV1 ApprovalV1

// ApprovalDecidedV1 is the payload for ApprovalDecided.
type ApprovalDecidedV1 ApprovalV1
