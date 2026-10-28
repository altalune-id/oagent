package invite_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/org/invite"
	"altalune.id/oagent/internal/org/member"
)

// With passwords off, install invites the first owner by email: no inviter, no email sent, renewed on a repeat.
func TestService_InviteOwner(t *testing.T) {
	f := newFixture(t)
	other := uuid.New()
	pending, err := f.svc.PendingOwner(f.ctx, other)
	require.NoError(t, err)
	assert.False(t, pending)

	inv, err := f.svc.InviteOwner(f.ctx, other, " Owner@Example.com ")
	require.NoError(t, err)
	assert.Equal(t, "owner@example.com", inv.Email)
	assert.Equal(t, member.RoleOwner, inv.Role)
	assert.Equal(t, uuid.Nil, inv.InvitedBy)
	assert.Equal(t, f.clock.Add(invite.TTL), inv.ExpiresAt)
	assert.Zero(t, f.mail.count(), "the owner signs in through the OIDC provider; nothing is emailed")
	got := f.events.all()
	require.NotEmpty(t, got)
	assert.Equal(t, events.InviteCreated, got[len(got)-1].t)

	pending, err = f.svc.PendingOwner(f.ctx, other)
	require.NoError(t, err)
	assert.True(t, pending)

	f.clock = f.clock.Add(invite.TTL)
	pending, err = f.svc.PendingOwner(f.ctx, other)
	require.NoError(t, err)
	assert.False(t, pending, "an expired owner invite no longer counts")

	again, err := f.svc.InviteOwner(f.ctx, other, "owner@example.com")
	require.NoError(t, err)
	assert.Equal(t, inv.PublicID, again.PublicID, "the live invite is renewed, not duplicated")
	assert.Equal(t, f.clock.Add(invite.TTL), again.ExpiresAt)

	_, err = f.svc.InviteOwner(f.ctx, other, "not an email")
	assert.Error(t, err)
	f.store.Err = assert.AnError
	_, err = f.svc.InviteOwner(f.ctx, other, "owner@example.com")
	assert.Error(t, err)
	_, err = f.svc.PendingOwner(f.ctx, other)
	assert.Error(t, err)
}

// SECURITY: only the seat InviteOwner leaves is claimed by email alone; any other invite needs its link.
func TestService_EmailOnlyOwner(t *testing.T) {
	f := newFixture(t)
	inv, err := f.svc.InviteOwner(f.ctx, f.org, "owner@example.com")
	require.NoError(t, err)
	assert.True(t, inv.EmailOnly)

	byOperator, err := f.svc.Create(f.ctx, member.Requester{Operator: true}, f.org, "owner@example.com", member.RoleOwner)
	require.NoError(t, err)
	assert.Equal(t, inv.ID, byOperator.Invite.ID, "the live invite is reused")
	assert.False(t, byOperator.Invite.EmailOnly, "an invite someone sends needs its link, even with no inviter")
}

func TestService_Live(t *testing.T) {
	f := newFixture(t)
	other := uuid.New()
	_, err := f.svc.Live(f.ctx, other, "owner@example.com")
	assert.True(t, invite.IsNotUsableError(err), "no invite yet")
	_, err = f.svc.Live(f.ctx, other, "not an email")
	assert.True(t, invite.IsNotUsableError(err))

	inv, err := f.svc.InviteOwner(f.ctx, other, "owner@example.com")
	require.NoError(t, err)
	got, err := f.svc.Live(f.ctx, other, " OWNER@example.com")
	require.NoError(t, err)
	assert.Equal(t, inv.ID, got.ID)

	f.clock = f.clock.Add(invite.TTL)
	_, err = f.svc.Live(f.ctx, other, "owner@example.com")
	assert.True(t, invite.IsNotUsableError(err), "expired")

	f.store.Err = assert.AnError
	_, err = f.svc.Live(f.ctx, other, "owner@example.com")
	assert.Error(t, err)
}
