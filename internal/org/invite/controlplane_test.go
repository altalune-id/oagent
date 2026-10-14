package invite_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"altalune.id/oagent/apperror"
	apperrorv1 "altalune.id/oagent/gen/go/apperror/v1"
	invitev1 "altalune.id/oagent/gen/go/invite/v1"
	invitev1connect "altalune.id/oagent/gen/go/invite/v1/invitev1connect"
	invitev1mcp "altalune.id/oagent/gen/go/invite/v1/invitev1mcp"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/org/invite"
	"altalune.id/oagent/internal/org/member"
	rootmcp "altalune.id/oagent/mcp"
)

const orgPublicID = "acmePublicId000000001"

type orgIDs map[string]uuid.UUID

func (o orgIDs) IDByPublicID(_ context.Context, publicID string) (uuid.UUID, error) {
	if id, ok := o[publicID]; ok {
		return id, nil
	}
	return uuid.Nil, &org.NotFoundError{}
}

type cpFixture struct {
	*fixture
	other uuid.UUID
	ids   orgIDs
}

func newCPFixture(t *testing.T) *cpFixture {
	f := &cpFixture{fixture: newFixture(t), other: uuid.New()}
	f.ids = orgIDs{orgPublicID: f.org, "otherPublicId00000001": f.other}
	return f
}

func (f *cpFixture) client(p session.Principal) invitev1connect.InviteServiceClient {
	f.t.Helper()
	_, h := invitev1connect.NewInviteServiceHandler(invite.NewControlPlane(f.svc, f.ids),
		connect.WithInterceptors(controlplane.Wrap(nil), principalInterceptor(p)))
	ts := httptest.NewServer(h)
	f.t.Cleanup(ts.Close)
	return invitev1connect.NewInviteServiceClient(http.DefaultClient, ts.URL)
}

func principalInterceptor(p session.Principal) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			return next(session.PrincipalInto(ctx, p), req)
		}
	}
}

func (f *cpFixture) as(r member.Requester) session.Principal {
	return session.Principal{Source: session.SourceLocal, UserID: r.UserID, ActiveOrgID: f.org, Scopes: []string{"*"}}
}

func errCode(t *testing.T, err error) apperror.Code {
	t.Helper()
	var ce *connect.Error
	require.True(t, errors.As(err, &ce), "not a connect error: %v", err)
	require.NotEmpty(t, ce.Details(), "no error detail: %v", err)
	v, derr := ce.Details()[0].Value()
	require.NoError(t, derr)
	det, ok := v.(*apperrorv1.ErrorDetail)
	require.True(t, ok)
	return apperror.Code(det.GetCode())
}

var uuidShape = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// SECURITY: no UUID may leave the server.
func assertNoUUID(t *testing.T, m proto.Message) {
	t.Helper()
	var walk func(protoreflect.Message)
	walk = func(msg protoreflect.Message) {
		msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
			switch {
			case fd.IsList():
				l := v.List()
				for i := range l.Len() {
					if fd.Kind() == protoreflect.MessageKind {
						walk(l.Get(i).Message())
					} else if fd.Kind() == protoreflect.StringKind {
						assert.False(t, uuidShape.MatchString(l.Get(i).String()), "%s carries a UUID", fd.FullName())
					}
				}
			case fd.Kind() == protoreflect.MessageKind:
				walk(v.Message())
			case fd.Kind() == protoreflect.StringKind:
				assert.False(t, uuidShape.MatchString(v.String()), "%s carries a UUID", fd.FullName())
			}
			return true
		})
	}
	walk(m.ProtoReflect())
}

