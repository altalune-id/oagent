package org_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgv1 "altalune.id/oagent/gen/go/org/v1"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/testutil/fakes"
)

func mustOrg(t *testing.T, slug string) *org.Org {
	t.Helper()
	o, err := org.NewOrg(slug, "Org", uuid.Nil)
	require.NoError(t, err)
	return o
}

func TestOrgReach_IDOnlyRPCs(t *testing.T) {
	orgA, orgB := mustOrg(t, "org-a"), mustOrg(t, "org-b")
	cp := org.NewControlPlane(org.NewService(&fakes.OrgStore{Org: orgB}, nil))
	for name, p := range map[string]session.Principal{
		"member of A": {Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgA.ID, Scopes: []string{"*"}},
		"key of A":    {Source: session.SourceAPIKey, ActiveOrgID: orgA.ID, AllProjects: true, Scopes: []string{"org:read", "org:write"}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := session.PrincipalInto(context.Background(), p)
			_, err := cp.GetOrg(ctx, connect.NewRequest(&orgv1.GetOrgRequest{Id: orgB.ID.String()}))
			assert.True(t, org.IsNotFoundError(err), "GetOrg: %v", err)
			_, err = cp.UpdateOrg(ctx, connect.NewRequest(&orgv1.UpdateOrgRequest{Id: orgB.ID.String(), Name: "Hacked"}))
			assert.True(t, org.IsNotFoundError(err), "UpdateOrg: %v", err)
		})
	}
}

func TestOrgReach_OwnOrgAndTrustedCallers(t *testing.T) {
	o := mustOrg(t, "org-a")
	cp := org.NewControlPlane(org.NewService(&fakes.OrgStore{Org: o}, nil))
	for name, ctx := range map[string]context.Context{
		"system":   fakes.OperatorContext(context.Background()),
		"member":   session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: o.ID}),
		"operator": session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceBasic, Operator: true}),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := cp.GetOrg(ctx, connect.NewRequest(&orgv1.GetOrgRequest{Id: o.ID.String()}))
			require.NoError(t, err)
		})
	}

	_, err := cp.GetOrg(context.Background(), connect.NewRequest(&orgv1.GetOrgRequest{Id: o.ID.String()}))
	assert.True(t, org.IsNotFoundError(err), "no principal reaches nothing: %v", err)
}

func TestOrgReach_ListOrgs(t *testing.T) {
	orgA, orgB := mustOrg(t, "org-a"), mustOrg(t, "org-b")
	cp := org.NewControlPlane(org.NewService(&fakes.OrgStore{Orgs: []*org.Org{orgA, orgB}}, nil))
	ids := func(ctx context.Context) []string {
		resp, err := cp.ListOrgs(ctx, connect.NewRequest(&orgv1.ListOrgsRequest{}))
		require.NoError(t, err)
		var out []string
		for _, o := range resp.Msg.GetOrgs() {
			out = append(out, o.GetId())
		}
		return out
	}
	assert.Empty(t, ids(context.Background()), "no principal sees nothing")
	assert.Len(t, ids(fakes.OperatorContext(context.Background())), 2, "system sees all")
	assert.Len(t, ids(session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceBasic, Operator: true})), 2, "operator sees all")
	assert.Equal(t, []string{orgA.ID.String()}, ids(session.PrincipalInto(context.Background(),
		session.Principal{Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgA.ID})), "person sees the active org")
	assert.Equal(t, []string{orgB.ID.String()}, ids(session.PrincipalInto(context.Background(),
		session.Principal{Source: session.SourceAPIKey, ActiveOrgID: orgB.ID})), "key sees its org")
}
