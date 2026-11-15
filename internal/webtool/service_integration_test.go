//go:build integration

package webtool_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/webtool"
)

func TestSetSecret_UpsertOpensAfterReplace(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	svc := webtool.NewService(f.store, nil).WithSealer(testSealer(t))

	first, err := svc.SetSecret(ctx, projectID, "api.example.com", "X-Api-Key", "one")
	require.NoError(t, err)
	second, err := svc.SetSecret(ctx, projectID, "api.example.com", "x-api-key", "two")
	require.NoError(t, err)
	assert.Equal(t, first.PublicID, second.PublicID)

	open, unsealable, err := svc.OpenSecrets(ctx, projectID, []string{"api.example.com"})
	require.NoError(t, err)
	assert.Zero(t, unsealable)
	assert.Equal(t, []webtool.OpenSecret{{Host: "api.example.com", Header: "x-api-key", Value: "two"}}, open)
}

func TestSetProvider_KeySurvivesAPriceEdit(t *testing.T) {
	f := newPgFixture(t)
	orgID, projectID := f.seedScope(t)
	ctx := in(orgID)
	svc := webtool.NewService(f.store, nil).WithSealer(testSealer(t))

	_, err := svc.SetProvider(ctx, projectID, webtool.KindExa, new("exa-key"), 0)
	require.NoError(t, err)
	_, err = svc.SetProvider(ctx, projectID, webtool.KindExa, nil, 7000)
	require.NoError(t, err)
	open, err := svc.OpenSearchKey(ctx, projectID)
	require.NoError(t, err)
	assert.Equal(t, webtool.OpenProvider{Kind: webtool.KindExa, APIKey: "exa-key", PricePerCallMicros: 7000}, open)
}
