package channel

import (
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/schema/entity"
)

type postgresStore struct {
	pool       db.Pool
	conns      *entity.ChannelConnections
	deliveries *entity.IngestDeliveries
	resolve    string
}

// NewStore builds the Postgres store over pool.
func NewStore(pool db.Pool, schema, tablePrefix string) *postgresStore {
	if schema == "" {
		schema = "public"
	}
	return &postgresStore{
		pool:       pool,
		conns:      entity.NewChannelConnections(schema, tablePrefix),
		deliveries: entity.NewIngestDeliveries(schema, tablePrefix),
		// NOTE: RawStatement because go-jet has no builder for a set-returning function in FROM position.
		resolve: `SELECT x.id AS "ref.id", x.org_id AS "ref.org_id", x.project_id AS "ref.project_id", x.agent_id AS "ref.agent_id",
			x.provider AS "ref.provider", x.device_id AS "ref.device_id", x.status AS "ref.status",
			x.secured AS "ref.secured" FROM ` +
			schema + "." + tablePrefix + "resolve_channel_connection_by_public_id(#publicID) x",
	}
}

type connRow struct {
	ID                     uuid.UUID  `alias:"channel_connections.id"`
	PublicID               string     `alias:"channel_connections.public_id"`
	OrgID                  uuid.UUID  `alias:"channel_connections.org_id"`
	ProjectID              uuid.UUID  `alias:"channel_connections.project_id"`
	AgentID                uuid.UUID  `alias:"channel_connections.agent_id"`
	Provider               string     `alias:"channel_connections.provider"`
	Status                 string     `alias:"channel_connections.status"`
	BaseURL                string     `alias:"channel_connections.base_url"`
	OpenWAOrg              string     `alias:"channel_connections.openwa_org"`
	OpenWAProject          string     `alias:"channel_connections.openwa_project"`
	DeviceID               string     `alias:"channel_connections.device_id"`
	SigningPrimarySealed   []byte     `alias:"channel_connections.signing_secret_primary_sealed"`
	SigningSecondarySealed []byte     `alias:"channel_connections.signing_secret_secondary_sealed"`
	DeviceKeySealed        []byte     `alias:"channel_connections.device_key_sealed"`
	QuietWindowMS          int32      `alias:"channel_connections.quiet_window_ms"`
	QuietMaxMS             int32      `alias:"channel_connections.quiet_max_ms"`
	ReplyFailure           string     `alias:"channel_connections.reply_failure"`
	ReplyVoice             string     `alias:"channel_connections.reply_voice"`
	ReplyPhotoUnreadable   string     `alias:"channel_connections.reply_photo_unreadable"`
	LastResult             *string    `alias:"channel_connections.last_result"`
	LastResultAt           *time.Time `alias:"channel_connections.last_result_at"`
	LastBadSignatureAt     *time.Time `alias:"channel_connections.last_bad_signature_at"`
	CreatedAt              time.Time  `alias:"channel_connections.created_at"`
	UpdatedAt              time.Time  `alias:"channel_connections.updated_at"`
}

func (r *connRow) toConnection() *Connection {
	c := &Connection{
		ID: r.ID, PublicID: r.PublicID, OrgID: r.OrgID, ProjectID: r.ProjectID, AgentID: r.AgentID, Provider: r.Provider, Status: r.Status,
		BaseURL: r.BaseURL, OpenWAOrg: r.OpenWAOrg, OpenWAProject: r.OpenWAProject, DeviceID: r.DeviceID,
		SigningPrimarySealed: r.SigningPrimarySealed, SigningSecondarySealed: r.SigningSecondarySealed, DeviceKeySealed: r.DeviceKeySealed,
		QuietWindow: time.Duration(r.QuietWindowMS) * time.Millisecond, QuietMax: time.Duration(r.QuietMaxMS) * time.Millisecond,
		ReplyFailure: r.ReplyFailure, ReplyVoice: r.ReplyVoice, ReplyPhotoUnreadable: r.ReplyPhotoUnreadable,
		LastResultAt: utcPtr(r.LastResultAt), LastBadSignatureAt: utcPtr(r.LastBadSignatureAt),
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if r.LastResult != nil {
		c.LastResult = *r.LastResult
	}
	return c
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
