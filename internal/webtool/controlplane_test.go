package webtool_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"altalune.id/oagent/apperror"
	webtoolv1 "altalune.id/oagent/gen/go/webtool/v1"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/internal/webtool"
	"altalune.id/oagent/sealer"
)

const (
	searchKey   = "sk-search-SECRET-1234"
	secretValue = "Bearer SECRET-VALUE-5678"
)

var (
	brave  = webtoolv1.WebSearchProviderKind_WEB_SEARCH_PROVIDER_KIND_BRAVE
	exa    = webtoolv1.WebSearchProviderKind_WEB_SEARCH_PROVIDER_KIND_EXA
	tavily = webtoolv1.WebSearchProviderKind_WEB_SEARCH_PROVIDER_KIND_TAVILY
)

func testSealer(t *testing.T) sealer.Sealer {
	t.Helper()
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	sl, err := sealer.New(key)
	require.NoError(t, err)
	return sl
}

type fixture struct {
	store      *fakes.WebToolStore
	refs       *fakes.PublicIDs
	svc        *webtool.Service
	cp         *webtool.WebToolService
	org, prj   uuid.UUID
	prjPub     string
	ctx        context.Context
	unexpected []error
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{store: &fakes.WebToolStore{}, refs: fakes.NewPublicIDs(), org: uuid.New(), prj: uuid.New()}
	f.prjPub = f.refs.Project(f.prj, f.org)
	f.svc = webtool.NewService(f.store, func(_ context.Context, _ string, err error, _ ...any) *apperror.AppError {
		f.unexpected = append(f.unexpected, err)
		return apperror.New(apperror.CodeUnexpected, "unexpected").WithCause(err)
	}).WithSealer(testSealer(t))
	f.cp = webtool.NewControlPlane(f.svc, f.refs)
	f.ctx = session.PrincipalInto(context.Background(), session.Principal{
		Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: f.org, Scopes: []string{"*"},
	})
	return f
}

