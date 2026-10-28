package install

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/invite"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/testutil/fakes"
)

func newRequiredService(t *testing.T, sys *org.Org) (*Service, *member.Service) {
	t.Helper()
	unexpected := apperror.NewReporter(nil, false).Unexpected
	members := member.NewService(&fakes.MemberStore{}, fakes.Transactor{}, unexpected)
	orgs := org.NewService(&fakes.OrgStore{Org: sys}, fakes.Transactor{}, unexpected)
	return NewService(fakes.Transactor{}, nil, orgs, members, nil), members
}

func TestRequired(t *testing.T) {
	ctx := context.Background()

	svc, _ := newRequiredService(t, nil)
	for _, basic := range []bool{false, true} {
		got, err := svc.Required(ctx, basic)
		require.NoError(t, err)
		assert.True(t, got, "no system org: basic=%v", basic)
	}

	sys, err := org.NewSystemOrg("default", "Default", uuid.Nil)
	require.NoError(t, err)
	svc, members := newRequiredService(t, sys)
	got, err := svc.Required(ctx, false)
	require.NoError(t, err)
	assert.True(t, got, "local: an ownerless system org still needs install")
	got, err = svc.Required(ctx, true)
	require.NoError(t, err)
	assert.False(t, got, "basic: the system org alone is enough")

	_, err = members.Add(ctx, sys.ID, uuid.New(), member.RoleOwner)
	require.NoError(t, err)
	got, err = svc.Required(ctx, false)
	require.NoError(t, err)
	assert.False(t, got)
}

type ownerInvites struct {
	pending bool
	err     error
}

func (o *ownerInvites) InviteOwner(context.Context, uuid.UUID, string) (*invite.Invite, error) {
	return nil, o.err
}

func (o *ownerInvites) PendingOwner(context.Context, uuid.UUID) (bool, error) {
	return o.pending, o.err
}

func TestRequired_PendingOwner(t *testing.T) {
	ctx := context.Background()
	sys, err := org.NewSystemOrg("default", "Default", uuid.Nil)
	require.NoError(t, err)
	svc, _ := newRequiredService(t, sys)
	assert.True(t, svc.Passwords())
	invites := &ownerInvites{pending: true}
	svc = svc.WithoutPasswords(invites)
	assert.False(t, svc.Passwords())

	got, err := svc.Required(ctx, false)
	require.NoError(t, err)
	assert.False(t, got, "a pending owner invite (passwords off) counts as installed")
	invites.pending = false
	got, err = svc.Required(ctx, false)
	require.NoError(t, err)
	assert.True(t, got, "an expired or used-up seat with no owner reopens install")
	invites.err = errors.New("invites down")
	_, err = svc.Required(ctx, false)
	require.Error(t, err)
}

func TestAlreadyInstalledError(t *testing.T) {
	err := &AlreadyInstalledError{}
	assert.Equal(t, "install: already installed", err.Error())
	assert.True(t, IsAlreadyInstalledError(err))
	assert.False(t, IsAlreadyInstalledError(assert.AnError))
	ae := err.ToAppError()
	assert.Equal(t, apperror.CodeInstallAlreadyDone, ae.Code())
	assert.Equal(t, "This instance is already installed", ae.Message())
}

func TestRequired_PropagatesErrors(t *testing.T) {
	ctx := context.Background()
	unexpected := apperror.NewReporter(nil, false).Unexpected
	sys, err := org.NewSystemOrg("default", "Default", uuid.Nil)
	require.NoError(t, err)

	orgs := org.NewService(&fakes.OrgStore{Err: errors.New("org store down")}, fakes.Transactor{}, unexpected)
	svc := NewService(fakes.Transactor{}, nil, orgs, member.NewService(&fakes.MemberStore{}, fakes.Transactor{}, unexpected), nil)
	_, err = svc.Required(ctx, true)
	require.Error(t, err)

	orgs = org.NewService(&fakes.OrgStore{Org: sys}, fakes.Transactor{}, unexpected)
	svc = NewService(fakes.Transactor{}, nil, orgs, member.NewService(&fakes.MemberStore{Err: errors.New("member store down")}, fakes.Transactor{}, unexpected), nil)
	_, err = svc.Required(ctx, true)
	require.NoError(t, err, "basic mode never counts owners")
	_, err = svc.Required(ctx, false)
	require.Error(t, err)
}
