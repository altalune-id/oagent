package modelprovider_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/sealer"
)

const apiKey = "sk-live-secret"

func testUnexpected() apperror.UnexpectedFunc {
	return apperror.NewReporter(nil, false).Unexpected
}

func testSealer(t *testing.T) sealer.Sealer {
	t.Helper()
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	sl, err := sealer.New(key)
	require.NoError(t, err)
	return sl
}

func input() modelprovider.Input {
	return modelprovider.Input{
		Name:    "openrouter",
		Kind:    modelprovider.KindOpenAICompatible,
		BaseURL: "https://openrouter.ai/api/v1",
		Models: []modelprovider.Model{
			{ID: "gpt-4o", Vision: true, Prices: &modelprovider.Prices{InputMicros: 2_500_000, OutputMicros: 10_000_000}},
			{ID: "llama-3"},
		},
	}
}

type fixture struct {
	store *fakes.ModelProviderStore
	usage *fakes.ModelProviderUsage
	tx    *fakes.CountingTransactor
	svc   *modelprovider.Service
	ctx   context.Context
	orgID uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		store: &fakes.ModelProviderStore{},
		usage: &fakes.ModelProviderUsage{},
		tx:    &fakes.CountingTransactor{},
		orgID: uuid.New(),
	}
	f.ctx = tenant.Into(context.Background(), tenant.Context{OrgID: f.orgID})
	f.svc = modelprovider.NewService(f.store, f.tx, testUnexpected()).WithSealer(testSealer(t)).WithUsage(f.usage)
	return f
}

func (f *fixture) create(t *testing.T) *modelprovider.Provider {
	t.Helper()
	p, err := f.svc.Create(f.ctx, uuid.New(), input(), apiKey)
	require.NoError(t, err)
	return p
}

func codeOf(t *testing.T, err error) apperror.Code {
	t.Helper()
	require.Error(t, err)
	e, ok := err.(interface{ ToAppError() *apperror.AppError })
	require.True(t, ok, "%T has no ToAppError: %v", err, err)
	return e.ToAppError().Code()
}

func reasonOf(t *testing.T, err error) string {
	t.Helper()
	e, ok := err.(interface{ ToAppError() *apperror.AppError })
	require.True(t, ok, "%T has no ToAppError: %v", err, err)
	return e.ToAppError().Meta()["reason"]
}

func TestService_CreateSealsAndNeverStoresPlainText(t *testing.T) {
	f := newFixture(t)
	projectID := uuid.New()
	p, err := f.svc.Create(f.ctx, projectID, input(), apiKey)
	require.NoError(t, err)
	assert.Equal(t, f.orgID, p.OrgID)
	assert.Equal(t, projectID, p.ProjectID)

	saved := f.store.Providers[p.ID]
	require.NotNil(t, saved)
	require.NotEmpty(t, saved.APIKeySealed)
	assert.False(t, bytes.Contains(saved.APIKeySealed, []byte(apiKey)), "the stored key is sealed")

	got, err := f.svc.ResolveModel(f.ctx, p.ID, "gpt-4o")
	require.NoError(t, err)
	assert.Equal(t, apiKey, got.APIKey)
}

func TestService_CreateWithoutKeyIsSecretRequired(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Create(f.ctx, uuid.New(), input(), "")
	assert.Equal(t, apperror.CodeModelProviderSecretRequired, codeOf(t, err))
	assert.Empty(t, f.store.Providers)
}

func TestService_CreateNeedsATenant(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Create(context.Background(), uuid.New(), input(), apiKey)
	assert.Error(t, err)
	assert.Empty(t, f.store.Providers)
}

func TestService_CreateInvalidInputSavesNothing(t *testing.T) {
	f := newFixture(t)
	in := input()
	in.BaseURL = "http://llm.example.com"
	_, err := f.svc.Create(f.ctx, uuid.New(), in, apiKey)
	assert.Equal(t, apperror.CodeModelProviderInvalidBaseURL, codeOf(t, err))
	assert.Empty(t, f.store.Providers)
}

func TestService_AllowInsecureURL(t *testing.T) {
	f := newFixture(t)
	f.svc.WithAllowInsecureURL()
	in := input()
	in.BaseURL = "http://127.0.0.1:11434/v1"
	_, err := f.svc.Create(f.ctx, uuid.New(), in, apiKey)
	require.NoError(t, err)
}

