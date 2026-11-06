//go:build integration

package boot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/mcpserver"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/scope"
)

type grantScope struct {
	ctx              context.Context
	orgID, projectID uuid.UUID
	providers        *modelprovider.Service
	servers          *mcpserver.Service
	agents           *agent.Service
}

func newGrantScope(t *testing.T, f *apiFixture) grantScope {
	t.Helper()
	o, err := org.NewOrg("org-"+uuid.NewString()[:8], "Org", uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.svcs.OrgStore.Save(tenant.WithOrg(t.Context(), o.ID), o))
	p, err := f.svcs.Projects.Create(t.Context(), project.CreateRequest{OrgID: o.ID, Slug: "main", Name: "Main"})
	require.NoError(t, err)
	return grantScope{
		ctx: tenant.Into(t.Context(), tenant.Context{OrgID: o.ID, ProjectID: p.ID}), orgID: o.ID, projectID: p.ID,
		providers: f.svcs.ModelProviders, servers: f.svcs.MCPServers, agents: f.svcs.Agents,
	}
}

func (g grantScope) provider(t *testing.T, name string) *modelprovider.Provider {
	t.Helper()
	p, err := g.providers.Create(g.ctx, g.projectID, modelprovider.Input{
		Name: name, Kind: modelprovider.KindOpenAICompatible, BaseURL: "https://example.com/v1",
		Models: []modelprovider.Model{{ID: "m", Prices: &modelprovider.Prices{InputMicros: 1, OutputMicros: 1}}},
	}, "sk-test")
	require.NoError(t, err)
	return p
}

func (g grantScope) agentUsing(t *testing.T, slug string, cfg agent.Config) *agent.Agent {
	t.Helper()
	a, err := g.agents.Create(g.ctx, agent.CreateRequest{OrgID: g.orgID, ProjectID: g.projectID, Slug: slug, Name: slug})
	require.NoError(t, err)
	_, err = g.agents.UpdateDraft(g.ctx, a.ID, cfg)
	require.NoError(t, err)
	return a
}

func codeOfErr(err error) apperror.Code {
	if e, ok := errors.AsType[interface {
		error
		ToAppError() *apperror.AppError
	}](err); ok {
		return e.ToAppError().Code()
	}
	if ae, ok := apperror.AsAppError(err); ok {
		return ae.Code()
	}
	return ""
}

func TestDeleteProviderUsedByPublishedVersion(t *testing.T) {
	f := newAPIFixture(t)
	g := newGrantScope(t, f)
	p := g.provider(t, "used")
	a := g.agentUsing(t, "bot", agent.Config{ProviderID: p.ID, Model: "m", MaxCostCents: 10})
	_, _, _, err := g.agents.Publish(g.ctx, a.ID, uuid.Nil)
	require.NoError(t, err)

	err = g.providers.Delete(g.ctx, p.ID)
	assert.Equal(t, apperror.CodeModelProviderInUse, codeOfErr(err), "got %v", err)
	_, err = g.providers.Update(g.ctx, p.ID, modelprovider.Input{
		Name: "used", Kind: modelprovider.KindOpenAICompatible, BaseURL: "https://example.com/v1",
		Models: []modelprovider.Model{{ID: "m"}},
	}, nil)
	assert.Equal(t, apperror.CodeModelProviderInUse, codeOfErr(err), "dropping the price a capped agent needs: %v", err)

	unused := g.provider(t, "unused")
	require.NoError(t, g.providers.Delete(g.ctx, unused.ID), "a provider only a draft names can go")
}

func TestDeleteMCPServerUsedByPublishedVersion(t *testing.T) {
	f := newAPIFixture(t)
	g := newGrantScope(t, f)
	p := g.provider(t, "p")
	s, err := g.servers.Create(g.ctx, g.projectID, mcpserver.Input{Name: "yasaku", URL: "https://example.com/mcp", AuthKind: mcpserver.AuthNone, TimeoutMS: 5000}, "")
	require.NoError(t, err)
	a := g.agentUsing(t, "bot", agent.Config{ProviderID: p.ID, Model: "m", MCPServers: []agent.MCPGrant{{ServerID: s.ID, Tools: []agent.Tool{{Name: "t"}}}}})
	_, _, _, err = g.agents.Publish(g.ctx, a.ID, uuid.Nil)
	require.NoError(t, err)
	assert.Equal(t, apperror.CodeMcpServerInUse, codeOfErr(g.servers.Delete(g.ctx, s.ID)))
}

