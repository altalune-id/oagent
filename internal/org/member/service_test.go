package member_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/testutil/fakes"
)

func newService(store member.Store) *member.Service {
	return member.NewService(store, apperror.NewReporter(nil, false).Unexpected)
}

func TestService_AddFindList(t *testing.T) {
	svc := newService(&fakes.MemberStore{})
	ctx := context.Background()
	usr, orgA, orgB := uuid.New(), uuid.New(), uuid.New()

	m, err := svc.Add(ctx, orgA, usr, member.RoleOwner)
	require.NoError(t, err)
	assert.Equal(t, member.RoleOwner, m.Role)
	_, err = svc.Add(ctx, orgB, usr, member.RoleMember)
	require.NoError(t, err)

	got, err := svc.Find(ctx, orgA, usr)
	require.NoError(t, err)
	assert.Equal(t, m.ID, got.ID)

	list, err := svc.ListByUser(ctx, usr)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestService_ExpectedErrors(t *testing.T) {
	svc := newService(&fakes.MemberStore{})
	ctx := context.Background()
	org, usr := uuid.New(), uuid.New()

	_, err := svc.Add(ctx, org, usr, "root")
	assert.True(t, member.IsInvalidRoleError(err))
	_, err = svc.Add(ctx, org, usr, member.RoleAdmin)
	require.NoError(t, err)
	_, err = svc.Add(ctx, org, usr, member.RoleMember)
	assert.True(t, member.IsAlreadyMemberError(err))
	_, err = svc.Find(ctx, org, uuid.New())
	assert.True(t, member.IsNotFoundError(err))
}

func TestService_StoreFailureIsReported(t *testing.T) {
	svc := newService(&fakes.MemberStore{Err: errors.New("db down")})
	ctx := context.Background()
	isUnexpected := func(err error) bool {
		ae, ok := errors.AsType[*apperror.AppError](err)
		return ok && ae.Code() == apperror.CodeUnexpected
	}
	_, err := svc.Add(ctx, uuid.New(), uuid.New(), member.RoleOwner)
	assert.True(t, isUnexpected(err), "got %v", err)
	_, err = svc.Find(ctx, uuid.New(), uuid.New())
	assert.True(t, isUnexpected(err))
	_, err = svc.ListByUser(ctx, uuid.New())
	assert.True(t, isUnexpected(err))
}

func TestService_CountOwners(t *testing.T) {
	svc := newService(&fakes.MemberStore{})
	ctx := context.Background()
	orgID := uuid.New()

	n, err := svc.CountOwners(ctx, orgID)
	require.NoError(t, err)
	assert.Zero(t, n)

	_, err = svc.Add(ctx, orgID, uuid.New(), member.RoleOwner)
	require.NoError(t, err)
	_, err = svc.Add(ctx, orgID, uuid.New(), member.RoleAdmin)
	require.NoError(t, err)
	n, err = svc.CountOwners(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}
