package agent_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/agent"
	"altalune.id/oagent/internal/testutil/fakes"
)

type grantsFixture struct {
	agent    *agent.Agent
	store    *fakes.AgentStore
	grants   *fakes.AgentGrants
	tx       *fakes.CountingTransactor
	svc      *agent.Service
	provider uuid.UUID
	server   uuid.UUID
}

func newGrantsFixture(t *testing.T) *grantsFixture {
	t.Helper()
	a, err := agent.New(uuid.New(), uuid.New(), "support", "Support", "", uuid.New())
	require.NoError(t, err)
	f := &grantsFixture{agent: a, store: &fakes.AgentStore{Agent: a}, tx: &fakes.CountingTransactor{}, provider: uuid.New(), server: uuid.New()}
	f.grants = &fakes.AgentGrants{
		Providers: map[uuid.UUID]fakes.GrantProvider{f.provider: {ProjectID: a.ProjectID, Models: []agent.ModelRef{
			{ID: "gpt-4o", Priced: true}, {ID: "mini", Priced: true}, {ID: "free"},
		}}},
		Servers: map[uuid.UUID]fakes.GrantServer{f.server: {ProjectID: a.ProjectID, Name: "yasaku"}},
	}
	f.svc = agent.NewService(f.store, f.tx, testUnexpected()).WithProviders(f.grants).WithMCPServers(f.grants).WithLocker(f.grants)
	return f
}

func (f *grantsFixture) config() agent.Config {
	return agent.Config{
		Instructions: "Be brief.", ProviderID: f.provider, Model: "gpt-4o", HelperModel: "mini", MaxCostCents: 100,
		MCPServers: []agent.MCPGrant{{ServerID: f.server, Tools: []agent.Tool{{Name: "record_expense", RequireApproval: true}}}},
	}
}

func grantReason(t *testing.T, err error) string {
	t.Helper()
	require.Error(t, err)
	e, ok := errors.AsType[*agent.InvalidGrantError](err)
	require.True(t, ok, "want AGT007, got %v", err)
	assert.Equal(t, apperror.CodeAgentInvalidGrant, e.ToAppError().Code())
	return e.ToAppError().Meta()["reason"]
}

func TestService_DraftAndPublishWithGrants(t *testing.T) {
	f := newGrantsFixture(t)
	ctx := context.Background()
	a, err := f.svc.UpdateDraft(ctx, f.agent.ID, f.config())
	require.NoError(t, err)
	assert.Equal(t, f.config(), a.Draft)

	_, v, created, err := f.svc.Publish(ctx, f.agent.ID, uuid.Nil)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, f.config(), v.Config)
	assert.Equal(t, [][]uuid.UUID{{f.provider, f.server}}, f.grants.Locked, "publish share-locks every reference")
}

func TestService_DraftRefusesBadReferences(t *testing.T) {
	for want, edit := range map[string]func(f *grantsFixture, c *agent.Config){
		"provider":     func(_ *grantsFixture, c *agent.Config) { c.ProviderID = uuid.New() },
		"model":        func(_ *grantsFixture, c *agent.Config) { c.Model = "gpt-9" },
		"helper_model": func(_ *grantsFixture, c *agent.Config) { c.HelperModel = "gpt-9" },
		"server":       func(_ *grantsFixture, c *agent.Config) { c.MCPServers[0].ServerID = uuid.New() },
		"tool_name": func(_ *grantsFixture, c *agent.Config) {
			c.MCPServers[0].Tools = []agent.Tool{{Name: strings.Repeat("t", 64-len("yasaku__")+1)}}
		},
		"legacy_tools": func(_ *grantsFixture, c *agent.Config) { c.Tools = []agent.Tool{{Name: "search"}} },
	} {
		t.Run(want, func(t *testing.T) {
			f := newGrantsFixture(t)
			c := f.config()
			edit(f, &c)
			_, err := f.svc.UpdateDraft(context.Background(), f.agent.ID, c)
			assert.Equal(t, want, grantReason(t, err))
			assert.Nil(t, f.store.Saved, "nothing saved")
		})
	}
}

