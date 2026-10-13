// Package invite brings people into an org by email, through a single-use link and a transactional accept.
package invite

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/nanoid"
)

const publicIDLen = 21

// TTL is how long an invite link works.
const TTL = 7 * 24 * time.Hour

// DraftTTL is how long a person has to confirm a draft an assistant made.
const DraftTTL = 15 * time.Minute

// Status is an invite's state as listed to the org.
type Status string

// The statuses of a live invite.
const (
	StatusPending Status = "pending"
	StatusExpired Status = "expired"
	StatusDraft   Status = "draft"
)

// Outcome is what accepting an invite did.
type Outcome int

// The outcomes of Accept.
const (
	Joined Outcome = iota + 1
	AlreadyMember
)

// Invite asks one email to join one org with a role.
type Invite struct {
	ID          uuid.UUID
	OrgID       uuid.UUID
	PublicID    string
	Email       string
	Role        member.Role
	TokenHash   string
	Draft       bool
	InvitedBy   uuid.UUID
	InviterName string
	ExpiresAt   time.Time
	AcceptedAt  *time.Time
	AcceptedBy  uuid.UUID
	RevokedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time

	unsaved bool
}

// Created is a new or renewed invite with its link. SECURITY: Link holds the raw token; it is returned once and never stored.
type Created struct {
	Invite *Invite
	Link   string
}

func newInvite(orgID uuid.UUID, email string, role member.Role, invitedBy uuid.UUID, now time.Time) (*Invite, error) {
	publicID, err := nanoid.New(publicIDLen)
	if err != nil {
		return nil, err
	}
	now = now.UTC()
	return &Invite{
		ID: uuid.New(), OrgID: orgID, PublicID: publicID, Email: email, Role: role,
		InvitedBy: invitedBy, CreatedAt: now, UpdatedAt: now, unsaved: true,
	}, nil
}

func (i *Invite) issue(now time.Time) (string, error) {
	raw, hash, err := nanoid.NewInviteToken()
	if err != nil {
		return "", err
	}
	i.TokenHash, i.Draft = hash, false
	i.ExpiresAt, i.UpdatedAt = now.UTC().Add(TTL), now.UTC()
	return raw, nil
}

func (i *Invite) draft(now time.Time) {
	i.TokenHash, i.Draft, i.Role = "", true, member.RoleMember
	i.ExpiresAt, i.UpdatedAt = now.UTC().Add(DraftTTL), now.UTC()
}

func (i *Invite) live() bool { return i.AcceptedAt == nil && i.RevokedAt == nil }

func (i *Invite) expired(now time.Time) bool { return !now.Before(i.ExpiresAt) }

// Status reports how i is listed at now.
func (i *Invite) Status(now time.Time) Status {
	switch {
	case i.Draft:
		return StatusDraft
	case i.expired(now):
		return StatusExpired
	}
	return StatusPending
}

// Usable returns nil when a person may still accept i at now, else a *NotUsableError with the reason.
func (i *Invite) Usable(now time.Time) error {
	switch {
	case i.Draft:
		return &NotUsableError{Reason: ReasonNotFound}
	case i.AcceptedAt != nil:
		return &NotUsableError{Reason: ReasonUsed}
	case i.RevokedAt != nil:
		return &NotUsableError{Reason: ReasonRevoked}
	case i.expired(now):
		return &NotUsableError{Reason: ReasonExpired}
	}
	return nil
}

type inviteRow struct {
	ID         uuid.UUID      `alias:"invites.id"`
	PublicID   string         `alias:"invites.public_id"`
	OrgID      uuid.UUID      `alias:"invites.org_id"`
	Email      string         `alias:"invites.email"`
	Role       string         `alias:"invites.role"`
	TokenHash  sql.NullString `alias:"invites.token_hash"`
	Draft      bool           `alias:"invites.draft"`
	InvitedBy  uuid.NullUUID  `alias:"invites.invited_by"`
	ExpiresAt  time.Time      `alias:"invites.expires_at"`
	AcceptedAt *time.Time     `alias:"invites.accepted_at"`
	AcceptedBy uuid.NullUUID  `alias:"invites.accepted_by"`
	RevokedAt  *time.Time     `alias:"invites.revoked_at"`
	CreatedAt  time.Time      `alias:"invites.created_at"`
	UpdatedAt  time.Time      `alias:"invites.updated_at"`
}

func (r *inviteRow) toInvite() *Invite {
	return &Invite{
		ID: r.ID, OrgID: r.OrgID, PublicID: r.PublicID, Email: r.Email, Role: member.Role(r.Role),
		TokenHash: r.TokenHash.String, Draft: r.Draft, InvitedBy: r.InvitedBy.UUID, ExpiresAt: r.ExpiresAt,
		AcceptedAt: r.AcceptedAt, AcceptedBy: r.AcceptedBy.UUID, RevokedAt: r.RevokedAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

type listedRow struct {
	Invite       inviteRow
	InviterName  sql.NullString `alias:"users.name"`
	InviterEmail sql.NullString `alias:"users.email"`
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
