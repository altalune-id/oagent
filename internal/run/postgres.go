package run

import (
	"encoding/json"
	"time"

	"github.com/go-jet/jet/v2/postgres"
	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool          db.Pool
	maxPerProject int
	maxAge        time.Duration
	threads       *entity.Threads
	runs          *entity.Runs
	messages      *entity.Messages
	conns         *entity.ChannelConnections
	identities    *entity.EndUserIdentities
	approvals     *entity.Approvals
	resolveThread string
	resolveRun    string
}

var _ Windows = (*postgresStore)(nil)

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	if schema == "" {
		schema = "public"
	}
	return &postgresStore{
		pool:          pool,
		maxPerProject: DefaultMaxConcurrentPerProject,
		maxAge:        DefaultMaxMessageAge,
		threads:       entity.NewThreads(schema, tablePrefix),
		runs:          entity.NewRuns(schema, tablePrefix),
		messages:      entity.NewMessages(schema, tablePrefix),
		conns:         entity.NewChannelConnections(schema, tablePrefix),
		identities:    entity.NewEndUserIdentities(schema, tablePrefix),
		approvals:     entity.NewApprovals(schema, tablePrefix),
		resolveThread: resolveSQL(schema, tablePrefix, "thread"),
		resolveRun:    resolveSQL(schema, tablePrefix, "run"),
	}
}

// WithMaxConcurrentPerProject caps how many threads of one project hold a live lease at once; n below 1 keeps the default.
func (s *postgresStore) WithMaxConcurrentPerProject(n int) *postgresStore {
	if n > 0 {
		s.maxPerProject = n
	}
	return s
}

// WithMaxMessageAge skips channel messages older than d when a turn takes them; d below 1 keeps the default.
func (s *postgresStore) WithMaxMessageAge(d time.Duration) *postgresStore {
	if d > 0 {
		s.maxAge = d
	}
	return s
}

type runRow struct {
	ID                    uuid.UUID  `alias:"runs.id"`
	PublicID              string     `alias:"runs.public_id"`
	OrgID                 uuid.UUID  `alias:"runs.org_id"`
	ProjectID             uuid.UUID  `alias:"runs.project_id"`
	ThreadID              uuid.UUID  `alias:"runs.thread_id"`
	AgentVersionID        uuid.UUID  `alias:"runs.agent_version_id"`
	Status                string     `alias:"runs.status"`
	IdempotencyKey        *string    `alias:"runs.idempotency_key"`
	Attempt               int32      `alias:"runs.attempt"`
	IsGroup               bool       `alias:"runs.is_group"`
	InputTokens           int64      `alias:"runs.input_tokens"`
	OutputTokens          int64      `alias:"runs.output_tokens"`
	CacheReadTokens       int64      `alias:"runs.cache_read_tokens"`
	CacheWriteTokens      int64      `alias:"runs.cache_write_tokens"`
	UsageEstimated        bool       `alias:"runs.usage_estimated"`
	PriceInputMicros      *int64     `alias:"runs.price_input_micros"`
	PriceOutputMicros     *int64     `alias:"runs.price_output_micros"`
	PriceCacheReadMicros  *int64     `alias:"runs.price_cache_read_micros"`
	PriceCacheWriteMicros *int64     `alias:"runs.price_cache_write_micros"`
	ErrorCode             *string    `alias:"runs.error_code"`
	StartedAt             time.Time  `alias:"runs.started_at"`
	EndedAt               *time.Time `alias:"runs.ended_at"`
	UpdatedAt             time.Time  `alias:"runs.updated_at"`
}