func TestInviteService_Flow(t *testing.T) {
	f := newCPFixture(t)
	c := f.client(f.as(f.owner))
	ctx := context.Background()

	created, err := c.CreateInvite(ctx, connect.NewRequest(&invitev1.CreateInviteRequest{OrgId: orgPublicID, Email: "bob@x.io", Role: "admin"}))
	require.NoError(t, err)
	assertNoUUID(t, created.Msg)
	inv := created.Msg.GetInvite()
	assert.Len(t, inv.GetId(), 21)
	assert.Equal(t, "bob@x.io", inv.GetEmail())
	assert.Equal(t, "admin", inv.GetRole())
	assert.Equal(t, "pending", inv.GetStatus())
	assert.Equal(t, "Olga", inv.GetInvitedByName())
	assert.Equal(t, f.clock.Add(invite.TTL), inv.GetExpiresAt().AsTime())
	assert.Contains(t, created.Msg.GetLink(), "https://oagent.example.com/invite/")

	resent, err := c.ResendInvite(ctx, connect.NewRequest(&invitev1.ResendInviteRequest{OrgId: orgPublicID, InviteId: inv.GetId()}))
	require.NoError(t, err)
	assertNoUUID(t, resent.Msg)
	assert.NotEqual(t, created.Msg.GetLink(), resent.Msg.GetLink())
	assert.Equal(t, inv.GetId(), resent.Msg.GetInvite().GetId())

	draft, err := c.DraftInvite(ctx, connect.NewRequest(&invitev1.DraftInviteRequest{OrgId: orgPublicID, Email: "dina@x.io"}))
	require.NoError(t, err)
	assertNoUUID(t, draft.Msg)
	assert.Equal(t, "draft", draft.Msg.GetInvite().GetStatus())
	assert.Equal(t, orgPublicID, draft.Msg.GetOrgId())
	assert.Equal(t, "Acme Corp", draft.Msg.GetOrgName())
	assert.Equal(t, "member", draft.Msg.GetInvite().GetRole())

	list, err := c.ListInvites(ctx, connect.NewRequest(&invitev1.ListInvitesRequest{OrgId: orgPublicID}))
	require.NoError(t, err)
	assertNoUUID(t, list.Msg)
	require.Len(t, list.Msg.GetInvites(), 1)
	require.Len(t, list.Msg.GetDrafts(), 1)
	assert.Equal(t, "Olga", list.Msg.GetInvites()[0].GetInvitedByName())

	confirmed, err := c.ConfirmDraftInvite(ctx, connect.NewRequest(&invitev1.ConfirmDraftInviteRequest{OrgId: orgPublicID, InviteId: draft.Msg.GetInvite().GetId()}))
	require.NoError(t, err)
	assertNoUUID(t, confirmed.Msg)
	assert.Equal(t, "pending", confirmed.Msg.GetInvite().GetStatus())

	again, err := c.DraftInvite(ctx, connect.NewRequest(&invitev1.DraftInviteRequest{OrgId: orgPublicID, Email: "zed@x.io"}))
	require.NoError(t, err)
	_, err = c.DiscardDraftInvite(ctx, connect.NewRequest(&invitev1.DiscardDraftInviteRequest{OrgId: orgPublicID, InviteId: again.Msg.GetInvite().GetId()}))
	require.NoError(t, err)
	_, err = c.RevokeInvite(ctx, connect.NewRequest(&invitev1.RevokeInviteRequest{OrgId: orgPublicID, InviteId: inv.GetId()}))
	require.NoError(t, err)

	list, err = c.ListInvites(ctx, connect.NewRequest(&invitev1.ListInvitesRequest{OrgId: orgPublicID}))
	require.NoError(t, err)
	assert.Len(t, list.Msg.GetInvites(), 1, "dina's confirmed invite")
	assert.Empty(t, list.Msg.GetDrafts())
}

