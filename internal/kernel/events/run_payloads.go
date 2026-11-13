package events

// RunUsageV1 is the token usage of one run, by token class.
type RunUsageV1 struct {
	InputTokens           int64 `json:"input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	CacheReadInputTokens  int64 `json:"cache_read_input_tokens"`
	CacheWriteInputTokens int64 `json:"cache_write_input_tokens"`
	EmbeddingTokens       int64 `json:"embedding_tokens"`
	Estimated             bool  `json:"estimated"`
}

// RunSettledV1 is the shared run event shape; ids are public ids, cost is null when unpriced. SECURITY: no text or phone.
type RunSettledV1 struct {
	RunID      string     `json:"run_id"`
	ThreadID   string     `json:"thread_id"`
	AgentID    string     `json:"agent_id"`
	Status     string     `json:"status"`
	Usage      RunUsageV1 `json:"usage"`
	CostMicros *int64     `json:"cost_micros"`
}

// RunCompletedV1 is the payload for RunCompleted.
type RunCompletedV1 RunSettledV1

// RunFailedV1 is the payload for RunFailed.
type RunFailedV1 RunSettledV1