func (r *runRow) toRun() *Run {
	out := &Run{
		ID: r.ID, PublicID: r.PublicID, OrgID: r.OrgID, ProjectID: r.ProjectID, ThreadID: r.ThreadID,
		AgentVersionID: r.AgentVersionID, Status: Status(r.Status), Attempt: r.Attempt, Group: r.IsGroup,
		Usage: Usage{
			InputTokens: r.InputTokens, OutputTokens: r.OutputTokens, CacheReadTokens: r.CacheReadTokens,
			CacheWriteTokens: r.CacheWriteTokens, Estimated: r.UsageEstimated,
		},
		StartedAt: r.StartedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if r.IdempotencyKey != nil {
		out.IdempotencyKey = *r.IdempotencyKey
	}
	if r.ErrorCode != nil {
		out.ErrorCode = *r.ErrorCode
	}
	if r.EndedAt != nil {
		out.EndedAt = new(r.EndedAt.UTC())
	}
	if r.PriceInputMicros != nil || r.PriceOutputMicros != nil || r.PriceCacheReadMicros != nil || r.PriceCacheWriteMicros != nil {
		out.Prices = &Prices{
			Input: r.PriceInputMicros, Output: r.PriceOutputMicros,
			CacheRead: r.PriceCacheReadMicros, CacheWrite: r.PriceCacheWriteMicros,
		}
	}
	return out
}

type messageRow struct {
	ID                uuid.UUID  `alias:"messages.id"`
	OrgID             uuid.UUID  `alias:"messages.org_id"`
	ProjectID         uuid.UUID  `alias:"messages.project_id"`
	ThreadID          uuid.UUID  `alias:"messages.thread_id"`
	Seq               int64      `alias:"messages.seq"`
	Role              string     `alias:"messages.role"`
	Body              string     `alias:"messages.body"`
	SpeakerID         *uuid.UUID `alias:"messages.speaker_id"`
	Status            string     `alias:"messages.status"`
	ExternalMessageID *string    `alias:"messages.external_message_id"`
	RunID             *uuid.UUID `alias:"messages.run_id"`
	ToolCallID        *string    `alias:"messages.tool_call_id"`
	CreatedAt         time.Time  `alias:"messages.created_at"`
	RequestHash       []byte     `alias:"messages.request_hash"`
	ChannelTime       *time.Time `alias:"messages.channel_time"`
	SkipReason        *string    `alias:"messages.skip_reason"`
}

func (r *messageRow) toMessage() Message {
	m := Message{
		ID: r.ID, OrgID: r.OrgID, ProjectID: r.ProjectID, ThreadID: r.ThreadID, Seq: r.Seq, Role: Role(r.Role),
		Body: json.RawMessage(r.Body), SpeakerID: r.SpeakerID, Status: MessageStatus(r.Status), RunID: r.RunID,
		CreatedAt: r.CreatedAt.UTC(), RequestHash: r.RequestHash,
	}
	if r.ExternalMessageID != nil {
		m.ExternalMessageID = *r.ExternalMessageID
	}
	if r.ToolCallID != nil {
		m.ToolCallID = *r.ToolCallID
	}
	if r.ChannelTime != nil {
		at := r.ChannelTime.UTC()
		m.ChannelTime = &at
	}
	if r.SkipReason != nil {
		m.SkipReason = *r.SkipReason
	}
	return m
}

func toMessages(rows []messageRow) []Message {
	out := make([]Message, len(rows))
	for i := range rows {
		out[i] = rows[i].toMessage()
	}
	return out
}

func orNull[T any](v *T, expr func(T) postgres.Expression) postgres.Expression {
	if v == nil {
		return postgres.NULL
	}
	return expr(*v)
}

func textOrNull(s string) postgres.Expression {
	if s == "" {
		return postgres.NULL
	}
	return postgres.String(s)
}

func jsonb(body json.RawMessage) postgres.Expression {
	return postgres.CAST(postgres.String(string(body))).AS("jsonb")
}

func resolveSQL(schema, tablePrefix, what string) string {
	return `SELECT x.id AS "ref.id", x.org_id AS "ref.org_id", x.project_id AS "ref.project_id", x.agent_id AS "ref.agent_id" FROM ` +
		schema + "." + tablePrefix + "resolve_" + what + "_by_public_id(#publicID) x"
}