func TestService_DraftRefusesAProviderOfAnotherProject(t *testing.T) {
	f := newGrantsFixture(t)
	other := uuid.New()
	f.grants.Providers[other] = fakes.GrantProvider{ProjectID: uuid.New(), Models: []agent.ModelRef{{ID: "gpt-4o"}}}
	c := f.config()
	c.ProviderID = other
	_, err := f.svc.UpdateDraft(context.Background(), f.agent.ID, c)
	assert.Equal(t, "provider", grantReason(t, err))
}

func TestService_DraftRefusesADottedGrantTool(t *testing.T) {
	f := newGrantsFixture(t)
	c := f.config()
	c.MCPServers[0].Tools = []agent.Tool{{Name: "a.b"}}
	_, err := f.svc.UpdateDraft(context.Background(), f.agent.ID, c)
	assert.Equal(t, "tool_name", grantReason(t, err))
}

func TestService_DraftMayLeaveTheProviderOut(t *testing.T) {
	f := newGrantsFixture(t)
	_, err := f.svc.UpdateDraft(context.Background(), f.agent.ID, agent.Config{Instructions: "later"})
	require.NoError(t, err)
}

func TestService_PublishLegacyIsNeedsRepublish(t *testing.T) {
	for name, cfg := range map[string]agent.Config{
		"no provider":     {Model: "gpt-4o"},
		"top-level tools": {Model: "gpt-4o", Tools: []agent.Tool{{Name: "x.y"}}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newGrantsFixture(t)
			f.agent.Draft = cfg
			if name == "top-level tools" {
				f.agent.Draft.ProviderID = f.provider
			}
			_, _, _, err := f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
			require.True(t, agent.IsNeedsRepublishError(err), "got %v", err)
			e, _ := errors.AsType[*agent.NeedsRepublishError](err)
			assert.Equal(t, apperror.CodeAgentNeedsRepublish, e.ToAppError().Code())
			assert.Empty(t, f.store.Published)
		})
	}
}

func TestService_PublishCappedUnpricedIsBudgetUnpriced(t *testing.T) {
	for name, edit := range map[string]func(*agent.Config){
		"model":        func(c *agent.Config) { c.Model = "free" },
		"helper model": func(c *agent.Config) { c.HelperModel = "free" },
	} {
		t.Run(name, func(t *testing.T) {
			f := newGrantsFixture(t)
			c := f.config()
			edit(&c)
			_, err := f.svc.UpdateDraft(context.Background(), f.agent.ID, c)
			require.NoError(t, err, "a draft may still name an unpriced model")
			_, _, _, err = f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
			assert.Equal(t, "budget_unpriced", grantReason(t, err))
			assert.Empty(t, f.store.Published)
		})
	}
}

func TestService_PublishUncappedUnpricedIsFine(t *testing.T) {
	f := newGrantsFixture(t)
	c := f.config()
	c.Model, c.MaxCostCents = "free", 0
	_, err := f.svc.UpdateDraft(context.Background(), f.agent.ID, c)
	require.NoError(t, err)
	_, _, _, err = f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	require.NoError(t, err)
}

func TestService_PublishNeedsAModel(t *testing.T) {
	f := newGrantsFixture(t)
	c := f.config()
	c.Model = ""
	_, err := f.svc.UpdateDraft(context.Background(), f.agent.ID, c)
	require.NoError(t, err)
	_, _, _, err = f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	assert.Equal(t, "model", grantReason(t, err))
}

func TestService_PublishRevalidatesEveryReference(t *testing.T) {
	f := newGrantsFixture(t)
	_, err := f.svc.UpdateDraft(context.Background(), f.agent.ID, f.config())
	require.NoError(t, err)
	delete(f.grants.Servers, f.server)
	_, _, _, err = f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	assert.Equal(t, "server", grantReason(t, err))
	assert.Empty(t, f.store.Published)
}

func TestService_PublishLocksInsideTheUnitOfWork(t *testing.T) {
	f := newGrantsFixture(t)
	_, err := f.svc.UpdateDraft(context.Background(), f.agent.ID, f.config())
	require.NoError(t, err)
	calls := f.tx.Calls
	_, _, _, err = f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	require.NoError(t, err)
	assert.Equal(t, calls+1, f.tx.Calls, "one unit of work")
	assert.Len(t, f.grants.Locked, 1, "the fake locker refuses a lock outside a unit of work")
}