func (f *fixture) setProvider(kind webtoolv1.WebSearchProviderKind, key *string, price int64) (*webtoolv1.WebSearchProvider, error) {
	res, err := f.cp.SetWebSearchProvider(f.ctx, connect.NewRequest(&webtoolv1.SetWebSearchProviderRequest{
		ProjectId: f.prjPub, Kind: kind, ApiKey: key, PricePerCallMicros: new(price),
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetProvider(), nil
}

func (f *fixture) setSecret(host, header, value string) (*webtoolv1.HTTPSecret, error) {
	res, err := f.cp.SetHTTPSecret(f.ctx, connect.NewRequest(&webtoolv1.SetHTTPSecretRequest{
		ProjectId: f.prjPub, Host: host, Header: header, Value: value,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetSecret(), nil
}

func (f *fixture) scoped() context.Context {
	return tenant.WithOrg(context.Background(), f.org)
}

func TestGetProvider_NoneListsAvailableKinds(t *testing.T) {
	f := newFixture(t)
	res, err := f.cp.GetWebSearchProvider(f.ctx, connect.NewRequest(&webtoolv1.GetWebSearchProviderRequest{ProjectId: f.prjPub}))
	require.NoError(t, err)
	assert.Nil(t, res.Msg.Provider)
	assert.Equal(t, []webtoolv1.WebSearchProviderKind{brave, exa}, res.Msg.GetAvailableKinds())
}

func TestSetProvider_RoundTripAndPriceZero(t *testing.T) {
	f := newFixture(t)
	p, err := f.setProvider(brave, new(searchKey), 0)
	require.NoError(t, err)
	assert.Equal(t, brave, p.GetKind())
	assert.True(t, p.GetKeySet())
	assert.Zero(t, p.GetPricePerCallMicros())

	p, err = f.setProvider(brave, nil, 5000)
	require.NoError(t, err, "a price change keeps the stored key")
	assert.Equal(t, int64(5000), p.GetPricePerCallMicros())

	open, err := f.svc.OpenSearchKey(f.scoped(), f.prj)
	require.NoError(t, err)
	assert.Equal(t, webtool.OpenProvider{Kind: webtool.KindBrave, APIKey: searchKey, PricePerCallMicros: 5000}, open)
}

func TestSetProvider_TavilyRefused(t *testing.T) {
	f := newFixture(t)
	_, err := f.setProvider(tavily, new(searchKey), 0)
	assert.True(t, webtool.IsProviderUnavailableError(err), "%v", err)
	assert.Empty(t, f.store.Providers)
}

func TestSetProvider_KeyRequiredOnCreateAndKindChange(t *testing.T) {
	f := newFixture(t)
	_, err := f.setProvider(brave, nil, 0)
	assert.True(t, webtool.IsSecretRequiredError(err), "create without a key: %v", err)
	_, err = f.setProvider(brave, new("  \n"), 0)
	assert.True(t, webtool.IsSecretRequiredError(err), "a blank or control-character key: %v", err)
	_, err = f.setProvider(brave, new("sk-a\r\nX-Injected: 1"), 0)
	assert.True(t, webtool.IsSecretRequiredError(err), "a key with an inner control character: %v", err)

	_, err = f.setProvider(brave, new(searchKey), 0)
	require.NoError(t, err)
	_, err = f.setProvider(exa, nil, 0)
	assert.True(t, webtool.IsSecretRequiredError(err), "kind change without a key: %v", err)
	_, err = f.setProvider(exa, new("exa-key"), 0)
	require.NoError(t, err)
	open, err := f.svc.OpenSearchKey(f.scoped(), f.prj)
	require.NoError(t, err)
	assert.Equal(t, "exa-key", open.APIKey)
}

func TestProvider_StoredKindNoLongerAvailable(t *testing.T) {
	f := newFixture(t)
	now := time.Now().UTC()
	f.store.Providers = map[uuid.UUID]*webtool.Provider{f.prj: {OrgID: f.org, ProjectID: f.prj, Kind: webtool.KindTavily, APIKeySealed: []byte{1}, CreatedAt: now, UpdatedAt: now}}
	res, err := f.cp.GetWebSearchProvider(f.ctx, connect.NewRequest(&webtoolv1.GetWebSearchProviderRequest{ProjectId: f.prjPub}))
	require.NoError(t, err)
	assert.Equal(t, tavily, res.Msg.GetProvider().GetKind(), "the console sees the stored kind")
	_, err = f.svc.OpenSearchKey(f.scoped(), f.prj)
	assert.True(t, webtool.IsNotFoundError(err), "a run treats it as no provider: %v", err)
}

func TestDeleteProvider_Idempotent(t *testing.T) {
	f := newFixture(t)
	_, err := f.setProvider(brave, new(searchKey), 0)
	require.NoError(t, err)
	for range 2 {
		_, err = f.cp.DeleteWebSearchProvider(f.ctx, connect.NewRequest(&webtoolv1.DeleteWebSearchProviderRequest{ProjectId: f.prjPub}))
		require.NoError(t, err)
	}
	_, err = f.svc.OpenSearchKey(f.scoped(), f.prj)
	assert.True(t, webtool.IsNotFoundError(err))
}

func TestSetSecret_ReservedHeaderRefused(t *testing.T) {
	f := newFixture(t)
	for _, h := range []string{"Host", "Cookie", "X-Forwarded-For", "X-HTTP-Method-Override", "x-method-override"} {
		_, err := f.setSecret("api.example.com", h, secretValue)
		ae, ok := apperror.AsAppError(err)
		require.True(t, ok, h)
		assert.Equal(t, "header", ae.Meta()["field"], h)
	}
	_, err := f.setSecret("api.example.com", "Authorization", secretValue)
	require.NoError(t, err, "a secret may own Authorization")
}

func TestSetSecret_InvalidHostRefused(t *testing.T) {
	f := newFixture(t)
	for _, h := range []string{"1.2.3.4", "localhost", "intranet", "0x7f.1"} {
		_, err := f.setSecret(h, "x-api-key", secretValue)
		ae, ok := apperror.AsAppError(err)
		require.True(t, ok, h)
		assert.Equal(t, "host", ae.Meta()["field"], h)
	}
	assert.Empty(t, f.store.HTTPSecrets)
}

func TestSetSecret_ValueWithControlCharactersRefused(t *testing.T) {
	f := newFixture(t)
	_, err := f.setSecret("api.example.com", "x-api-key", "v\r\nX-Injected: 1")
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, "value", ae.Meta()["field"])
}

func TestSetSecret_LimitReached(t *testing.T) {
	f := newFixture(t)
	now := time.Now().UTC()
	for i := range webtool.MaxHTTPSecrets {
		f.store.HTTPSecrets = append(f.store.HTTPSecrets, &webtool.HTTPSecret{
			ID: uuid.New(), PublicID: uuid.NewString(), OrgID: f.org, ProjectID: f.prj,
			Host: "h" + string(rune('a'+i%26)) + ".example.com", Header: "x-" + uuid.NewString()[:8], CreatedAt: now, UpdatedAt: now,
		})
	}
	_, err := f.setSecret("api.example.com", "x-api-key", secretValue)
	assert.True(t, webtool.IsSecretLimitError(err), "%v", err)

	existing := f.store.HTTPSecrets[0]
	_, err = f.setSecret(existing.Host, existing.Header, "new value")
	assert.NoError(t, err, "replacing a stored secret at the limit is allowed")
}

func TestSetSecret_ReplacesValueKeepsID(t *testing.T) {
	f := newFixture(t)
	first, err := f.setSecret("api.example.com", "X-Api-Key", "one")
	require.NoError(t, err)
	assert.Equal(t, "x-api-key", first.GetHeader())
	second, err := f.setSecret("api.example.com", "x-api-key", "two")
	require.NoError(t, err)
	assert.Equal(t, first.GetId(), second.GetId())

	open, unsealable, err := f.svc.OpenSecrets(f.scoped(), f.prj, []string{"api.example.com"})
	require.NoError(t, err)
	assert.Zero(t, unsealable)
	assert.Equal(t, []webtool.OpenSecret{{Host: "api.example.com", Header: "x-api-key", Value: "two"}}, open)
}

func TestOpenSecrets_OnlyListedHostsAndUnsealableCounted(t *testing.T) {
	f := newFixture(t)
	_, err := f.setSecret("api.example.com", "x-api-key", "a")
	require.NoError(t, err)
	_, err = f.setSecret("other.example.com", "x-api-key", "b")
	require.NoError(t, err)
	f.store.HTTPSecrets[1].ValueSealed = []byte("garbage")

	open, unsealable, err := f.svc.OpenSecrets(f.scoped(), f.prj, []string{"api.example.com", "other.example.com"})
	require.NoError(t, err)
	assert.Equal(t, 1, unsealable)
	assert.Equal(t, []webtool.OpenSecret{{Host: "api.example.com", Header: "x-api-key", Value: "a"}}, open)

	none, _, err := f.svc.OpenSecrets(f.scoped(), f.prj, nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestListAndDeleteSecrets(t *testing.T) {
	f := newFixture(t)
	s, err := f.setSecret("api.example.com", "x-api-key", secretValue)
	require.NoError(t, err)
	list, err := f.cp.ListHTTPSecrets(f.ctx, connect.NewRequest(&webtoolv1.ListHTTPSecretsRequest{ProjectId: f.prjPub}))
	require.NoError(t, err)
	require.Len(t, list.Msg.GetSecrets(), 1)
	assert.Equal(t, s.GetId(), list.Msg.GetSecrets()[0].GetId())

	_, err = f.cp.DeleteHTTPSecret(f.ctx, connect.NewRequest(&webtoolv1.DeleteHTTPSecretRequest{Id: s.GetId()}))
	require.NoError(t, err)
	_, err = f.cp.DeleteHTTPSecret(f.ctx, connect.NewRequest(&webtoolv1.DeleteHTTPSecretRequest{Id: s.GetId()}))
	assert.True(t, webtool.IsNotFoundError(err))
}

func TestNoKeyInResponseOrError(t *testing.T) {
	f := newFixture(t)
	var out []proto.Message
	p, err := f.setProvider(brave, new(searchKey), 1)
	require.NoError(t, err)
	out = append(out, p)
	s, err := f.setSecret("api.example.com", "x-api-key", secretValue)
	require.NoError(t, err)
	out = append(out, s)
	get, err := f.cp.GetWebSearchProvider(f.ctx, connect.NewRequest(&webtoolv1.GetWebSearchProviderRequest{ProjectId: f.prjPub}))
	require.NoError(t, err)
	out = append(out, get.Msg)
	list, err := f.cp.ListHTTPSecrets(f.ctx, connect.NewRequest(&webtoolv1.ListHTTPSecretsRequest{ProjectId: f.prjPub}))
	require.NoError(t, err)
	out = append(out, list.Msg)
	for _, m := range out {
		raw, err := protojson.Marshal(m)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "SECRET")
	}

	f.store.Err = assert.AnError
	for _, call := range []func() error{
		func() error { _, err := f.setProvider(exa, new(searchKey), 1); return err },
		func() error { _, err := f.setSecret("api.example.com", "x-api-key", secretValue); return err },
	} {
		err := call()
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "SECRET")
		for _, u := range f.unexpected {
			assert.NotContains(t, u.Error(), "SECRET", "a reported error names no key or value")
		}
	}
}

func TestWebToolReach_AnotherOrgIsNotFound(t *testing.T) {
	f := newFixture(t)
	orgB, prjB := uuid.New(), uuid.New()
	prjBPub := f.refs.Project(prjB, orgB)
	now := time.Now().UTC()
	foreign := &webtool.HTTPSecret{ID: uuid.New(), PublicID: "foreignsecretpublicid", OrgID: orgB, ProjectID: prjB, Host: "api.example.com", Header: "x", ValueSealed: []byte{1}, CreatedAt: now, UpdatedAt: now}
	f.store.HTTPSecrets = []*webtool.HTTPSecret{foreign}

	_, err := f.cp.GetWebSearchProvider(f.ctx, connect.NewRequest(&webtoolv1.GetWebSearchProviderRequest{ProjectId: prjBPub}))
	assert.True(t, webtool.IsNotFoundError(err))
	_, err = f.cp.SetWebSearchProvider(f.ctx, connect.NewRequest(&webtoolv1.SetWebSearchProviderRequest{ProjectId: prjBPub, Kind: brave, ApiKey: new("k"), PricePerCallMicros: proto.Int64(0)}))
	assert.True(t, webtool.IsNotFoundError(err))
	_, err = f.cp.DeleteWebSearchProvider(f.ctx, connect.NewRequest(&webtoolv1.DeleteWebSearchProviderRequest{ProjectId: prjBPub}))
	assert.True(t, webtool.IsNotFoundError(err))
	_, err = f.cp.ListHTTPSecrets(f.ctx, connect.NewRequest(&webtoolv1.ListHTTPSecretsRequest{ProjectId: prjBPub}))
	assert.True(t, webtool.IsNotFoundError(err))
	_, err = f.cp.SetHTTPSecret(f.ctx, connect.NewRequest(&webtoolv1.SetHTTPSecretRequest{ProjectId: prjBPub, Host: "api.example.com", Header: "x", Value: "v"}))
	assert.True(t, webtool.IsNotFoundError(err))
	for _, id := range []string{foreign.PublicID, foreign.ID.String(), "unknownunknownunknown"} {
		_, err = f.cp.DeleteHTTPSecret(f.ctx, connect.NewRequest(&webtoolv1.DeleteHTTPSecretRequest{Id: id}))
		assert.True(t, webtool.IsNotFoundError(err), id)
	}
	assert.Empty(t, f.store.Providers)
	assert.Len(t, f.store.HTTPSecrets, 1)
	assert.Equal(t, []byte{1}, f.store.HTTPSecrets[0].ValueSealed)
}

func TestWebToolReach_LimitedKeyCannotSet(t *testing.T) {
	f := newFixture(t)
	f.ctx = session.PrincipalInto(context.Background(), session.Principal{
		Source: session.SourceAPIKey, ActiveOrgID: f.org, ProjectIDs: []uuid.UUID{f.prj}, ResourceIDs: []uuid.UUID{uuid.New()},
	})
	_, err := f.cp.GetWebSearchProvider(f.ctx, connect.NewRequest(&webtoolv1.GetWebSearchProviderRequest{ProjectId: f.prjPub}))
	require.NoError(t, err, "a limited key may read")
	_, err = f.cp.ListHTTPSecrets(f.ctx, connect.NewRequest(&webtoolv1.ListHTTPSecretsRequest{ProjectId: f.prjPub}))
	require.NoError(t, err)

	_, err = f.setProvider(brave, new(searchKey), 0)
	assert.True(t, webtool.IsNotFoundError(err), "set provider: %v", err)
	_, err = f.cp.DeleteWebSearchProvider(f.ctx, connect.NewRequest(&webtoolv1.DeleteWebSearchProviderRequest{ProjectId: f.prjPub}))
	assert.True(t, webtool.IsNotFoundError(err), "delete provider: %v", err)
	_, err = f.setSecret("api.example.com", "x-api-key", secretValue)
	assert.True(t, webtool.IsNotFoundError(err), "set secret: %v", err)
	assert.Empty(t, f.store.Providers)
	assert.Empty(t, f.store.HTTPSecrets)
}

func TestService_WithoutASealerIsUnavailable(t *testing.T) {
	f := newFixture(t)
	svc := webtool.NewService(f.store, nil)
	cp := webtool.NewControlPlane(svc, f.refs)
	_, err := cp.SetWebSearchProvider(f.ctx, connect.NewRequest(&webtoolv1.SetWebSearchProviderRequest{ProjectId: f.prjPub, Kind: brave, ApiKey: new("k"), PricePerCallMicros: proto.Int64(0)}))
	assert.True(t, webtool.IsSecretsUnavailableError(err), "%v", err)
	_, err = cp.SetHTTPSecret(f.ctx, connect.NewRequest(&webtoolv1.SetHTTPSecretRequest{ProjectId: f.prjPub, Host: "api.example.com", Header: "x", Value: "v"}))
	assert.True(t, webtool.IsSecretsUnavailableError(err), "%v", err)
}

func TestService_StoreErrorsAreReported(t *testing.T) {
	f := newFixture(t)
	f.store.Err = assert.AnError
	_, err := f.cp.GetWebSearchProvider(f.ctx, connect.NewRequest(&webtoolv1.GetWebSearchProviderRequest{ProjectId: f.prjPub}))
	require.ErrorIs(t, err, assert.AnError)
	_, err = f.cp.ListHTTPSecrets(f.ctx, connect.NewRequest(&webtoolv1.ListHTTPSecretsRequest{ProjectId: f.prjPub}))
	require.ErrorIs(t, err, assert.AnError)
	_, err = f.cp.DeleteWebSearchProvider(f.ctx, connect.NewRequest(&webtoolv1.DeleteWebSearchProviderRequest{ProjectId: f.prjPub}))
	require.ErrorIs(t, err, assert.AnError)
	_, err = f.cp.DeleteHTTPSecret(f.ctx, connect.NewRequest(&webtoolv1.DeleteHTTPSecretRequest{Id: "x"}))
	require.ErrorIs(t, err, assert.AnError)
	_, _, err = f.svc.OpenSecrets(f.scoped(), f.prj, []string{"a.example"})
	require.ErrorIs(t, err, assert.AnError)
	_, err = f.svc.OpenSearchKey(f.scoped(), f.prj)
	require.ErrorIs(t, err, assert.AnError)
	assert.NotEmpty(t, f.unexpected)
}

func TestWebToolReach_LimitedKeyListsOnlyItsSecrets(t *testing.T) {
	f := newFixture(t)
	mine, err := f.setSecret("api.example.com", "x-api-key", secretValue)
	require.NoError(t, err)
	_, err = f.setSecret("other.example.com", "x-other", secretValue)
	require.NoError(t, err)
	var mineID uuid.UUID
	for _, s := range f.store.HTTPSecrets {
		if s.PublicID == mine.GetId() {
			mineID = s.ID
		}
	}
	f.ctx = session.PrincipalInto(context.Background(), session.Principal{
		Source: session.SourceAPIKey, ActiveOrgID: f.org, ProjectIDs: []uuid.UUID{f.prj}, ResourceIDs: []uuid.UUID{mineID},
	})
	list, err := f.cp.ListHTTPSecrets(f.ctx, connect.NewRequest(&webtoolv1.ListHTTPSecretsRequest{ProjectId: f.prjPub}))
	require.NoError(t, err)
	require.Len(t, list.Msg.GetSecrets(), 1, "a resource-limited key sees only the secrets it holds")
	assert.Equal(t, mine.GetId(), list.Msg.GetSecrets()[0].GetId())
}