// SECURITY: a publish and a delete of its provider never both win, so no published version points at a deleted provider.
func TestPublishVsDeleteProvider_Race(t *testing.T) {
	f := newAPIFixture(t)
	g := newGrantScope(t, f)
	publishes, deletes := 0, 0
	for round := range 20 {
		p := g.provider(t, "race-"+uuid.NewString()[:8])
		a := g.agentUsing(t, "racer-"+uuid.NewString()[:8], agent.Config{ProviderID: p.ID, Model: "m"})
		start := make(chan struct{})
		var wg sync.WaitGroup
		var pubErr, delErr error
		wg.Go(func() {
			<-start
			_, _, _, pubErr = g.agents.Publish(g.ctx, a.ID, uuid.Nil)
		})
		wg.Go(func() {
			<-start
			delErr = g.providers.Delete(g.ctx, p.ID)
		})
		close(start)
		wg.Wait()

		require.True(t, (pubErr == nil) != (delErr == nil), "round %d: exactly one wins: publish %v, delete %v", round, pubErr, delErr)
		if pubErr == nil {
			publishes++
			assert.Equal(t, apperror.CodeModelProviderInUse, codeOfErr(delErr), "round %d: %v", round, delErr)
			_, err := g.providers.Get(g.ctx, p.ID)
			require.NoError(t, err, "round %d: a published version's provider still exists", round)
			continue
		}
		deletes++
		e, ok := errors.AsType[*agent.InvalidGrantError](pubErr)
		require.True(t, ok, "round %d: a publish that lost answers AGT007, got %v", round, pubErr)
		assert.Equal(t, apperror.CodeAgentInvalidGrant, e.ToAppError().Code())
		assert.Equal(t, "provider", e.ToAppError().Meta()["reason"], "never PRV001, MCS001 or GEN900")
		v, err := f.svcs.AgentStore.LatestVersion(g.ctx, a.ID)
		require.NoError(t, err)
		assert.Nil(t, v, "round %d: no version after a lost publish", round)
	}
	t.Logf("publish won %d rounds, delete won %d", publishes, deletes)
}

type pausingLocker struct {
	inner   agent.Locker
	locked  chan struct{}
	release chan struct{}
}

func (l pausingLocker) LockShared(ctx context.Context, providerIDs, serverIDs []uuid.UUID) error {
	err := l.inner.LockShared(ctx, providerIDs, serverIDs)
	close(l.locked)
	<-l.release
	return err
}

// A publish that holds its share lock makes a delete of its provider wait, then the delete finds the version and answers PRV005.
func TestPublishHoldingItsLockMakesTheDeleteWait(t *testing.T) {
	f := newAPIFixture(t)
	g := newGrantScope(t, f)
	p := g.provider(t, "held")
	a := g.agentUsing(t, "holder", agent.Config{ProviderID: p.ID, Model: "m"})

	locker := pausingLocker{
		inner:  grantLocker{providers: f.svcs.ModelProviderStore, servers: f.svcs.MCPServerStore},
		locked: make(chan struct{}), release: make(chan struct{}),
	}
	var releaseOnce sync.Once
	unlock := func() { releaseOnce.Do(func() { close(locker.release) }) }
	t.Cleanup(unlock)
	publisher := agent.NewService(f.svcs.AgentStore, db.NewTransactor(f.pool), apperror.NewReporter(nil, false).Unexpected).
		WithProviders(grantProviders{providers: f.svcs.ModelProviders}).WithMCPServers(grantServers{servers: f.svcs.MCPServers}).
		WithLocker(locker)

	published := make(chan error, 1)
	go func() {
		_, _, _, err := publisher.Publish(g.ctx, a.ID, uuid.Nil)
		published <- err
	}()
	<-locker.locked

	deleted := make(chan error, 1)
	go func() { deleted <- g.providers.Delete(g.ctx, p.ID) }()
	require.Eventually(t, func() bool {
		var waiting int
		err := f.db.QueryRowContext(t.Context(), `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock' AND query ILIKE '%model_providers%FOR UPDATE%'`).Scan(&waiting)
		return err == nil && waiting > 0
	}, 10*time.Second, 20*time.Millisecond, "the delete waits on the publish's share lock")
	select {
	case err := <-deleted:
		t.Fatalf("the delete returned while the publish held its lock: %v", err)
	default:
	}

	unlock()
	require.NoError(t, <-published)
	err := <-deleted
	assert.Equal(t, apperror.CodeModelProviderInUse, codeOfErr(err), "got %v", err)
	_, err = g.providers.Get(g.ctx, p.ID)
	require.NoError(t, err, "the provider stays")
}

// A delete that commits first leaves the publish nothing to lock, so the publish answers AGT007 provider and writes no version.
func TestDeleteCommittedFirstRefusesThePublish(t *testing.T) {
	f := newAPIFixture(t)
	g := newGrantScope(t, f)
	p := g.provider(t, "gone")
	a := g.agentUsing(t, "late", agent.Config{ProviderID: p.ID, Model: "m"})
	require.NoError(t, g.providers.Delete(g.ctx, p.ID), "only a draft names it")

	_, _, _, err := g.agents.Publish(g.ctx, a.ID, uuid.Nil)
	e, ok := errors.AsType[*agent.InvalidGrantError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, apperror.CodeAgentInvalidGrant, e.ToAppError().Code())
	assert.Equal(t, "provider", e.ToAppError().Meta()["reason"])
	v, err := f.svcs.AgentStore.LatestVersion(g.ctx, a.ID)
	require.NoError(t, err)
	assert.Nil(t, v)
}