func TestService_DisabledSealerIsSecretsUnavailable(t *testing.T) {
	f := newFixture(t)
	p := f.create(t)
	f.svc.WithSealer(sealer.Disabled())

	_, err := f.svc.Create(f.ctx, uuid.New(), input(), apiKey)
	assert.Equal(t, apperror.CodeModelProviderSecretsUnavailable, codeOf(t, err))
	assert.True(t, sealer.IsUnavailableError(err), "wraps the sealer error")

	key := "sk-new"
	_, err = f.svc.Update(f.ctx, p.ID, input(), &key)
	assert.Equal(t, apperror.CodeModelProviderSecretsUnavailable, codeOf(t, err))

	_, err = f.svc.ResolveModel(f.ctx, p.ID, "gpt-4o")
	assert.Equal(t, apperror.CodeModelProviderSecretsUnavailable, codeOf(t, err))
}

func TestService_NoSealerIsSecretsUnavailable(t *testing.T) {
	svc := modelprovider.NewService(&fakes.ModelProviderStore{}, fakes.Transactor{}, testUnexpected())
	ctx := tenant.Into(context.Background(), tenant.Context{OrgID: uuid.New()})
	_, err := svc.Create(ctx, uuid.New(), input(), apiKey)
	assert.Equal(t, apperror.CodeModelProviderSecretsUnavailable, codeOf(t, err))
}

func TestService_UpdateWithNilKeyKeepsSealedBytes(t *testing.T) {
	f := newFixture(t)
	p := f.create(t)
	before := f.store.Providers[p.ID].APIKeySealed

	in := input()
	in.Name = "renamed"
	got, err := f.svc.Update(f.ctx, p.ID, in, nil)
	require.NoError(t, err)
	assert.Equal(t, "renamed", got.Name)
	assert.Equal(t, before, f.store.Providers[p.ID].APIKeySealed)
	assert.Equal(t, []string{"update:" + p.ID.String()}, f.store.Locks, "the row is locked inside the unit of work")
	assert.Equal(t, 1, f.tx.Calls)
}

func TestFakeStore_LocksNeedAUnitOfWork(t *testing.T) {
	f := newFixture(t)
	p := f.create(t)
	_, err := f.store.LockForUpdate(f.ctx, p.ID)
	assert.Error(t, err)
	assert.Error(t, f.store.LockShared(f.ctx, []uuid.UUID{p.ID}))
	assert.Empty(t, f.store.Locks)
}

func TestService_UpdateBaseURLWithNilKeyIsSecretRequired(t *testing.T) {
	f := newFixture(t)
	p := f.create(t)
	in := input()
	in.BaseURL = "https://api.openai.com/v1"

	_, err := f.svc.Update(f.ctx, p.ID, in, nil)
	assert.Equal(t, apperror.CodeModelProviderSecretRequired, codeOf(t, err))
	assert.Equal(t, "secret_required", reasonOf(t, err))
	assert.Equal(t, "https://openrouter.ai/api/v1", f.store.Providers[p.ID].BaseURL, "nothing saved")
}

func TestService_UpdateBaseURLWithKeyReseals(t *testing.T) {
	f := newFixture(t)
	p := f.create(t)
	before := f.store.Providers[p.ID].APIKeySealed
	in := input()
	in.BaseURL = "https://api.openai.com/v1"
	key := "sk-openai"

	_, err := f.svc.Update(f.ctx, p.ID, in, &key)
	require.NoError(t, err)
	assert.NotEqual(t, before, f.store.Providers[p.ID].APIKeySealed)
	got, err := f.svc.ResolveModel(f.ctx, p.ID, "gpt-4o")
	require.NoError(t, err)
	assert.Equal(t, "sk-openai", got.APIKey)
	assert.Equal(t, "https://api.openai.com/v1", got.BaseURL)
}

func TestService_UpdateWithEmptyKeyIsSecretRequired(t *testing.T) {
	f := newFixture(t)
	p := f.create(t)
	empty := ""
	_, err := f.svc.Update(f.ctx, p.ID, input(), &empty)
	assert.Equal(t, apperror.CodeModelProviderSecretRequired, codeOf(t, err))
}

func TestService_UpdateRemovingAUsedModelIsInUse(t *testing.T) {
	f := newFixture(t)
	p := f.create(t)
	f.usage.Uses = map[uuid.UUID]modelprovider.ProviderUse{p.ID: {InUse: true, Models: map[string]bool{"llama-3": false}}}
	in := input()
	in.Models = in.Models[:1]

	_, err := f.svc.Update(f.ctx, p.ID, in, nil)
	assert.Equal(t, apperror.CodeModelProviderInUse, codeOf(t, err))
	assert.Equal(t, "model_removed", reasonOf(t, err))
	assert.Len(t, f.store.Providers[p.ID].Models, 2, "nothing saved")
}

func TestService_UpdateRemovingAUsedPriceIsInUse(t *testing.T) {
	f := newFixture(t)
	p := f.create(t)
	f.usage.Uses = map[uuid.UUID]modelprovider.ProviderUse{p.ID: {InUse: true, Models: map[string]bool{"gpt-4o": true}}}
	in := input()
	in.Models[0].Prices = nil

	_, err := f.svc.Update(f.ctx, p.ID, in, nil)
	assert.Equal(t, apperror.CodeModelProviderInUse, codeOf(t, err))
	assert.Equal(t, "price_removed", reasonOf(t, err))
	assert.NotNil(t, f.store.Providers[p.ID].Models[0].Prices, "nothing saved")
}