func TestInviteService_UnknownIDsAreNotFound(t *testing.T) {
	f := newCPFixture(t)
	c := f.client(f.as(f.owner))
	ctx := context.Background()
	created, err := c.CreateInvite(ctx, connect.NewRequest(&invitev1.CreateInviteRequest{OrgId: orgPublicID, Email: "bob@x.io", Role: "member"}))
	require.NoError(t, err)
	inviteID := created.Msg.GetInvite().GetId()
	stored, err := f.svc.ByPublicID(ctx, inviteID)
	require.NoError(t, err)

	for name, orgID := range map[string]string{
		"unknown org":       "nopeNopeNopeNope00001",
		"org out of reach":  "otherPublicId00000001",
		"the org's UUID":    f.org.String(),
		"another org's id ": f.other.String(),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := c.ListInvites(ctx, connect.NewRequest(&invitev1.ListInvitesRequest{OrgId: orgID}))
			assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "%v", err)
			assert.Equal(t, apperror.CodeOrgNotFound, errCode(t, err))
		})
	}
	for name, id := range map[string]string{
		"unknown invite":    "nopeNopeNopeNope00001",
		"the invite's UUID": stored.ID.String(),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := c.ResendInvite(ctx, connect.NewRequest(&invitev1.ResendInviteRequest{OrgId: orgPublicID, InviteId: id}))
			assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "%v", err)
		})
	}

	_, err = f.members.Add(ctx, f.other, f.owner.UserID, member.RoleOwner)
	require.NoError(t, err)
	both := f.as(f.owner)
	both.ActiveOrgID = f.other
	_, err = f.client(both).RevokeInvite(ctx, connect.NewRequest(&invitev1.RevokeInviteRequest{OrgId: "otherPublicId00000001", InviteId: inviteID}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "an invite of another org: %v", err)

	_, err = c.ListInvites(ctx, connect.NewRequest(&invitev1.ListInvitesRequest{}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "an empty org id is the request rules' to refuse; the handler only looks it up")
	_, err = c.RevokeInvite(ctx, connect.NewRequest(&invitev1.RevokeInviteRequest{OrgId: orgPublicID}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "an empty invite id is the request rules' to refuse; the handler only looks it up")
}

func TestInviteService_OnlyPeopleWrite(t *testing.T) {
	f := newCPFixture(t)
	key := session.Principal{Source: session.SourceAPIKey, KeyID: uuid.New(), ActiveOrgID: f.org, AllProjects: true, Scopes: []string{"*"}}
	_, err := f.client(key).CreateInvite(context.Background(),
		connect.NewRequest(&invitev1.CreateInviteRequest{OrgId: orgPublicID, Email: "bob@x.io", Role: "member"}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	_, err = f.client(key).ListInvites(context.Background(), connect.NewRequest(&invitev1.ListInvitesRequest{OrgId: orgPublicID}))
	require.NoError(t, err, "a whole-org key may list")
}

func TestInviteService_ActorComesFromTheMembership(t *testing.T) {
	f := newCPFixture(t)
	_, err := f.client(f.as(f.admin)).CreateInvite(context.Background(),
		connect.NewRequest(&invitev1.CreateInviteRequest{OrgId: orgPublicID, Email: "bob@x.io", Role: "owner"}))
	assert.Equal(t, apperror.CodeMemberRoleNotAllowed, errCode(t, err), "an admin with a '*' session still acts as admin")
	_, err = f.client(f.as(f.plain)).CreateInvite(context.Background(),
		connect.NewRequest(&invitev1.CreateInviteRequest{OrgId: orgPublicID, Email: "bob@x.io", Role: "member"}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}

func TestInviteService_LinkOnlyWhereItBelongs(t *testing.T) {
	for _, m := range []proto.Message{
		&invitev1.ListInvitesResponse{}, &invitev1.DraftInviteResponse{}, &invitev1.Invite{},
		&invitev1.RevokeInviteResponse{}, &invitev1.DiscardDraftInviteResponse{}, &invitev1.ConfirmDraftInviteResponse{},
	} {
		assert.Nil(t, m.ProtoReflect().Descriptor().Fields().ByName("link"), "%T", m)
	}
	for _, m := range []proto.Message{&invitev1.CreateInviteResponse{}, &invitev1.ResendInviteResponse{}} {
		assert.NotNil(t, m.ProtoReflect().Descriptor().Fields().ByName("link"), "%T", m)
	}
}

func TestInviteService_ExpiredStatus(t *testing.T) {
	f := newCPFixture(t)
	c := f.client(f.as(f.owner))
	_, err := c.CreateInvite(context.Background(), connect.NewRequest(&invitev1.CreateInviteRequest{OrgId: orgPublicID, Email: "bob@x.io", Role: "member"}))
	require.NoError(t, err)
	f.clock = f.clock.Add(invite.TTL + time.Minute)
	list, err := c.ListInvites(context.Background(), connect.NewRequest(&invitev1.ListInvitesRequest{OrgId: orgPublicID}))
	require.NoError(t, err)
	require.Len(t, list.Msg.GetInvites(), 1)
	assert.Equal(t, "expired", list.Msg.GetInvites()[0].GetStatus())
}

func TestMCPMemberInvite_DraftsOnly(t *testing.T) {
	f := newCPFixture(t)
	reg := rootmcp.NewRegistry()
	invitev1mcp.RegisterInviteServiceTools(reg, invite.NewControlPlane(f.svc, f.ids), func(string) []string { return []string{"member:write"} }, func(proto.Message) error { return nil })
	spec, ok := reg.Spec(invitev1mcp.MemberInviteToolName)
	require.True(t, ok)
	ctx := session.PrincipalInto(context.Background(), f.as(f.admin))

	out, err := spec.Handler(ctx, json.RawMessage(`{"orgId":"`+orgPublicID+`","email":"dina@x.io"}`))
	require.NoError(t, err)
	var got struct {
		Invite  struct{ ID, Email, Role, Status string }
		OrgID   string `json:"orgId"`
		OrgName string `json:"orgName"`
	}
	require.NoError(t, json.Unmarshal(out, &got))
	assert.Equal(t, "draft", got.Invite.Status)
	assert.Equal(t, "member", got.Invite.Role)
	assert.Equal(t, orgPublicID, got.OrgID)
	assert.Equal(t, "Acme Corp", got.OrgName)
	assert.NotContains(t, string(out), "link")
	assert.False(t, uuidShape.MatchString(string(out)), "no UUID in the tool result")
	assert.Zero(t, f.mail.count(), "a draft sends no email")
	stored, err := f.svc.ByPublicID(context.Background(), got.Invite.ID)
	require.NoError(t, err)
	assert.Empty(t, stored.TokenHash, "a draft has no token")

	_, err = spec.Handler(ctx, json.RawMessage(`{"orgId":"`+orgPublicID+`","email":"eve@x.io","role":"owner"}`))
	assert.Error(t, err, "the tool takes no role")

	confirm, ok := reg.Spec(invitev1mcp.MemberInviteConfirmToolName)
	require.True(t, ok)
	sent, err := confirm.Handler(ctx, json.RawMessage(`{"orgId":"`+orgPublicID+`","inviteId":"`+got.Invite.ID+`"}`))
	require.NoError(t, err)
	assert.Contains(t, string(sent), `"pending"`)
	assert.NotContains(t, string(sent), "link", "the invite link never reaches a model")
	assert.NotContains(t, string(sent), "/invite/")
}
