package channel

import (
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/nanoid"
)

// ProviderOpenWA is the OpenWA provider, the only one today.
const ProviderOpenWA = "openwa"

// The connection's on/off switch; a disabled connection answers deliveries like an unknown one.
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

// The verified outcomes a connection reports; a bad signature is never one of them.
const (
	ResultConnected      = "connected"
	ResultDeviceMismatch = "device_mismatch"
	ResultKeyRefused     = "key_refused"
)

const (
	publicIDLen = 21
	maxBaseURL  = 2048
	maxSetReply = 1000
)

var deviceIDPattern = regexp.MustCompile(`^dev_[A-Za-z0-9_-]{16}$`)

// Connection binds an agent to a WhatsApp device through a provider; sealed fields hold secrets that only OpenSecrets opens.
type Connection struct {
	ID, OrgID, ProjectID, AgentID                  uuid.UUID
	PublicID, Provider, Status                     string
	BaseURL, OpenWAOrg, OpenWAProject, DeviceID    string
	SigningPrimarySealed, SigningSecondarySealed   []byte
	DeviceKeySealed                                []byte
	QuietWindow, QuietMax                          time.Duration
	ReplyFailure, ReplyVoice, ReplyPhotoUnreadable string
	ApprovalPrompt, ApprovalExpired                string
	LastResult                                     string
	LastResultAt, LastBadSignatureAt               *time.Time
	CreatedAt, UpdatedAt                           time.Time
}

// Input is a connection's editable settings; a nil duration or approval text, or an empty reply or status, keeps the value.
type Input struct {
	BaseURL, OpenWAOrg, OpenWAProject, DeviceID    string
	QuietWindow, QuietMax                          *time.Duration
	ReplyFailure, ReplyVoice, ReplyPhotoUnreadable string
	ApprovalPrompt, ApprovalExpired                *string
	Status                                         string
}

// Ref is what a connection public id resolves to before any tenant is bound.
type Ref struct {
	ID, OrgID, ProjectID, AgentID uuid.UUID
	Provider, DeviceID, Status    string
	// SecretSet is false until the first signing secret is set.
	SecretSet bool
}

// Secrets are a connection's opened secrets: the signing secrets newest first, and the device key, the bearer the provider's API takes.
type Secrets struct {
	Signing   []string
	DeviceKey string
}

type options struct{ allowPrivate bool }

// Option configures validation.
type Option func(*options)

// AllowPrivateHosts lets the base URL use http, for an operator who runs OpenWA on a private network.
func AllowPrivateHosts(allow bool) Option { return func(o *options) { o.allowPrivate = allow } }

// New builds a connection from in with the defaults applied, after validating every setting.
func New(orgID, projectID, agentID uuid.UUID, in Input, now time.Time, opts ...Option) (*Connection, error) {
	now = now.UTC()
	c := &Connection{
		ID: uuid.Must(uuid.NewV7()), PublicID: nanoid.MustNew(publicIDLen), OrgID: orgID, ProjectID: projectID, AgentID: agentID,
		Provider: ProviderOpenWA, Status: StatusActive, QuietWindow: DefaultQuietWindow, QuietMax: DefaultQuietMax,
		ReplyFailure: DefaultReplyFailure, ReplyVoice: DefaultReplyVoice, ReplyPhotoUnreadable: DefaultReplyPhotoUnreadable,
		ApprovalPrompt: DefaultApprovalPrompt, ApprovalExpired: DefaultApprovalExpired,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := c.apply(in, now, opts...); err != nil {
		return nil, err
	}
	return c, nil
}

// Apply replaces c's editable settings with in, validated; unset values keep what c has.
func (c *Connection) Apply(in Input, now time.Time, opts ...Option) error {
	return c.apply(in, now.UTC(), opts...)
}

func (c *Connection) apply(in Input, now time.Time, opts ...Option) error {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	next := *c
	next.BaseURL, next.OpenWAOrg, next.OpenWAProject, next.DeviceID = strings.TrimSpace(in.BaseURL), strings.TrimSpace(in.OpenWAOrg),
		strings.TrimSpace(in.OpenWAProject), strings.TrimSpace(in.DeviceID)
	if in.QuietWindow != nil {
		next.QuietWindow = *in.QuietWindow
	}
	if in.QuietMax != nil {
		next.QuietMax = *in.QuietMax
	}
	for _, r := range []struct {
		dst *string
		v   string
	}{{&next.ReplyFailure, in.ReplyFailure}, {&next.ReplyVoice, in.ReplyVoice}, {&next.ReplyPhotoUnreadable, in.ReplyPhotoUnreadable}} {
		if strings.TrimSpace(r.v) != "" {
			*r.dst = strings.TrimSpace(r.v)
		}
	}
	if in.ApprovalPrompt != nil {
		next.ApprovalPrompt = strings.TrimSpace(*in.ApprovalPrompt)
	}
	if in.ApprovalExpired != nil {
		next.ApprovalExpired = strings.TrimSpace(*in.ApprovalExpired)
	}
	if in.Status != "" {
		next.Status = in.Status
	}
	if err := next.validate(o); err != nil {
		return err
	}
	next.UpdatedAt = now
	*c = next
	return nil
}

func (c *Connection) validate(o options) error {
	if err := validBaseURL(c.BaseURL, o.allowPrivate); err != nil {
		return err
	}
	switch {
	case !deviceIDPattern.MatchString(c.DeviceID):
		return &InvalidError{Field: "device_id"}
	case c.OpenWAOrg == "" || len(c.OpenWAOrg) > 128:
		return &InvalidError{Field: "openwa_org"}
	case c.OpenWAProject == "" || len(c.OpenWAProject) > 128:
		return &InvalidError{Field: "openwa_project"}
	case c.QuietWindow < 0 || c.QuietWindow > MaxQuiet:
		return &InvalidError{Field: "quiet_window_ms"}
	case c.QuietMax < c.QuietWindow || c.QuietMax > MaxQuiet:
		return &InvalidError{Field: "quiet_max_ms"}
	case len([]rune(c.ReplyFailure)) > maxSetReply:
		return &InvalidError{Field: "reply_failure"}
	case len([]rune(c.ReplyVoice)) > maxSetReply:
		return &InvalidError{Field: "reply_voice"}
	case len([]rune(c.ReplyPhotoUnreadable)) > maxSetReply:
		return &InvalidError{Field: "reply_photo_unreadable"}
	case c.ApprovalPrompt == "" || len([]rune(c.ApprovalPrompt)) > maxSetReply:
		return &InvalidError{Field: "approval_prompt"}
	case c.ApprovalExpired == "" || len([]rune(c.ApprovalExpired)) > maxSetReply:
		return &InvalidError{Field: "approval_expired"}
	case c.Status != StatusActive && c.Status != StatusDisabled:
		return &InvalidError{Field: "status"}
	}
	return nil
}

// SECURITY: a CR or LF would inject headers into requests to OpenWA; a query or fragment would swallow the path built after it.
func validBaseURL(raw string, allowPrivate bool) error {
	if raw == "" || len(raw) > maxBaseURL || strings.ContainsAny(raw, "\r\n?#") {
		return &InvalidError{Field: "base_url"}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return &InvalidError{Field: "base_url"}
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !allowPrivate) {
		return &InvalidError{Field: "base_url"}
	}
	return nil
}

// ReportedStatus is the status the console and events show: the last verified outcome, or waiting before the first.
func (c *Connection) ReportedStatus() string {
	if c.LastResult == "" {
		return "waiting"
	}
	return c.LastResult
}