func TestService_UpdateDroppingAnUnpricedUseKeepsWorking(t *testing.T) {
	f := newFixture(t)
	p := f.create(t)
	f.usage.Uses = map[uuid.UUID]modelprovider.ProviderUse{p.ID: {InUse: true, Models: map[string]bool{"gpt-4o": false}}}
	in := input()
	in.Models[0].Prices = nil

	_, err := f.svc.Update(f.ctx, p.ID, in, nil)
	require.NoError(t, err)
}

func TestService_UpdateMissingIsNotFound(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Update(f.ctx, uuid.New(), input(), nil)
	assert.Equal(t, apperror.CodeModelProviderNotFound, codeOf(t, err))
}

func TestService_DeleteInUseIsInUse(t *testing.T) {
	f := newFixture(t)
	p := f.create(t)
	f.usage.Uses = map[uuid.UUID]modelprovider.ProviderUse{p.ID: {InUse: true}}

	err := f.svc.Delete(f.ctx, p.ID)
	assert.Equal(t, apperror.CodeModelProviderInUse, codeOf(t, err))
	assert.Equal(t, "delete", reasonOf(t, err))
	assert.Contains(t, f.store.Providers, p.ID)
}

func TestService_DeleteLocksAndChecksUsageInOneUnitOfWork(t *testing.T) {
	f := newFixture(t)
	p := f.create(t)
	checked := false
	f.usage.Fn = func(ctx context.Context, id uuid.UUID) (modelprovider.ProviderUse, error) {
		checked = true
		assert.Equal(t, p.ID, id)
		assert.True(t, fakes.InTx(ctx), "the usage check runs inside the unit of work")
		assert.Equal(t, []string{"update:" + p.ID.String()}, f.store.Locks, "after the row lock")
		return modelprovider.ProviderUse{}, nil
	}

	require.NoError(t, f.svc.Delete(f.ctx, p.ID))
	assert.True(t, checked)
	assert.Equal(t, 1, f.tx.Calls)
	assert.NotContains(t, f.store.Providers, p.ID)
}

func TestService_DeleteMissingIsNotFound(t *testing.T) {
	f := newFixture(t)
	err := f.svc.Delete(f.ctx, uuid.New())
	assert.Equal(t, apperror.CodeModelProviderNotFound, codeOf(t, err))
}

func TestService_UsageFailureIsUnexpected(t *testing.T) {
	f := newFixture(t)
	p := f.create(t)
	f.usage.Err = assert.AnError

	err := f.svc.Delete(f.ctx, p.ID)
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, apperror.CodeUnexpected, ae.Code())
	assert.Contains(t, f.store.Providers, p.ID)
}

func TestService_GetAndList(t *testing.T) {
	f := newFixture(t)
	projectID := uuid.New()
	p, err := f.svc.Create(f.ctx, projectID, input(), apiKey)
	require.NoError(t, err)

	got, err := f.svc.Get(f.ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ID)

	list, err := f.svc.List(f.ctx, projectID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, p.ID, list[0].ID)

	_, err = f.svc.Get(f.ctx, uuid.New())
	assert.Equal(t, apperror.CodeModelProviderNotFound, codeOf(t, err))
}

func TestService_ResolveModelOpensTheKey(t *testing.T) {
	f := newFixture(t)
	p := f.create(t)

	got, err := f.svc.ResolveModel(f.ctx, p.ID, "gpt-4o")
	require.NoError(t, err)
	assert.Equal(t, modelprovider.Resolved{
		BaseURL: "https://openrouter.ai/api/v1",
		APIKey:  apiKey,
		Model:   modelprovider.Model{ID: "gpt-4o", Vision: true, Prices: &modelprovider.Prices{InputMicros: 2_500_000, OutputMicros: 10_000_000}},
	}, got)
}

func TestService_ResolveModelUnknownModel(t *testing.T) {
	f := newFixture(t)
	p := f.create(t)

	_, err := f.svc.ResolveModel(f.ctx, p.ID, "gpt-9")
	assert.Equal(t, apperror.CodeModelProviderUnknownModel, codeOf(t, err))
}

