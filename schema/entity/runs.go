package entity

import "github.com/go-jet/jet/v2/postgres"

// Runs is the jet binding for the runs table.
type Runs struct {
	postgres.Table

	ID                    postgres.ColumnString
	PublicID              postgres.ColumnString
	OrgID                 postgres.ColumnString
	ProjectID             postgres.ColumnString
	ThreadID              postgres.ColumnString
	AgentVersionID        postgres.ColumnString
	Status                postgres.ColumnString
	IdempotencyKey        postgres.ColumnString
	Attempt               postgres.ColumnInteger
	InputTokens           postgres.ColumnInteger
	OutputTokens          postgres.ColumnInteger
	CacheReadTokens       postgres.ColumnInteger
	CacheWriteTokens      postgres.ColumnInteger
	UsageEstimated        postgres.ColumnBool
	PriceInputMicros      postgres.ColumnInteger
	PriceOutputMicros     postgres.ColumnInteger
	PriceCacheReadMicros  postgres.ColumnInteger
	PriceCacheWriteMicros postgres.ColumnInteger
	CostMicros            postgres.ColumnInteger
	LatencyMS             postgres.ColumnInteger
	ErrorCode             postgres.ColumnString
	StartedAt             postgres.ColumnTimestampz
	EndedAt               postgres.ColumnTimestampz
	CreatedAt             postgres.ColumnTimestampz
	UpdatedAt             postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewRuns builds the runs binding.
func NewRuns(schema, tablePrefix string) *Runs {
	if schema == "" {
		schema = "public"
	}
	var (
		id                    = postgres.StringColumn("id")
		publicID              = postgres.StringColumn("public_id")
		orgID                 = postgres.StringColumn("org_id")
		projectID             = postgres.StringColumn("project_id")
		threadID              = postgres.StringColumn("thread_id")
		agentVersionID        = postgres.StringColumn("agent_version_id")
		status                = postgres.StringColumn("status")
		idempotencyKey        = postgres.StringColumn("idempotency_key")
		attempt               = postgres.IntegerColumn("attempt")
		inputTokens           = postgres.IntegerColumn("input_tokens")
		outputTokens          = postgres.IntegerColumn("output_tokens")
		cacheReadTokens       = postgres.IntegerColumn("cache_read_tokens")
		cacheWriteTokens      = postgres.IntegerColumn("cache_write_tokens")
		usageEstimated        = postgres.BoolColumn("usage_estimated")
		priceInputMicros      = postgres.IntegerColumn("price_input_micros")
		priceOutputMicros     = postgres.IntegerColumn("price_output_micros")
		priceCacheReadMicros  = postgres.IntegerColumn("price_cache_read_micros")
		priceCacheWriteMicros = postgres.IntegerColumn("price_cache_write_micros")
		costMicros            = postgres.IntegerColumn("cost_micros")
		latencyMS             = postgres.IntegerColumn("latency_ms")
		errorCode             = postgres.StringColumn("error_code")
		startedAt             = postgres.TimestampzColumn("started_at")
		endedAt               = postgres.TimestampzColumn("ended_at")
		createdAt             = postgres.TimestampzColumn("created_at")
		updatedAt             = postgres.TimestampzColumn("updated_at")
		all                   = postgres.ColumnList{
			id, publicID, orgID, projectID, threadID, agentVersionID, status, idempotencyKey, attempt, inputTokens,
			outputTokens, cacheReadTokens, cacheWriteTokens, usageEstimated, priceInputMicros, priceOutputMicros,
			priceCacheReadMicros, priceCacheWriteMicros, costMicros, latencyMS, errorCode, startedAt, endedAt,
			createdAt, updatedAt,
		}
	)
	return &Runs{
		Table:                 postgres.NewTable(schema, tablePrefix+"runs", "runs", all...),
		ID:                    id,
		PublicID:              publicID,
		OrgID:                 orgID,
		ProjectID:             projectID,
		ThreadID:              threadID,
		AgentVersionID:        agentVersionID,
		Status:                status,
		IdempotencyKey:        idempotencyKey,
		Attempt:               attempt,
		InputTokens:           inputTokens,
		OutputTokens:          outputTokens,
		CacheReadTokens:       cacheReadTokens,
		CacheWriteTokens:      cacheWriteTokens,
		UsageEstimated:        usageEstimated,
		PriceInputMicros:      priceInputMicros,
		PriceOutputMicros:     priceOutputMicros,
		PriceCacheReadMicros:  priceCacheReadMicros,
		PriceCacheWriteMicros: priceCacheWriteMicros,
		CostMicros:            costMicros,
		LatencyMS:             latencyMS,
		ErrorCode:             errorCode,
		StartedAt:             startedAt,
		EndedAt:               endedAt,
		CreatedAt:             createdAt,
		UpdatedAt:             updatedAt,
		AllColumns:            all,
	}
}