func TestService_GrantPortsFailClosed(t *testing.T) {
	a, err := agent.New(uuid.New(), uuid.New(), "support", "Support", "", uuid.New())
	require.NoError(t, err)
	svc := agent.NewService(&fakes.AgentStore{Agent: a}, fakes.Transactor{}, testUnexpected())
	_, err = svc.UpdateDraft(context.Background(), a.ID, agent.Config{ProviderID: uuid.New(), Model: "m"})
	assert.Equal(t, "provider", grantReason(t, err), "no provider port answers like an unknown provider")
	_, err = svc.UpdateDraft(context.Background(), a.ID, agent.Config{MCPServers: []agent.MCPGrant{{ServerID: uuid.New()}}})
	assert.Equal(t, "server", grantReason(t, err))
}

func TestService_GrantPortFailureIsUnexpected(t *testing.T) {
	f := newGrantsFixture(t)
	f.grants.Err = assert.AnError
	_, err := f.svc.UpdateDraft(context.Background(), f.agent.ID, f.config())
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperror.CodeUnexpected, ae.Code())
}

func TestService_PublishWithoutALockerFailsClosed(t *testing.T) {
	f := newGrantsFixture(t)
	svc := agent.NewService(f.store, f.tx, testUnexpected()).WithProviders(f.grants).WithMCPServers(f.grants)
	_, err := svc.UpdateDraft(context.Background(), f.agent.ID, f.config())
	require.NoError(t, err)
	_, _, _, err = svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, apperror.CodeUnexpected, ae.Code(), "an unlocked publish never happens")
	assert.Empty(t, f.store.Published)
}

func TestService_DraftVersionHasPublishBlockers(t *testing.T) {
	for name, edit := range map[string]func(*grantsFixture, *agent.Config){
		"legacy":           func(_ *grantsFixture, c *agent.Config) { c.ProviderID = uuid.Nil },
		"no model":         func(_ *grantsFixture, c *agent.Config) { c.Model = "" },
		"capped unpriced":  func(_ *grantsFixture, c *agent.Config) { c.Model = "free" },
		"server gone":      func(f *grantsFixture, _ *agent.Config) { delete(f.grants.Servers, f.server) },
		"provider unknown": func(f *grantsFixture, _ *agent.Config) { delete(f.grants.Providers, f.provider) },
	} {
		t.Run(name, func(t *testing.T) {
			f := newGrantsFixture(t)
			c := f.config()
			f.agent.Draft = c
			edit(f, &f.agent.Draft)
			_, _, _, publishErr := f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
			require.Error(t, publishErr)
			_, err := f.svc.DraftVersion(context.Background(), f.agent.ID, uuid.Nil)
			require.Error(t, err)
			want, _ := apperror.AsAppError(publishErr)
			got, ok := apperror.AsAppError(err)
			require.True(t, ok, "got %v", err)
			assert.Equal(t, want.Code(), got.Code(), "the same AGT code as publish")
			assert.Equal(t, want.Meta(), got.Meta(), "and the same field")
			assert.Empty(t, f.store.Drafts, "a refused try run pins nothing")
		})
	}
}

func TestService_DraftVersionPinsAndReusesTheDraft(t *testing.T) {
	f := newGrantsFixture(t)
	_, err := f.svc.UpdateDraft(context.Background(), f.agent.ID, f.config())
	require.NoError(t, err)
	by := uuid.New()
	calls := f.tx.Calls
	v, err := f.svc.DraftVersion(context.Background(), f.agent.ID, by)
	require.NoError(t, err)
	assert.Equal(t, calls+1, f.tx.Calls, "one unit of work")
	assert.Len(t, f.grants.Locked, 1, "the references stay locked while the draft is pinned")
	assert.Equal(t, agent.VersionDraft, v.Kind)
	assert.Equal(t, f.config(), v.Config)
	assert.Equal(t, by, v.CreatedBy.UUID)
	assert.Empty(t, f.store.Published, "a draft pin is never a published version")
	again, err := f.svc.DraftVersion(context.Background(), f.agent.ID, by)
	require.NoError(t, err)
	assert.Equal(t, v.ID, again.ID, "an unchanged draft reuses its row")
}

func newKnowledgeFixture(t *testing.T) (*grantsFixture, uuid.UUID) {
	t.Helper()
	f := newGrantsFixture(t)
	src := uuid.New()
	f.grants.Sources = map[uuid.UUID]uuid.UUID{src: f.agent.ProjectID}
	f.svc.WithKnowledge(f.grants)
	return f, src
}