func TestService_StoreFailureIsUnexpected(t *testing.T) {
	f := newFixture(t)
	f.store.Err = assert.AnError
	for name, call := range map[string]func() error{
		"create":  func() error { _, err := f.svc.Create(f.ctx, uuid.New(), input(), apiKey); return err },
		"get":     func() error { _, err := f.svc.Get(f.ctx, uuid.New()); return err },
		"list":    func() error { _, err := f.svc.List(f.ctx, uuid.New()); return err },
		"resolve": func() error { _, err := f.svc.ResolveModel(f.ctx, uuid.New(), "m"); return err },
		"delete":  func() error { return f.svc.Delete(f.ctx, uuid.New()) },
		"update":  func() error { _, err := f.svc.Update(f.ctx, uuid.New(), input(), nil); return err },
	} {
		t.Run(name, func(t *testing.T) {
			ae, ok := apperror.AsAppError(call())
			require.True(t, ok)
			assert.Equal(t, apperror.CodeUnexpected, ae.Code())
		})
	}
}

func TestService_ResolveNamesThePublicID(t *testing.T) {
	f := newFixture(t)
	p := f.create(t)

	ref, err := f.svc.Resolve(context.Background(), p.PublicID)
	require.NoError(t, err)
	assert.Equal(t, modelprovider.Ref{ID: p.ID, OrgID: p.OrgID, ProjectID: p.ProjectID}, ref)

	_, err = f.svc.Resolve(context.Background(), "nope")
	assert.Equal(t, apperror.CodeModelProviderNotFound, codeOf(t, err))

	f.store.Err = assert.AnError
	_, err = f.svc.Resolve(context.Background(), p.PublicID)
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperror.CodeUnexpected, ae.Code())
}

type projectOrgs map[uuid.UUID]uuid.UUID

func (m projectOrgs) OrgOf(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
	if org, ok := m[id]; ok {
		return org, nil
	}
	return uuid.Nil, &modelprovider.NotFoundError{}
}

func TestService_CreateProjectMustBelongToOrg(t *testing.T) {
	f := newFixture(t)
	foreign, own := uuid.New(), uuid.New()
	f.svc.WithProjects(projectOrgs{foreign: uuid.New(), own: f.orgID})

	_, err := f.svc.Create(f.ctx, foreign, input(), apiKey)
	assert.True(t, tenant.IsProjectOutsideOrgError(err), "got %v", err)
	assert.Equal(t, apperror.CodeProjectNotInOrg, codeOf(t, err), "the tenant error, not GEN900")
	assert.Empty(t, f.store.Providers)

	_, err = f.svc.Create(f.ctx, uuid.New(), input(), apiKey)
	assert.True(t, tenant.IsProjectOutsideOrgError(err), "an unknown project answers the same: %v", err)

	_, err = f.svc.Create(f.ctx, own, input(), apiKey)
	require.NoError(t, err)
}

func TestService_APIKeyIsTrimmed(t *testing.T) {
	f := newFixture(t)
	p, err := f.svc.Create(f.ctx, uuid.New(), input(), "  "+apiKey+"\n")
	require.NoError(t, err)
	got, err := f.svc.ResolveModel(f.ctx, p.ID, "gpt-4o")
	require.NoError(t, err)
	assert.Equal(t, apiKey, got.APIKey)

	key := "\tsk-new "
	_, err = f.svc.Update(f.ctx, p.ID, input(), &key)
	require.NoError(t, err)
	got, err = f.svc.ResolveModel(f.ctx, p.ID, "gpt-4o")
	require.NoError(t, err)
	assert.Equal(t, "sk-new", got.APIKey)
}

func TestService_BlankAPIKeyIsSecretRequired(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Create(f.ctx, uuid.New(), input(), " \t\n")
	assert.Equal(t, apperror.CodeModelProviderSecretRequired, codeOf(t, err))
	assert.Equal(t, "secret_required", reasonOf(t, err))
	assert.Empty(t, f.store.Providers)

	p := f.create(t)
	before := f.store.Providers[p.ID].APIKeySealed
	blank := "   "
	_, err = f.svc.Update(f.ctx, p.ID, input(), &blank)
	assert.Equal(t, apperror.CodeModelProviderSecretRequired, codeOf(t, err))
	assert.Equal(t, before, f.store.Providers[p.ID].APIKeySealed)
}

func TestService_APIKeyWithControlCharactersIsSecretRequired(t *testing.T) {
	f := newFixture(t)
	p := f.create(t)
	before := f.store.Providers[p.ID].APIKeySealed
	for _, v := range []string{"sk\r\nX-Evil: 1", "sk\x00x", "sk\tx"} {
		_, err := f.svc.Create(f.ctx, uuid.New(), input(), v)
		assert.Equal(t, apperror.CodeModelProviderSecretRequired, codeOf(t, err), "create %q", v)
		_, err = f.svc.Update(f.ctx, p.ID, input(), &v)
		assert.Equal(t, apperror.CodeModelProviderSecretRequired, codeOf(t, err), "update %q", v)
	}
	assert.Len(t, f.store.Providers, 1)
	assert.Equal(t, before, f.store.Providers[p.ID].APIKeySealed)
}
