package entity

import "github.com/go-jet/jet/v2/postgres"

// ChannelConnections is the jet binding for the channel_connections table.
type ChannelConnections struct {
	postgres.Table

	ID                           postgres.ColumnString
	PublicID                     postgres.ColumnString
	OrgID                        postgres.ColumnString
	ProjectID                    postgres.ColumnString
	AgentID                      postgres.ColumnString
	Provider                     postgres.ColumnString
	Status                       postgres.ColumnString
	BaseURL                      postgres.ColumnString
	OpenwaOrg                    postgres.ColumnString
	OpenwaProject                postgres.ColumnString
	DeviceID                     postgres.ColumnString
	SigningSecretPrimarySealed   postgres.ColumnBytea
	SigningSecretSecondarySealed postgres.ColumnBytea
	DeviceKeySealed              postgres.ColumnBytea
	QuietWindowMS                postgres.ColumnInteger
	QuietMaxMS                   postgres.ColumnInteger
	ReplyFailure                 postgres.ColumnString
	ReplyVoice                   postgres.ColumnString
	ReplyPhotoUnreadable         postgres.ColumnString
	LastResult                   postgres.ColumnString
	LastResultAt                 postgres.ColumnTimestampz
	LastBadSignatureAt           postgres.ColumnTimestampz
	CreatedAt                    postgres.ColumnTimestampz
	UpdatedAt                    postgres.ColumnTimestampz

	AllColumns postgres.ColumnList
}

// NewChannelConnections builds the channel_connections binding.
func NewChannelConnections(schema, tablePrefix string) *ChannelConnections {
	if schema == "" {
		schema = "public"
	}
	var (
		id                           = postgres.StringColumn("id")
		publicID                     = postgres.StringColumn("public_id")
		orgID                        = postgres.StringColumn("org_id")
		projectID                    = postgres.StringColumn("project_id")
		agentID                      = postgres.StringColumn("agent_id")
		provider                     = postgres.StringColumn("provider")
		status                       = postgres.StringColumn("status")
		baseURL                      = postgres.StringColumn("base_url")
		openwaOrg                    = postgres.StringColumn("openwa_org")
		openwaProject                = postgres.StringColumn("openwa_project")
		deviceID                     = postgres.StringColumn("device_id")
		signingSecretPrimarySealed   = postgres.ByteaColumn("signing_secret_primary_sealed")
		signingSecretSecondarySealed = postgres.ByteaColumn("signing_secret_secondary_sealed")
		deviceKeySealed              = postgres.ByteaColumn("device_key_sealed")
		quietWindowMS                = postgres.IntegerColumn("quiet_window_ms")
		quietMaxMS                   = postgres.IntegerColumn("quiet_max_ms")
		replyFailure                 = postgres.StringColumn("reply_failure")
		replyVoice                   = postgres.StringColumn("reply_voice")
		replyPhotoUnreadable         = postgres.StringColumn("reply_photo_unreadable")
		lastResult                   = postgres.StringColumn("last_result")
		lastResultAt                 = postgres.TimestampzColumn("last_result_at")
		lastBadSignatureAt           = postgres.TimestampzColumn("last_bad_signature_at")
		createdAt                    = postgres.TimestampzColumn("created_at")
		updatedAt                    = postgres.TimestampzColumn("updated_at")
		all                          = postgres.ColumnList{
			id, publicID, orgID, projectID, agentID, provider, status, baseURL, openwaOrg, openwaProject, deviceID,
			signingSecretPrimarySealed, signingSecretSecondarySealed, deviceKeySealed, quietWindowMS, quietMaxMS, replyFailure,
			replyVoice, replyPhotoUnreadable, lastResult, lastResultAt, lastBadSignatureAt, createdAt, updatedAt,
		}
	)
	return &ChannelConnections{
		Table:                        postgres.NewTable(schema, tablePrefix+"channel_connections", "channel_connections", all...),
		ID:                           id,
		PublicID:                     publicID,
		OrgID:                        orgID,
		ProjectID:                    projectID,
		AgentID:                      agentID,
		Provider:                     provider,
		Status:                       status,
		BaseURL:                      baseURL,
		OpenwaOrg:                    openwaOrg,
		OpenwaProject:                openwaProject,
		DeviceID:                     deviceID,
		SigningSecretPrimarySealed:   signingSecretPrimarySealed,
		SigningSecretSecondarySealed: signingSecretSecondarySealed,
		DeviceKeySealed:              deviceKeySealed,
		QuietWindowMS:                quietWindowMS,
		QuietMaxMS:                   quietMaxMS,
		ReplyFailure:                 replyFailure,
		ReplyVoice:                   replyVoice,
		ReplyPhotoUnreadable:         replyPhotoUnreadable,
		LastResult:                   lastResult,
		LastResultAt:                 lastResultAt,
		LastBadSignatureAt:           lastBadSignatureAt,
		CreatedAt:                    createdAt,
		UpdatedAt:                    updatedAt,
		AllColumns:                   all,
	}
}
