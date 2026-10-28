package invite

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Store persists invites. A missing row is a *NotFoundError.
type Store interface {
	// LockEmail serializes changes to orgID's live invite for email until the transaction on ctx ends.
	LockEmail(ctx context.Context, orgID uuid.UUID, email string) error
	// Live returns orgID's invite for email that is neither accepted nor revoked, expired or not, locked until the transaction on ctx ends.
	Live(ctx context.Context, orgID uuid.UUID, email string) (*Invite, error)
	// Lock returns the invite id, locked FOR UPDATE until the transaction on ctx ends.
	Lock(ctx context.Context, id uuid.UUID) (*Invite, error)
	ByID(ctx context.Context, id uuid.UUID) (*Invite, error)
	ByTokenHash(ctx context.Context, hash string) (*Invite, error)
	ByPublicID(ctx context.Context, publicID string) (*Invite, error)
	// ResolveByPublicID finds publicID's invite in any org for the pre-sign-in landing page; the result never carries its token hash.
	ResolveByPublicID(ctx context.Context, publicID string) (*Invite, error)
	Insert(ctx context.Context, i *Invite) error
	// Update writes i's role, token, draft flag, inviter and expiry.
	Update(ctx context.Context, i *Invite) error
	// MarkAccepted uses up the invite id for userID at now, only while it is a live, unexpired, non-draft invite.
	MarkAccepted(ctx context.Context, id, userID uuid.UUID, now time.Time) (*Invite, error)
	// Revoke sets revoked_at on the invite id at now, only while it is live.
	Revoke(ctx context.Context, id uuid.UUID, now time.Time) error
	// ListLive returns orgID's live invites and drafts with the inviter's name, oldest first.
	ListLive(ctx context.Context, orgID uuid.UUID) ([]*Invite, error)
}
