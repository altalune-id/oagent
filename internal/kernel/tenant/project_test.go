package tenant

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
)

type resolver map[uuid.UUID]uuid.UUID

func (r resolver) OrgOf(_ context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	if org, ok := r[projectID]; ok {
		return org, nil
	}
	return uuid.Nil, apperror.New(apperror.CodeProjectNotFound, "project not found")
}

type failing struct{}

func (failing) OrgOf(context.Context, uuid.UUID) (uuid.UUID, error) {
	return uuid.Nil, errors.New("db down")
}

func TestRequireProjectInOrg(t *testing.T) {
	orgA, orgB, prjA, prjB := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	r := resolver{prjA: orgA, prjB: orgB}
	ctx := context.Background()

	require.NoError(t, RequireProjectInOrg(ctx, r, orgA, prjA))
	assert.True(t, IsProjectOutsideOrgError(RequireProjectInOrg(ctx, r, orgA, prjB)), "another org's project")
	assert.True(t, IsProjectOutsideOrgError(RequireProjectInOrg(ctx, r, orgA, uuid.New())), "a missing project looks the same")
	assert.NoError(t, RequireProjectInOrg(ctx, nil, orgA, prjB), "no resolver, no check")

	err := RequireProjectInOrg(ctx, failing{}, orgA, prjA)
	require.Error(t, err)
	assert.False(t, IsProjectOutsideOrgError(err))

	ae := (&ProjectOutsideOrgError{}).ToAppError()
	assert.Equal(t, apperror.CodeProjectNotInOrg, ae.Code())
	assert.Equal(t, apperror.KindInvalid, apperror.KindOf(ae.Code()))
}
