package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/nanoid"
)

func TestNewReset(t *testing.T) {
	now := time.Date(2026, 10, 13, 9, 0, 0, 0, time.FixedZone("WIB", 7*3600))
	userID := uuid.New()
	r, raw, err := newReset(userID, now)
	require.NoError(t, err)
	assert.Equal(t, userID, r.UserID)
	assert.True(t, nanoid.IsValid(r.PublicID, 21))
	assert.Len(t, raw, 32)
	assert.Equal(t, hashResetToken(raw), r.TokenHash)
	assert.NotContains(t, r.TokenHash, raw)
	assert.Equal(t, now.UTC().Add(time.Hour), r.ExpiresAt)
	assert.Equal(t, time.UTC, r.CreatedAt.Location())
	assert.Nil(t, r.UsedAt)

	other, raw2, err := newReset(userID, now)
	require.NoError(t, err)
	assert.NotEqual(t, raw, raw2)
	assert.NotEqual(t, r.PublicID, other.PublicID)
}

func TestReset_Usable(t *testing.T) {
	now := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	r, _, err := newReset(uuid.New(), now)
	require.NoError(t, err)
	assert.NoError(t, r.Usable(now))
	assert.NoError(t, r.Usable(now.Add(ResetTTL-time.Second)))
	assert.Equal(t, ResetExpired, ResetReasonOf(r.Usable(now.Add(ResetTTL))))
	used := now.Add(time.Minute)
	r.UsedAt = &used
	assert.Equal(t, ResetUsed, ResetReasonOf(r.Usable(now)))
}

func TestResetRow(t *testing.T) {
	used := time.Now()
	row := resetRow{ID: uuid.New(), PublicID: "p", UserID: uuid.New(), TokenHash: "h", ExpiresAt: used, UsedAt: &used, CreatedAt: used}
	r := row.toReset()
	assert.Equal(t, Reset{ID: row.ID, PublicID: "p", UserID: row.UserID, TokenHash: "h", ExpiresAt: used, UsedAt: &used, CreatedAt: used}, *r)
}
