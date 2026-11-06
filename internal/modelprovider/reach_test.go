package modelprovider_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	modelproviderv1 "altalune.id/oagent/gen/go/modelprovider/v1"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/modelprovider"
	"altalune.id/oagent/internal/testutil/fakes"
)

func mustProvider(t *testing.T, orgID, projectID uuid.UUID) *modelprovider.Provider {
	t.Helper()
	in := input()
	in.Name = "p-" + uuid.NewString()[:6]
	p, err := modelprovider.New(orgID, projectID, in, time.Now())
	require.NoError(t, err)
	p.APIKeySealed = []byte{1}
	return p
}

func storeWith(ps ...*modelprovider.Provider) *fakes.ModelProviderStore {
	s := &fakes.ModelProviderStore{Providers: map[uuid.UUID]*modelprovider.Provider{}}
	for _, p := range ps {
		s.Providers[p.ID] = p
	}
	return s
}

func TestModelProviderReach_IDOnlyRPCs(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgA, orgB, prjB := uuid.New(), uuid.New(), uuid.New()
	refs.Org(orgA)
	orgBPub := refs.Org(orgB)
	prjBPub := refs.Project(prjB, orgB)
	foreign := mustProvider(t, orgB, prjB)
	for name, p := range map[string]session.Principal{
		"member of A": {Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgA, Scopes: []string{"*"}},
		"key of A":    {Source: session.SourceAPIKey, ActiveOrgID: orgA, AllProjects: true},
	} {
		t.Run(name, func(t *testing.T) {
			store := storeWith(foreign)
			cp := modelprovider.NewControlPlane(modelprovider.NewService(store, fakes.Transactor{}, nil).WithSealer(testSealer(t)), refs)
			ctx := session.PrincipalInto(context.Background(), p)

			for _, id := range []string{foreign.PublicID, foreign.ID.String()} {
				_, err := cp.GetModelProvider(ctx, connect.NewRequest(&modelproviderv1.GetModelProviderRequest{Id: id}))
				assert.True(t, modelprovider.IsNotFoundError(err), "Get %s: %v", id, err)
				_, err = cp.UpdateModelProvider(ctx, connect.NewRequest(&modelproviderv1.UpdateModelProviderRequest{
					Id: id, Name: "Hacked", Kind: modelproviderv1.ModelProviderKind_MODEL_PROVIDER_KIND_OPENAI_COMPATIBLE,
					BaseUrl: foreign.BaseURL, Models: []*modelproviderv1.Model{{Id: "m"}},
				}))
				assert.True(t, modelprovider.IsNotFoundError(err), "Update %s: %v", id, err)
				_, err = cp.DeleteModelProvider(ctx, connect.NewRequest(&modelproviderv1.DeleteModelProviderRequest{Id: id}))
				assert.True(t, modelprovider.IsNotFoundError(err), "Delete %s: %v", id, err)
			}
			require.Len(t, store.Providers, 1)
			assert.Equal(t, foreign.Name, store.Providers[foreign.ID].Name, "no write")
			assert.Empty(t, store.Locks)

			for _, req := range []*modelproviderv1.CreateModelProviderRequest{
				createRequest(orgBPub, prjBPub),
				createRequest(orgB.String(), prjB.String()),
			} {
				_, err := cp.CreateModelProvider(ctx, connect.NewRequest(req))
				assert.True(t, modelprovider.IsNotFoundError(err), "Create in %s: %v", req.GetProjectId(), err)
				_, err = cp.ListModelProviders(ctx, connect.NewRequest(&modelproviderv1.ListModelProvidersRequest{ProjectId: req.GetProjectId()}))
				assert.True(t, modelprovider.IsNotFoundError(err), "List in %s: %v", req.GetProjectId(), err)
			}
			assert.Len(t, store.Providers, 1)
		})
	}
}

func TestModelProviderReach_ListFiltersToReach(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgA, prj := uuid.New(), uuid.New()
	refs.Org(orgA)
	prjPub := refs.Project(prj, orgA)
	mine, other := mustProvider(t, orgA, prj), mustProvider(t, orgA, prj)
	foreign := mustProvider(t, uuid.New(), prj)
	cp := modelprovider.NewControlPlane(modelprovider.NewService(storeWith(mine, other, foreign), fakes.Transactor{}, nil), refs)
	list := func(p session.Principal) ([]string, error) {
		ctx := context.Background()
		if p.Source != "" {
			ctx = session.PrincipalInto(ctx, p)
		}
		resp, err := cp.ListModelProviders(ctx, connect.NewRequest(&modelproviderv1.ListModelProvidersRequest{ProjectId: prjPub}))
		if err != nil {
			return nil, err
		}
		var out []string
		for _, mp := range resp.Msg.GetModelProviders() {
			out = append(out, mp.GetId())
		}
		return out, nil
	}
	_, err := list(session.Principal{})
	assert.True(t, modelprovider.IsNotFoundError(err), "no principal reaches nothing: %v", err)
	got, err := list(session.System())
	require.NoError(t, err)
	assert.Len(t, got, 3, "system sees all")
	got, err = list(session.Principal{Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgA})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{mine.PublicID, other.PublicID}, got)
	got, err = list(session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, ProjectIDs: []uuid.UUID{prj}, ResourceIDs: []uuid.UUID{mine.ID}})
	require.NoError(t, err)
	assert.Equal(t, []string{mine.PublicID}, got)
}

func TestModelProviderReach_OwnProviderWorks(t *testing.T) {
	refs := fakes.NewPublicIDs()
	orgA, prj := uuid.New(), uuid.New()
	refs.Org(orgA)
	refs.Project(prj, orgA)
	p := mustProvider(t, orgA, prj)
	store := storeWith(p)
	cp := modelprovider.NewControlPlane(modelprovider.NewService(store, fakes.Transactor{}, nil), refs)
	ctx := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgA, ProjectIDs: []uuid.UUID{prj}})
	resp, err := cp.GetModelProvider(ctx, connect.NewRequest(&modelproviderv1.GetModelProviderRequest{Id: p.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, p.PublicID, resp.Msg.GetModelProvider().GetId())
	_, err = cp.DeleteModelProvider(ctx, connect.NewRequest(&modelproviderv1.DeleteModelProviderRequest{Id: p.PublicID}))
	require.NoError(t, err)
	assert.Empty(t, store.Providers)
}