func (f *grantsFixture) withTools(c agent.Config, tools ...agent.Tool) agent.Config {
	c.MCPServers = []agent.MCPGrant{{ServerID: f.server, Tools: tools}}
	return c
}

func (f *grantsFixture) published(t *testing.T, c agent.Config) {
	t.Helper()
	f.store.Published = []*agent.Version{{ID: uuid.New(), AgentID: f.agent.ID, Version: 1, Config: c, Kind: agent.VersionPublished}}
}

func refusedTool(t *testing.T, err error) string {
	t.Helper()
	assert.Equal(t, "unapproved_tool", grantReason(t, err))
	e, _ := errors.AsType[*agent.InvalidGrantError](err)
	return e.ToAppError().Meta()["name"]
}

func TestService_KnowledgeWithAnUnapprovedToolIsRefused(t *testing.T) {
	f, src := newKnowledgeFixture(t)
	f.agent.Draft = f.withTools(f.config(), agent.Tool{Name: "send"})
	f.agent.Draft.KnowledgeSourceIDs = []uuid.UUID{src}

	_, _, _, err := f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	assert.Equal(t, "yasaku__send", refusedTool(t, err))
	assert.Empty(t, f.store.Published, "nothing is published")
	_, err = f.svc.DraftVersion(context.Background(), f.agent.ID, uuid.Nil)
	assert.Equal(t, "yasaku__send", refusedTool(t, err), "Try it refuses the same draft")
	assert.Empty(t, f.store.Drafts)
}

func TestService_KnowledgeWithApprovedOrConditionalToolsPublishes(t *testing.T) {
	f, src := newKnowledgeFixture(t)
	f.agent.Draft = f.withTools(f.config(), agent.Tool{Name: "a", RequireApproval: true},
		agent.Tool{Name: "b", RequireApproval: true, FreeWhen: []agent.FreeCondition{{Arg: "x", Absent: true}}})
	f.agent.Draft.KnowledgeSourceIDs = []uuid.UUID{src}
	_, _, _, err := f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	require.NoError(t, err)
}

func TestService_UnapprovedToolWithoutKnowledgePublishes(t *testing.T) {
	f, _ := newKnowledgeFixture(t)
	f.agent.Draft = f.withTools(f.config(), agent.Tool{Name: "send"})
	_, _, _, err := f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	require.NoError(t, err)
}

func TestService_GrandfatheredAgentRepublishesAnInstructionsChange(t *testing.T) {
	f, src := newKnowledgeFixture(t)
	live := f.withTools(f.config(), agent.Tool{Name: "send"})
	live.KnowledgeSourceIDs = []uuid.UUID{src}
	f.published(t, live)
	f.agent.Draft = live
	f.agent.Draft.Instructions = "Be briefer."
	_, v, created, err := f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "Be briefer.", v.Config.Instructions)
	_, err = f.svc.DraftVersion(context.Background(), f.agent.ID, uuid.Nil)
	require.NoError(t, err, "Try it uses the same grandfathered tools")
}

func TestService_GrandfatheredAgentAddingAnUnapprovedToolIsRefused(t *testing.T) {
	f, src := newKnowledgeFixture(t)
	live := f.withTools(f.config(), agent.Tool{Name: "send"})
	live.KnowledgeSourceIDs = []uuid.UUID{src}
	f.published(t, live)
	f.agent.Draft = f.withTools(live, agent.Tool{Name: "send"}, agent.Tool{Name: "delete"})
	_, _, _, err := f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	assert.Equal(t, "yasaku__delete", refusedTool(t, err))
}

func TestService_TurningKnowledgeOnOverAnUnapprovedToolIsRefused(t *testing.T) {
	f, src := newKnowledgeFixture(t)
	live := f.withTools(f.config(), agent.Tool{Name: "send"})
	f.published(t, live)
	f.agent.Draft = live
	f.agent.Draft.KnowledgeSourceIDs = []uuid.UUID{src}
	_, _, _, err := f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	assert.Equal(t, "yasaku__send", refusedTool(t, err))
}

