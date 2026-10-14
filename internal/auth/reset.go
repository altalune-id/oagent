package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"

	"altalune.id/oagent/nanoid"
)

// ResetTTL is how long a password reset link works.
const ResetTTL = time.Hour

const resetPublicIDLen = 21

// Reset is one emailed password reset; only the token's hash is kept.
type Reset struct {
	ID        uuid.UUID
	PublicID  string
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

func newReset(userID uuid.UUID, now time.Time) (*Reset, string, error) {
	raw, hash, err := nanoid.NewInviteToken()
	if err != nil {
		return nil, "", err
	}
	publicID, err := nanoid.New(resetPublicIDLen)
	if err != nil {
		return nil, "", err
	}
	now = now.UTC()
	return &Reset{
		ID: uuid.New(), PublicID: publicID, UserID: userID, TokenHash: hash,
		ExpiresAt: now.Add(ResetTTL), CreatedAt: now,
	}, raw, nil
}

// Usable returns a *ResetNotUsableError when r was used or has expired at now.
func (r *Reset) Usable(now time.Time) error {
	switch {
	case r.UsedAt != nil:
		return &ResetNotUsableError{Reason: ResetUsed}
	case !now.Before(r.ExpiresAt):
		return &ResetNotUsableError{Reason: ResetExpired}
	}
	return nil
}

func hashResetToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

type resetRow struct {
	ID        uuid.UUID  `alias:"password_resets.id"`
	PublicID  string     `alias:"password_resets.public_id"`
	UserID    uuid.UUID  `alias:"password_resets.user_id"`
	TokenHash string     `alias:"password_resets.token_hash"`
	ExpiresAt time.Time  `alias:"password_resets.expires_at"`
	UsedAt    *time.Time `alias:"password_resets.used_at"`
	CreatedAt time.Time  `alias:"password_resets.created_at"`
}

func (r *resetRow) toReset() *Reset {
	return &Reset{
		ID: r.ID, PublicID: r.PublicID, UserID: r.UserID, TokenHash: r.TokenHash,
		ExpiresAt: r.ExpiresAt, UsedAt: r.UsedAt, CreatedAt: r.CreatedAt,
	}
}
