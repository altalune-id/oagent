package modelprovider_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"altalune.id/oagent/apperror"
	modelproviderv1 "altalune.id/oagent/gen/go/modelprovider/v1"
	modelproviderv1connect "altalune.id/oagent/gen/go/modelprovider/v1/modelproviderv1connect"
	"altalune.id/oagent/gen/go/modelprovider/v1/modelproviderv1mcp"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/testutil/fakes"
)

type cpFixture struct {
	store      *fakes.ModelProviderStore
	refs       *fakes.PublicIDs
	cp         *modelprovider.ModelProviderService
	client     modelproviderv1connect.ModelProviderServiceClient
	orgID      uuid.UUID
	projectID  uuid.UUID
	orgPub     string
	projectPub string
}

func newCPFixture(t *testing.T) *cpFixture {
	t.Helper()
	f := &cpFixture{store: &fakes.ModelProviderStore{}, refs: fakes.NewPublicIDs(), orgID: uuid.New(), projectID: uuid.New()}
	f.orgPub = f.refs.Org(f.orgID)
	f.projectPub = f.refs.Project(f.projectID, f.orgID)
	svc := modelprovider.NewService(f.store, fakes.Transactor{}, testUnexpected()).WithSealer(testSealer(t))
	f.cp = modelprovider.NewControlPlane(svc, f.refs)
	_, handler := modelproviderv1connect.NewModelProviderServiceHandler(f.cp,
		connect.WithInterceptors(fakes.OperatorInterceptor()))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	f.client = modelproviderv1connect.NewModelProviderServiceClient(http.DefaultClient, ts.URL)
	return f
}

func createRequest(orgPub, projectPub string) *modelproviderv1.CreateModelProviderRequest {
	return &modelproviderv1.CreateModelProviderRequest{
		OrgId:     orgPub,
		ProjectId: projectPub,
		Name:      "openrouter",
		Kind:      modelproviderv1.ModelProviderKind_MODEL_PROVIDER_KIND_OPENAI_COMPATIBLE,
		BaseUrl:   "https://openrouter.ai/api/v1",
		ApiKey:    apiKey,
		Models: []*modelproviderv1.Model{
			{Id: "gpt-4o", Vision: true, Prices: &modelproviderv1.ModelPrices{InputMicros: 2_500_000, OutputMicros: 10_000_000, CacheReadMicros: 1, CacheWriteMicros: 2}},
			{Id: "llama-3"},
			{Id: "text-embedding-3-small", Embedding: true, Prices: &modelproviderv1.ModelPrices{InputMicros: 20_000}},
		},
	}
}

func TestModelProviderService_CreateReturnsNoKey(t *testing.T) {
	f := newCPFixture(t)
	req := createRequest(f.orgPub, f.projectPub)
	req.Models = append(req.Models, &modelproviderv1.Model{Id: "whisper-1", Transcription: true, Prices: &modelproviderv1.ModelPrices{AudioMinuteMicros: 6_000}})
	resp, err := f.client.CreateModelProvider(context.Background(), connect.NewRequest(req))
	require.NoError(t, err)
	mp := resp.Msg.GetModelProvider()
	assert.True(t, mp.GetApiKeySet())
	assert.Equal(t, f.orgPub, mp.GetOrgId())
	assert.Equal(t, f.projectPub, mp.GetProjectId())
	assert.Equal(t, "openrouter", mp.GetName())
	assert.Equal(t, modelproviderv1.ModelProviderKind_MODEL_PROVIDER_KIND_OPENAI_COMPATIBLE, mp.GetKind())
	assert.Equal(t, "https://openrouter.ai/api/v1", mp.GetBaseUrl())
	require.Len(t, mp.GetModels(), 4)
	assert.True(t, mp.GetModels()[0].GetVision())
	assert.False(t, mp.GetModels()[0].GetEmbedding())
	assert.True(t, mp.GetModels()[2].GetEmbedding(), "the embedding flag round-trips")
	assert.False(t, mp.GetModels()[2].GetTranscription())
	assert.True(t, mp.GetModels()[3].GetTranscription(), "the transcription flag round-trips")
	assert.Equal(t, int64(6_000), mp.GetModels()[3].GetPrices().GetAudioMinuteMicros(), "the audio minute price round-trips")
	assert.Equal(t, int64(2_500_000), mp.GetModels()[0].GetPrices().GetInputMicros())
	assert.Nil(t, mp.GetModels()[1].Prices, "an unpriced model has no prices")
	assert.NotEmpty(t, mp.GetCreatedAt())

	raw, err := protojson.Marshal(resp.Msg)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), apiKey)
	require.Len(t, f.store.Providers, 1)
	for id, p := range f.store.Providers {
		assert.Equal(t, mp.GetId(), p.PublicID)
		assert.NotEqual(t, id.String(), mp.GetId())
		assert.Equal(t, f.orgID, p.OrgID)
		assert.Equal(t, f.projectID, p.ProjectID)
	}
}

