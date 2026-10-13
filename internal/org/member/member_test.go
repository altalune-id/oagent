package member

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
)

func TestRole(t *testing.T) {
	for role, want := range map[Role]bool{
		RoleOwner:    true,
		RoleAdmin:    true,
		RoleMember:   true,
		Role("root"): false,
		Role(""):     false,
	} {
		assert.Equal(t, want, role.IsValid(), "%q valid", role)
	}
}

func TestRole_Label(t *testing.T) {
	assert.Equal(t, "Owner", RoleOwner.Label())
	assert.Equal(t, "Admin", RoleAdmin.Label())
	assert.Equal(t, "Member", RoleMember.Label())
	assert.Equal(t, "auditor", Role("auditor").Label(), "an unknown role is never shown as Member")
}

func TestNew(t *testing.T) {
	org, usr := uuid.New(), uuid.New()
	m, err := New(org, usr, RoleOwner)
	require.NoError(t, err)
	assert.Len(t, m.PublicID, 21)
	assert.Equal(t, org, m.OrgID)
	assert.Equal(t, usr, m.UserID)
	assert.NotEqual(t, uuid.Nil, m.ID)

	_, err = New(uuid.New(), uuid.New(), "root")
	assert.True(t, IsInvalidRoleError(err))
	assert.Equal(t, map[string]string{"role": "root"}, mustAppErr(t, err).Meta())

	_, err = New(uuid.Nil, uuid.New(), RoleMember)
	assert.Equal(t, map[string]string{"field": "organization_id", "reason": "required"}, mustAppErr(t, err).Meta())
	_, err = New(uuid.New(), uuid.Nil, RoleMember)
	assert.Equal(t, map[string]string{"field": "user_id", "reason": "required"}, mustAppErr(t, err).Meta())
}

func mustAppErr(t *testing.T, err error) *apperror.AppError {
	t.Helper()
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok, "%v carries no code", err)
	return ae
}
