//go:build integration

package boot

import (
	"context"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authv1 "altalune.id/oagent/gen/go/auth/v1"
	"altalune.id/oagent/gen/go/auth/v1/authv1connect"
	orgv1 "altalune.id/oagent/gen/go/org/v1"
	"altalune.id/oagent/gen/go/org/v1/orgv1connect"
	"altalune.id/oagent/internal/apikey"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/project"
	"altalune.id/oagent/nanoid"
)

func (f *apiFixture) setActiveOrg(cookie, bearer, orgID string) (*authv1.WhoamiResponse, error) {
	req := connect.NewRequest(&authv1.SetActiveOrgRequest{OrgId: orgID})
	if cookie != "" {
		req = withCookie(req, cookie)
	}
	if bearer != "" {
		req.Header().Set("Authorization", "Bearer "+bearer)
	}
	resp, err := authv1connect.NewAuthServiceClient(http.DefaultClient, f.url).SetActiveOrg(context.Background(), req)
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetMe(), nil
}

func (f *apiFixture) getOrg(cookie, orgID string) error {
	_, err := orgv1connect.NewOrgServiceClient(http.DefaultClient, f.url).
		GetOrg(context.Background(), withCookie(connect.NewRequest(&orgv1.GetOrgRequest{Id: orgID}), cookie))
	return err
}

func (f *apiFixture) listOrgIDs(t *testing.T, cookie string) []string {
	t.Helper()
	resp, err := orgv1connect.NewOrgServiceClient(http.DefaultClient, f.url).
		ListOrgs(context.Background(), withCookie(connect.NewRequest(&orgv1.ListOrgsRequest{}), cookie))
	require.NoError(t, err)
	var ids []string
	for _, o := range resp.Msg.GetOrgs() {
		ids = append(ids, o.GetId())
	}
	return ids
}

func (f *apiFixture) newOrg(t *testing.T, slug string) *org.Org {
	t.Helper()
	o, err := org.NewOrg(slug, slug, uuid.Nil)
	require.NoError(t, err)
	require.NoError(t, f.svcs.OrgStore.Save(tenant.WithOrg(t.Context(), o.ID), o))
	_, err = f.svcs.Projects.Create(t.Context(), project.CreateRequest{OrgID: o.ID, Slug: "main", Name: "Main"})
	require.NoError(t, err)
	return o
}

func TestAPI_APersonSwitchesBetweenTheirOrgs(t *testing.T) {
	f := newAPIFixture(t)
	beta, foreign := f.newOrg(t, "beta-org"), f.newOrg(t, "foreign-org")
	cookie, _, userID := f.personCookie(t, "two-orgs@example.com", member.RoleMember)
	_, err := f.svcs.Members.Add(t.Context(), beta.ID, userID, member.RoleAdmin)
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{f.orgID, beta.PublicID}, f.listOrgIDs(t, cookie), "every org the person belongs to, and nothing else")
	require.Error(t, f.getOrg(cookie, beta.PublicID), "before the switch the second org is out of reach")

	me, err := f.setActiveOrg(cookie, "", beta.PublicID)
	require.NoError(t, err)
	assert.Equal(t, beta.PublicID, me.GetActiveOrgId())
	main, err := f.svcs.Projects.BySlug(t.Context(), beta.ID, "main")
	require.NoError(t, err)
	assert.Equal(t, main.PublicID, me.GetActiveProjectId(), "the reply is read in the new org's scope")
	assert.Contains(t, me.GetPermissions(), "member:*", "admin scopes in the new org")

	require.NoError(t, f.getOrg(cookie, beta.PublicID), "the switch holds for later requests")
	require.Error(t, f.getOrg(cookie, f.orgID), "the old org is now out of reach")

	_, err = f.setActiveOrg(cookie, "", foreign.PublicID)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "an org you do not belong to is not found: %v", err)
	unknown, err := nanoid.New(21)
	require.NoError(t, err)
	_, err = f.setActiveOrg(cookie, "", unknown)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "an unknown org is not found: %v", err)

	_, err = f.setActiveOrg("", f.mint(t, apikey.KindOrg, "org:read"), beta.PublicID)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "a key never switches: %v", err)

	require.NoError(t, f.svcs.Members.Remove(t.Context(), member.Requester{Operator: true}, beta.ID, userID))
	cookie, _ = f.signInCookie(t, "two-orgs@example.com")
	assert.Equal(t, []string{f.orgID}, f.listOrgIDs(t, cookie))
	_, err = f.setActiveOrg(cookie, "", beta.PublicID)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "a removed membership can't be switched into: %v", err)
}