func TestModelProviderService_GetUpdateListDelete(t *testing.T) {
	f := newCPFixture(t)
	ctx := context.Background()
	created, err := f.client.CreateModelProvider(ctx, connect.NewRequest(createRequest(f.orgPub, f.projectPub)))
	require.NoError(t, err)
	id := created.Msg.GetModelProvider().GetId()

	got, err := f.client.GetModelProvider(ctx, connect.NewRequest(&modelproviderv1.GetModelProviderRequest{Id: id}))
	require.NoError(t, err)
	assert.Equal(t, id, got.Msg.GetModelProvider().GetId())
	assert.Equal(t, f.orgPub, got.Msg.GetModelProvider().GetOrgId())
	assert.Equal(t, f.projectPub, got.Msg.GetModelProvider().GetProjectId())

	upd := &modelproviderv1.UpdateModelProviderRequest{
		Id: id, Name: "renamed", Kind: modelproviderv1.ModelProviderKind_MODEL_PROVIDER_KIND_OPENAI_COMPATIBLE,
		BaseUrl: "https://openrouter.ai/api/v1", Models: []*modelproviderv1.Model{{Id: "only"}},
	}
	updated, err := f.client.UpdateModelProvider(ctx, connect.NewRequest(upd))
	require.NoError(t, err)
	assert.Equal(t, "renamed", updated.Msg.GetModelProvider().GetName())
	assert.True(t, updated.Msg.GetModelProvider().GetApiKeySet())

	upd.BaseUrl = "https://api.openai.com/v1"
	_, err = f.cp.UpdateModelProvider(fakes.OperatorContext(ctx), connect.NewRequest(upd))
	assert.Equal(t, apperror.CodeModelProviderSecretRequired, codeOf(t, err), "a new base URL needs a key")
	key := "sk-openai"
	upd.ApiKey = &key
	_, err = f.client.UpdateModelProvider(ctx, connect.NewRequest(upd))
	require.NoError(t, err)

	list, err := f.client.ListModelProviders(ctx, connect.NewRequest(&modelproviderv1.ListModelProvidersRequest{ProjectId: f.projectPub}))
	require.NoError(t, err)
	require.Len(t, list.Msg.GetModelProviders(), 1)
	assert.Equal(t, f.orgPub, list.Msg.GetModelProviders()[0].GetOrgId())
	assert.Equal(t, "https://api.openai.com/v1", list.Msg.GetModelProviders()[0].GetBaseUrl())

	_, err = f.client.DeleteModelProvider(ctx, connect.NewRequest(&modelproviderv1.DeleteModelProviderRequest{Id: id}))
	require.NoError(t, err)
	assert.Empty(t, f.store.Providers)
}