func TestService_FixedAgentCannotRegress(t *testing.T) {
	f, src := newKnowledgeFixture(t)
	live := f.withTools(f.config(), agent.Tool{Name: "send", RequireApproval: true})
	live.KnowledgeSourceIDs = []uuid.UUID{src}
	f.published(t, live)
	f.agent.Draft = f.withTools(live, agent.Tool{Name: "send"})
	_, _, _, err := f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	assert.Equal(t, "yasaku__send", refusedTool(t, err))
}

func TestService_ServerRenameKeepsTheGrandfather(t *testing.T) {
	f, src := newKnowledgeFixture(t)
	live := f.withTools(f.config(), agent.Tool{Name: "send"})
	live.KnowledgeSourceIDs = []uuid.UUID{src}
	f.published(t, live)
	f.grants.Servers[f.server] = fakes.GrantServer{ProjectID: f.agent.ProjectID, Name: "renamed"}
	f.agent.Draft = live
	f.agent.Draft.Instructions = "Be briefer."
	_, _, _, err := f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	require.NoError(t, err, "tools are keyed by server id, so a rename grandfathers nothing new and refuses nothing")
}

func newMemoryFixture(t *testing.T) (*grantsFixture, uuid.UUID) {
	t.Helper()
	f := newGrantsFixture(t)
	store := uuid.New()
	f.grants.Stores = map[uuid.UUID]uuid.UUID{store: f.agent.ProjectID}
	f.svc.WithMemory(f.grants)
	return f, store
}

func TestService_MemoryStoreMustBeTheProjects(t *testing.T) {
	f, store := newMemoryFixture(t)
	for name, id := range map[string]uuid.UUID{"unknown": uuid.New(), "another project's": func() uuid.UUID {
		other := uuid.New()
		f.grants.Stores[other] = uuid.New()
		return other
	}()} {
		c := f.config()
		c.MemoryStoreID = id
		_, err := f.svc.UpdateDraft(context.Background(), f.agent.ID, c)
		assert.Equal(t, "memory_store", grantReason(t, err), "%s store on save", name)
		f.agent.Draft = c
		_, _, _, err = f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
		assert.Equal(t, "memory_store", grantReason(t, err), "%s store on publish", name)
	}
	c := f.config()
	c.MemoryStoreID = store
	_, err := f.svc.UpdateDraft(context.Background(), f.agent.ID, c)
	require.NoError(t, err)
	f.agent.Draft = c
	f.grants.LockedStores = nil
	_, _, _, err = f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{store}, f.grants.LockedStores, "publish share-locks the store so it cannot be deleted under the version")
}

func TestService_MemoryStoreOnADisabledBackendIsRefused(t *testing.T) {
	f, store := newMemoryFixture(t)
	f.grants.DisabledStores = map[uuid.UUID]bool{store: true}
	c := f.config()
	c.MemoryStoreID = store
	_, err := f.svc.UpdateDraft(context.Background(), f.agent.ID, c)
	assert.Equal(t, "memory_store_disabled", grantReason(t, err))
	f.agent.Draft = c
	_, _, _, err = f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	assert.Equal(t, "memory_store_disabled", grantReason(t, err))
}

func TestService_MemoryStoreWithoutAMemoryPortIsRefused(t *testing.T) {
	f := newGrantsFixture(t)
	c := f.config()
	c.MemoryStoreID = uuid.New()
	_, err := f.svc.UpdateDraft(context.Background(), f.agent.ID, c)
	assert.Equal(t, "memory_store", grantReason(t, err))
}

func TestService_AStoreGoneBeforePublishIsRefused(t *testing.T) {
	f, store := newMemoryFixture(t)
	c := f.config()
	c.MemoryStoreID = store
	f.agent.Draft = c
	delete(f.grants.Stores, store)
	_, _, _, err := f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	assert.Equal(t, "memory_store", grantReason(t, err))
	assert.Empty(t, f.grants.LockedStores)
}

func TestService_AMemoryStoreRefusesAnUnapprovedTool(t *testing.T) {
	f, store := newMemoryFixture(t)
	c := f.withTools(f.config(), agent.Tool{Name: "send"})
	c.MemoryStoreID = store
	f.agent.Draft = c
	_, _, _, err := f.svc.Publish(context.Background(), f.agent.ID, uuid.Nil)
	assert.Equal(t, "yasaku__send", refusedTool(t, err), "remembered facts are untrusted input")
}
