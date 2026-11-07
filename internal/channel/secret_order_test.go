package channel_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/channel"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/testutil/fakes"
)

func secretlessService(t *testing.T) (*channel.Service, *fakes.ChannelStore, context.Context, uuid.UUID, uuid.UUID) {
	t.Helper()
	refs := fakes.NewPublicIDs()
	org, project, agent := uuid.New(), uuid.New(), uuid.New()
	refs.Org(org)
	refs.Project(project, org)
	refs.Resource(agent, org)
	store := &fakes.ChannelStore{}
	svc := channel.NewService(store, fakes.Transactor{}, refs, apperror.NewReporter(nil, false).Unexpected).WithSealer(testSealer(t))
	return svc, store, tenant.Into(t.Context(), tenant.Context{OrgID: org, ProjectID: project}), project, agent
}

func TestService_CreateWithoutASecretWaitsForOne(t *testing.T) {
	svc, _, ctx, project, agent := secretlessService(t)
	c, err := svc.Create(ctx, project, agent, input(), "owk_device")
	require.NoError(t, err, "OpenWA mints the secret only after it has the webhook URL, so a connection starts without one")
	assert.Empty(t, c.SigningPrimarySealed)
	assert.NotEmpty(t, c.DeviceKeySealed)
	assert.Equal(t, "waiting", c.ReportedStatus())

	_, err = svc.Create(ctx, project, agent, input(), "")
	assert.True(t, channel.IsSecretRequiredError(err), "the device key is still required: %v", err)
}

func TestService_FirstSecretSetsThePrimaryOnly(t *testing.T) {
	svc, store, ctx, project, agent := secretlessService(t)
	c, err := svc.Create(ctx, project, agent, input(), "owk_device")
	require.NoError(t, err)
	c, err = svc.SetSecret(ctx, c.ID, "whsec_from_openwa", true)
	require.NoError(t, err)
	assert.NotEmpty(t, store.Connections[c.ID].SigningPrimarySealed)
	assert.Empty(t, store.Connections[c.ID].SigningSecondarySealed, "there is no previous secret to keep")
	secrets, err := svc.OpenSecrets(ctx, store.Connections[c.ID])
	require.NoError(t, err)
	assert.Equal(t, []string{"whsec_from_openwa"}, secrets.Signing)
}

func create(ctx context.Context, svc *channel.Service, project, agent uuid.UUID, in channel.Input, secret, key string) (*channel.Connection, error) {
	c, err := svc.Create(ctx, project, agent, in, key)
	if err != nil || secret == "" {
		return c, err
	}
	return svc.SetSecret(ctx, c.ID, secret, false)
}

func TestService_SetSecretKeepsOrDropsThePrevious(t *testing.T) {
	svc, store, ctx, project, agent := secretlessService(t)
	c, err := create(ctx, svc, project, agent, input(), "whsec_a", "owk_device")
	require.NoError(t, err)
	_, err = svc.SetSecret(ctx, c.ID, "whsec_b", false)
	require.NoError(t, err)
	secrets, err := svc.OpenSecrets(ctx, store.Connections[c.ID])
	require.NoError(t, err)
	assert.Equal(t, []string{"whsec_b", "whsec_a"}, secrets.Signing, "the zero value keeps the old secret as secondary")

	_, err = svc.SetSecret(ctx, c.ID, "whsec_b", true)
	require.NoError(t, err)
	secrets, err = svc.OpenSecrets(ctx, store.Connections[c.ID])
	require.NoError(t, err)
	assert.Equal(t, []string{"whsec_b"}, secrets.Signing, "drop_previous retires the old secret")
	assert.Empty(t, store.Connections[c.ID].SigningSecondarySealed)

	_, err = svc.SetSecret(ctx, c.ID, "  ", true)
	assert.True(t, channel.IsSecretRequiredError(err), "nothing clears the primary: %v", err)
	assert.NotEmpty(t, store.Connections[c.ID].SigningPrimarySealed)
}