func (g grantScope) server(t *testing.T, name string) *mcpserver.Server {
	t.Helper()
	s, err := g.servers.Create(g.ctx, g.projectID, mcpserver.Input{Name: name, URL: startProbeMCPServer(t), AuthKind: mcpserver.AuthNone, TimeoutMS: 5000}, "")
	require.NoError(t, err)
	return s
}

func TestGrantAdapters_ForeignOrgIsInvalidGrant(t *testing.T) {
	f := newAPIFixture(t)
	home, foreign := newGrantScope(t, f), newGrantScope(t, f)
	foreignProvider, foreignServer := foreign.provider(t, "theirs"), foreign.server(t, "theirs")
	providers := grantProviders{providers: f.svcs.ModelProviders}
	servers := grantServers{servers: f.svcs.MCPServers}
	locker := grantLocker{providers: f.svcs.ModelProviderStore, servers: f.svcs.MCPServerStore}

	_, err := providers.Models(home.ctx, foreign.projectID, foreignProvider.ID)
	assert.Equal(t, apperror.CodeAgentInvalidGrant, codeOfErr(err), "provider of another org, its own project id")
	_, err = servers.Exists(home.ctx, foreign.projectID, foreignServer.ID)
	assert.Equal(t, apperror.CodeAgentInvalidGrant, codeOfErr(err), "server of another org, its own project id")
	err = db.NewTransactor(f.pool).RunInTx(home.ctx, func(ctx context.Context) error {
		return locker.LockShared(ctx, []uuid.UUID{foreignProvider.ID}, nil)
	})
	assert.Equal(t, apperror.CodeAgentInvalidGrant, codeOfErr(err), "lock on another org's provider")
	err = db.NewTransactor(f.pool).RunInTx(home.ctx, func(ctx context.Context) error {
		return locker.LockShared(ctx, nil, []uuid.UUID{foreignServer.ID})
	})
	assert.Equal(t, apperror.CodeAgentInvalidGrant, codeOfErr(err), "lock on another org's server")
}

func TestGrantAdapters_OtherProjectIsInvalidGrant(t *testing.T) {
	f := newAPIFixture(t)
	home := newGrantScope(t, f)
	other, err := f.svcs.Projects.Create(home.ctx, project.CreateRequest{OrgID: home.orgID, Slug: "other", Name: "Other"})
	require.NoError(t, err)
	sibling := home
	sibling.projectID = other.ID
	sibling.ctx = tenant.Into(home.ctx, tenant.Context{OrgID: home.orgID, ProjectID: other.ID})
	siblingProvider, siblingServer := sibling.provider(t, "sibling"), sibling.server(t, "sibling")

	_, err = grantProviders{providers: f.svcs.ModelProviders}.Models(home.ctx, home.projectID, siblingProvider.ID)
	assert.Equal(t, apperror.CodeAgentInvalidGrant, codeOfErr(err))
	_, err = grantServers{servers: f.svcs.MCPServers}.Exists(home.ctx, home.projectID, siblingServer.ID)
	assert.Equal(t, apperror.CodeAgentInvalidGrant, codeOfErr(err))
}

func TestMCP_AgentScopedKeyCannotBindAProvider(t *testing.T) {
	f := newAPIFixture(t)
	home := newGrantScope(t, f)
	p := home.provider(t, "project-credential")
	a := home.agentUsing(t, "scoped", agent.Config{Instructions: "Be brief."})
	call := func(key string, providerID string) string {
		raw, err := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{
				"name": "agent_update_draft",
				"arguments": map[string]any{"id": a.PublicID, "config": map[string]any{
					"instructions": "Be brief.", "providerId": providerID, "model": "m",
				}},
			},
		})
		require.NoError(t, err)
		status, _, body := f.mcpCall(t, key, string(raw))
		require.Equal(t, http.StatusOK, status, string(body))
		return string(body)
	}

	scoped := f.mintKey(t, apikey.CreateRequest{
		OrgID: home.orgID, Kind: apikey.KindOrg, Scopes: []string{scope.AgentRead, scope.AgentWrite},
		ProjectIDs: []uuid.UUID{home.projectID}, ResourceIDs: []uuid.UUID{a.ID},
	})
	refused := call(scoped, p.PublicID)
	assert.Contains(t, refused, `"isError":true`)
	assert.Contains(t, refused, string(apperror.CodeAgentInvalidGrant))

	granted := f.mintKey(t, apikey.CreateRequest{
		OrgID: home.orgID, Kind: apikey.KindOrg, Scopes: []string{scope.AgentRead, scope.AgentWrite, scope.ProviderRead},
		ProjectIDs: []uuid.UUID{home.projectID}, ResourceIDs: []uuid.UUID{a.ID, p.ID},
	})
	assert.NotContains(t, call(granted, p.PublicID), `"isError":true`)
}
