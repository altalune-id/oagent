package enduser_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	enduserv1 "altalune.id/oagent/gen/go/enduser/v1"
	"altalune.id/oagent/internal/enduser"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/testutil/fakes"
)

type activity struct {
	seen    map[uuid.UUID]enduser.Seen
	threads map[uuid.UUID][]enduser.PersonThread
}

func (a activity) Seen(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]enduser.Seen, error) {
	out := map[uuid.UUID]enduser.Seen{}
	for _, id := range ids {
		out[id] = a.seen[id]
	}
	return out, nil
}

func (a activity) Threads(_ context.Context, id uuid.UUID) ([]enduser.PersonThread, error) {
	return a.threads[id], nil
}

type cpWorld struct {
	cp                   *enduser.EndUserService
	refs                 *fakes.PublicIDs
	org, project, agent  uuid.UUID
	projectPub, agentPub string
	sari, budi           *enduser.EndUser
}

const sariPhone = "6281234567890"

func newCPWorld(t *testing.T) *cpWorld {
	t.Helper()
	w := &cpWorld{refs: fakes.NewPublicIDs(), org: uuid.New(), project: uuid.New(), agent: uuid.New()}
	w.refs.Org(w.org)
	w.projectPub = w.refs.Project(w.project, w.org)
	w.agentPub = w.refs.Resource(w.agent, w.org)
	store := &fakes.EndUserStore{}
	svc := enduser.NewService(store, fakes.Transactor{}, nil)
	ctx := tenant.Into(context.Background(), tenant.Context{OrgID: w.org, ProjectID: w.project})
	var err error
	w.sari, err = svc.Upsert(ctx, w.project, enduser.Identity{Kind: enduser.KindWAPhone, Value: sariPhone})
	require.NoError(t, err)
	require.NoError(t, svc.SetDisplayName(ctx, w.sari.ID, "Sari Wulandari"))
	w.budi, err = svc.Upsert(ctx, w.project, enduser.Identity{Kind: enduser.KindWALid, Value: "123456789012345"})
	require.NoError(t, err)
	seenAt := time.Now().Add(-time.Minute)
	act := activity{
		seen: map[uuid.UUID]enduser.Seen{w.sari.ID: {Last: seenAt, Threads: 2}},
		threads: map[uuid.UUID][]enduser.PersonThread{w.sari.ID: {
			{PublicID: "thrdthrdthrdthrdthrd1", OrgID: w.org, AgentID: w.agent, Kind: "dm", LastActivity: seenAt},
			{PublicID: "thrdthrdthrdthrdthrd2", OrgID: w.org, AgentID: w.agent, Kind: "group", LastActivity: seenAt},
		}},
	}
	w.cp = enduser.NewControlPlane(svc, w.refs, act)
	return w
}

func person(orgID uuid.UUID) context.Context {
	return session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceLocal, UserID: uuid.New(), ActiveOrgID: orgID, Scopes: []string{"*"}})
}

func TestEndUserService_ListAndGetShowNoIdentityValue(t *testing.T) {
	w := newCPWorld(t)
	list, err := w.cp.ListEndUsers(person(w.org), connect.NewRequest(&enduserv1.ListEndUsersRequest{ProjectId: w.projectPub}))
	require.NoError(t, err)
	users := list.Msg.GetEndUsers()
	require.Len(t, users, 2)
	byID := map[string]*enduserv1.EndUser{}
	for _, u := range users {
		byID[u.GetId()] = u
		assert.Equal(t, w.projectPub, u.GetProjectId())
		assert.NotNil(t, u.GetFirstSeenAt())
	}
	sari := byID[w.sari.PublicID]
	require.NotNil(t, sari)
	assert.Equal(t, "Sari Wulandari", sari.GetDisplayName())
	assert.Equal(t, enduserv1.IdentityKind_IDENTITY_KIND_WA_PHONE, sari.GetIdentityKind())
	assert.Equal(t, "90", sari.GetIdentityHint())
	assert.Equal(t, int64(2), sari.GetThreadCount())
	assert.NotNil(t, sari.GetLastSeenAt())
	budi := byID[w.budi.PublicID]
	assert.Equal(t, enduserv1.IdentityKind_IDENTITY_KIND_WA_LID, budi.GetIdentityKind())
	assert.Equal(t, "45", budi.GetIdentityHint())
	assert.Zero(t, budi.GetThreadCount())

	got, err := w.cp.GetEndUser(person(w.org), connect.NewRequest(&enduserv1.GetEndUserRequest{Id: w.sari.PublicID}))
	require.NoError(t, err)
	assert.Equal(t, "Sari Wulandari", got.Msg.GetEndUser().GetDisplayName())
	threads := got.Msg.GetThreads()
	require.Len(t, threads, 2)
	assert.Equal(t, w.agentPub, threads[0].GetAgentId())
	assert.Equal(t, "thrdthrdthrdthrdthrd1", threads[0].GetId())
	assert.Equal(t, "dm", threads[0].GetKind())

	for _, b := range []string{protojson.Format(list.Msg), protojson.Format(got.Msg)} {
		assert.NotContains(t, b, sariPhone)
		assert.NotContains(t, b, "123456789012345")
		for _, id := range []uuid.UUID{w.org, w.project, w.agent, w.sari.ID, w.budi.ID} {
			assert.NotContains(t, b, id.String(), "public ids only")
		}
	}
}

