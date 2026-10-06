package tenant_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/tenant"
)

func TestFrom_Missing(t *testing.T) {
	_, err := tenant.From(context.Background())
	assert.True(t, tenant.IsMissingError(err))
}

func TestFrom_Unscoped(t *testing.T) {
	ctx := tenant.Into(context.Background(), tenant.Context{})
	_, err := tenant.From(ctx)
	assert.True(t, tenant.IsUnscopedError(err))
}

func TestRoundTrip(t *testing.T) {
	org, project, user := uuid.New(), uuid.New(), uuid.New()
	ctx := tenant.Into(context.Background(), tenant.Context{OrgID: org, ProjectID: project, UserID: user})

	tc, err := tenant.From(ctx)
	require.NoError(t, err)
	assert.Equal(t, org, tc.OrgID)
	assert.Equal(t, project, tc.ProjectID)
	assert.Equal(t, user, tc.UserID)
}

func TestWithProjectOrg(t *testing.T) {
	ctx := context.Background()
	pid, oid := uuid.New(), uuid.New()

	tc, err := tenant.From(tenant.WithProject(tenant.WithOrg(ctx, oid), pid))
	require.NoError(t, err)
	assert.Equal(t, oid, tc.OrgID)
	assert.Equal(t, pid, tc.ProjectID)
}