func TestModelProviderService_EveryProtoKindRoundTrips(t *testing.T) {
	assert.Len(t, modelprovider.Kinds(), len(modelproviderv1.ModelProviderKind_name)-1, "every domain kind has a proto value")
	for value, name := range modelproviderv1.ModelProviderKind_name {
		kind := modelproviderv1.ModelProviderKind(value)
		if kind == modelproviderv1.ModelProviderKind_MODEL_PROVIDER_KIND_UNSPECIFIED {
			continue
		}
		t.Run(name, func(t *testing.T) {
			f := newCPFixture(t)
			req := createRequest(f.orgPub, f.projectPub)
			req.Kind = kind
			created, err := f.client.CreateModelProvider(context.Background(), connect.NewRequest(req))
			require.NoError(t, err)
			assert.Equal(t, kind, created.Msg.GetModelProvider().GetKind())
			for _, p := range f.store.Providers {
				assert.Contains(t, modelprovider.Kinds(), p.Kind)
			}
		})
	}
}

func TestModelProviderService_UpdateWithAnotherKindIsRefused(t *testing.T) {
	f := newCPFixture(t)
	ctx := context.Background()
	created, err := f.client.CreateModelProvider(ctx, connect.NewRequest(createRequest(f.orgPub, f.projectPub)))
	require.NoError(t, err)

	_, err = f.cp.UpdateModelProvider(fakes.OperatorContext(ctx), connect.NewRequest(&modelproviderv1.UpdateModelProviderRequest{
		Id: created.Msg.GetModelProvider().GetId(), Name: "openrouter",
		Kind:    modelproviderv1.ModelProviderKind_MODEL_PROVIDER_KIND_ANTHROPIC,
		BaseUrl: "https://openrouter.ai/api/v1", Models: []*modelproviderv1.Model{{Id: "gpt-4o"}},
	}))
	assert.Equal(t, apperror.CodeValidation, codeOf(t, err))
	assert.Equal(t, string(apperror.FieldNotAllowed), reasonOf(t, err))
	assert.True(t, modelprovider.IsKindChangedError(err), "the refusal names the kind field, got %v", err)
}

func TestModelProviderService_GetWithAnotherIDIsNotFound(t *testing.T) {
	f := newCPFixture(t)
	ctx := context.Background()
	_, err := f.client.CreateModelProvider(ctx, connect.NewRequest(createRequest(f.orgPub, f.projectPub)))
	require.NoError(t, err)
	var rowID uuid.UUID
	for id := range f.store.Providers {
		rowID = id
	}

	for _, id := range []string{f.projectPub, rowID.String()} {
		_, err := f.cp.GetModelProvider(fakes.OperatorContext(ctx), connect.NewRequest(&modelproviderv1.GetModelProviderRequest{Id: id}))
		assert.Equal(t, apperror.CodeModelProviderNotFound, codeOf(t, err), "id %s", id)
	}
}

// SECURITY: a provider's API key never travels through an MCP tool, so the tools that take one are not generated.
func TestModelProviderMCPTools_NeverTakeAKey(t *testing.T) {
	names := modelproviderv1mcp.ModelProviderServiceToolNames()
	assert.ElementsMatch(t, []string{"model_provider_get", "model_provider_list", "model_provider_delete"}, names)
	assert.NotContains(t, names, "model_provider_create")
	assert.NotContains(t, names, "model_provider_update")
}

func TestModelProviderService_CreateWithAProjectOfAnotherOrgIsNotFound(t *testing.T) {
	f := newCPFixture(t)
	otherOrg := f.refs.Org(uuid.New())
	for _, org := range []string{otherOrg, "unknown-org-id-000000", f.orgID.String()} {
		_, err := f.cp.CreateModelProvider(fakes.OperatorContext(context.Background()), connect.NewRequest(createRequest(org, f.projectPub)))
		assert.Equal(t, apperror.CodeModelProviderNotFound, codeOf(t, err), "org %s", org)
	}
	assert.Empty(t, f.store.Providers)
}