func TestEndUserReach_WholeProjectOnly(t *testing.T) {
	w := newCPWorld(t)
	for name, ctx := range map[string]context.Context{
		"member of another org": person(uuid.New()),
		"agent-scoped key": session.PrincipalInto(context.Background(), session.Principal{
			Source: session.SourceAPIKey, ActiveOrgID: w.org, ProjectIDs: []uuid.UUID{w.project}, ResourceIDs: []uuid.UUID{w.agent},
		}),
	} {
		_, err := w.cp.ListEndUsers(ctx, connect.NewRequest(&enduserv1.ListEndUsersRequest{ProjectId: w.projectPub}))
		assert.True(t, enduser.IsNotFoundError(err), "%s list: %v", name, err)
		for _, id := range []string{w.sari.PublicID, w.sari.ID.String()} {
			_, err = w.cp.GetEndUser(ctx, connect.NewRequest(&enduserv1.GetEndUserRequest{Id: id}))
			assert.True(t, enduser.IsNotFoundError(err), "%s get %s: %v", name, id, err)
		}
	}
	whole := session.PrincipalInto(context.Background(), session.Principal{Source: session.SourceAPIKey, ActiveOrgID: w.org, ProjectIDs: []uuid.UUID{w.project}})
	_, err := w.cp.GetEndUser(whole, connect.NewRequest(&enduserv1.GetEndUserRequest{Id: w.sari.PublicID}))
	assert.NoError(t, err, "a key that holds the whole project reads its end users")
}

func TestListEndUsers_PagesAndRefusesAForeignToken(t *testing.T) {
	w := newCPWorld(t)
	first, err := w.cp.ListEndUsers(person(w.org), connect.NewRequest(&enduserv1.ListEndUsersRequest{ProjectId: w.projectPub, PageSize: 1}))
	require.NoError(t, err)
	require.Len(t, first.Msg.GetEndUsers(), 1)
	require.NotEmpty(t, first.Msg.GetNextPageToken(), "a full page names the next one")
	_, err = w.cp.ListEndUsers(person(w.org), connect.NewRequest(&enduserv1.ListEndUsersRequest{ProjectId: w.projectPub, PageToken: first.Msg.GetNextPageToken()}))
	require.NoError(t, err)
	for _, bad := range []string{"%%%", "bm9waXBl", "bm90LWEtdGltZXxwdWI"} {
		_, err = w.cp.ListEndUsers(person(w.org), connect.NewRequest(&enduserv1.ListEndUsersRequest{ProjectId: w.projectPub, PageToken: bad}))
		var invalid *enduser.InvalidPageTokenError
		assert.ErrorAs(t, err, &invalid, bad)
	}
	_, err = w.cp.ListEndUsers(person(w.org), connect.NewRequest(&enduserv1.ListEndUsersRequest{ProjectId: "unknwnunknwnunknwnabc"}))
	assert.True(t, enduser.IsNotFoundError(err), "an unknown project")
	_, err = w.cp.GetEndUser(person(w.org), connect.NewRequest(&enduserv1.GetEndUserRequest{Id: "unknwnunknwnunknwnabc"}))
	assert.True(t, enduser.IsNotFoundError(err), "an unknown end user")
	assert.NotEmpty(t, (&enduser.InvalidPageTokenError{}).Error())
	assert.Equal(t, "page_token", (&enduser.InvalidPageTokenError{}).ToAppError().Violations()[0].Field)
}
