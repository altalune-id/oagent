//go:build integration

package knowledge_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/knowledge"
)

// SECURITY: a settings row in another org's name for this project meets the upsert's org guard and is refused, never saved silently.
func TestStore_SettingsOfAnotherOrgsProjectAreRefused(t *testing.T) {
	e := newEnv(t)
	provider := e.provider(t)
	mine := &knowledge.Settings{OrgID: e.org, ProjectID: e.prj, ProviderID: provider, Model: "m", Dims: 1536, UpdatedAt: time.Now()}
	require.NoError(t, e.store.SaveSettings(e.ctx, mine))

	forged := &knowledge.Settings{OrgID: e.otherOrg, ProjectID: e.prj, ProviderID: provider, Model: "forged", Dims: 768, UpdatedAt: time.Now()}
	err := e.store.SaveSettings(e.other, forged)
	assert.True(t, knowledge.IsSourceNotFoundError(err), "got %T: %v", err, err)
	got, err := e.store.Settings(e.ctx, e.prj)
	require.NoError(t, err)
	assert.Equal(t, "m", got.Model, "the other org's setting is untouched")
}
