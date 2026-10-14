package member_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	apperrorv1 "altalune.id/oagent/gen/go/apperror/v1"
	memberv1 "altalune.id/oagent/gen/go/member/v1"
	memberv1connect "altalune.id/oagent/gen/go/member/v1/memberv1connect"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/kernel/authz"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/testutil/fakes"
)

type cpFixture struct {
	t     *testing.T
	store *fakes.MemberStore
	svc   *member.Service
	ids   *fakes.PublicIDs
	org   uuid.UUID
}

func newCPFixture(t *testing.T) *cpFixture {
	store := &fakes.MemberStore{}
	return &cpFixture{t: t, store: store, svc: newService(store), ids: fakes.NewPublicIDs(), org: uuid.New()}
}

func (f *cpFixture) orgPub() string { return f.ids.Org(f.org) }

func (f *cpFixture) user(id uuid.UUID) string { return f.ids.User(id) }

func (f *cpFixture) add(role member.Role, email string) uuid.UUID {
	f.t.Helper()
	m, err := f.svc.Add(context.Background(), f.org, uuid.New(), role)
	require.NoError(f.t, err)
	f.store.SetContact(m.UserID, fakes.MemberContact{PublicID: f.user(m.UserID), Email: email, Name: "Name " + email})
	return m.UserID
}

func (f *cpFixture) client(p session.Principal) memberv1connect.MemberServiceClient {
	f.t.Helper()
	_, handler := memberv1connect.NewMemberServiceHandler(member.NewControlPlane(f.svc, f.ids),
		connect.WithInterceptors(controlplane.Wrap(nil), principalInterceptor(p)))
	ts := httptest.NewServer(handler)
	f.t.Cleanup(ts.Close)
	return memberv1connect.NewMemberServiceClient(http.DefaultClient, ts.URL)
}

func (f *cpFixture) role(userID uuid.UUID) member.Role {
	f.t.Helper()
	m, err := f.svc.Find(context.Background(), f.org, userID)
	require.NoError(f.t, err)
	return m.Role
}

func principalInterceptor(p session.Principal) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if p.Source != "" {
				ctx = session.PrincipalInto(ctx, p)
			}
			return next(ctx, req)
		}
	}
}

func personIn(userID, orgID uuid.UUID) session.Principal {
	return session.Principal{Source: session.SourceLocal, UserID: userID, ActiveOrgID: orgID, Scopes: []string{"*"}}
}

func keyIn(orgID uuid.UUID) session.Principal {
	return session.Principal{Source: session.SourceAPIKey, KeyID: uuid.New(), ActiveOrgID: orgID, AllProjects: true, Scopes: []string{"member:read"}}
}

func operatorPrincipal() session.Principal {
	return session.Principal{Source: session.SourceBasic, Operator: true}
}

func errDetail(t *testing.T, err error) *apperrorv1.ErrorDetail {
	t.Helper()
	var ce *connect.Error
	require.True(t, errors.As(err, &ce), "not a connect error: %v", err)
	require.NotEmpty(t, ce.Details(), "no error detail: %v", err)
	v, derr := ce.Details()[0].Value()
	require.NoError(t, derr)
	det, ok := v.(*apperrorv1.ErrorDetail)
	require.True(t, ok)
	return det
}

func errCode(t *testing.T, err error) apperror.Code {
	t.Helper()
	return apperror.Code(errDetail(t, err).GetCode())
}

func TestMemberService_ListMembers(t *testing.T) {
	f := newCPFixture(t)
	owner := f.add(member.RoleOwner, "owner@example.com")
	mem := f.add(member.RoleMember, "member@example.com")

	resp, err := f.client(personIn(mem, f.org)).ListMembers(context.Background(),
		connect.NewRequest(&memberv1.ListMembersRequest{OrgId: f.orgPub()}))
	require.NoError(t, err)
	rows := resp.Msg.GetMembers()
	require.Len(t, rows, 2)
	assert.Equal(t, f.user(owner), rows[0].GetUserId())
	assert.Equal(t, "owner@example.com", rows[0].GetEmail())
	assert.Equal(t, "Name owner@example.com", rows[0].GetName())
	assert.Equal(t, "owner", rows[0].GetRole())
	assert.False(t, rows[0].GetJoinedAt().AsTime().IsZero())
	assert.Equal(t, "member", rows[1].GetRole())
}

// SECURITY: member emails are org-wide data, so a key limited to some projects must not list them.
func TestMemberService_ListMembers_NeedsAWholeOrgKey(t *testing.T) {
	f := newCPFixture(t)
	f.add(member.RoleOwner, "owner@example.com")
	list := func(p session.Principal) error {
		_, err := f.client(p).ListMembers(context.Background(),
			connect.NewRequest(&memberv1.ListMembersRequest{OrgId: f.orgPub()}))
		return err
	}

	require.NoError(t, list(keyIn(f.org)), "a whole-org key lists members")
	limited := keyIn(f.org)
	limited.AllProjects = false
	limited.ProjectIDs = []uuid.UUID{uuid.New()}
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(list(limited)))
}

func TestMemberService_ListMembers_StoreError(t *testing.T) {
	f := newCPFixture(t)
	f.store.Err = assert.AnError
	_, err := f.client(operatorPrincipal()).ListMembers(context.Background(),
		connect.NewRequest(&memberv1.ListMembersRequest{OrgId: f.orgPub()}))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}

// SECURITY: an unknown, malformed, UUID-shaped or out-of-reach id answers the same not-found as a missing membership.
func TestMemberService_UnknownIDs(t *testing.T) {
	f := newCPFixture(t)
	owner := f.add(member.RoleOwner, "owner@example.com")
	target := f.add(member.RoleMember, "member@example.com")
	c := f.client(personIn(owner, f.org))
	ctx := context.Background()
	org, user := f.orgPub(), f.user(target)

	for name, orgID := range map[string]string{
		"malformed":     "bad",
		"uuid":          f.org.String(),
		"another org's": f.ids.Org(uuid.New()),
		"a user's":      user,
	} {
		t.Run(name+" org", func(t *testing.T) {
			for call, err := range map[string]error{
				"list": func() error {
					_, err := c.ListMembers(ctx, connect.NewRequest(&memberv1.ListMembersRequest{OrgId: orgID}))
					return err
				}(),
				"set": func() error {
					_, err := c.SetMemberRole(ctx, connect.NewRequest(&memberv1.SetMemberRoleRequest{OrgId: orgID, UserId: user, Role: "admin"}))
					return err
				}(),
				"remove": func() error {
					_, err := c.RemoveMember(ctx, connect.NewRequest(&memberv1.RemoveMemberRequest{OrgId: orgID, UserId: user}))
					return err
				}(),
				"leave": func() error {
					_, err := c.LeaveOrg(ctx, connect.NewRequest(&memberv1.LeaveOrgRequest{OrgId: orgID}))
					return err
				}(),
			} {
				assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), call)
				assert.Equal(t, apperror.CodeMemberNotFound, errCode(t, err), call)
			}
		})
	}
	for name, userID := range map[string]string{
		"malformed":      "bad",
		"uuid":           target.String(),
		"the org's":      org,
		"a non-member's": f.user(uuid.New()),
		"unregistered":   "V1StGXR8_Z5jdHi6B-myT",
	} {
		t.Run(name+" user", func(t *testing.T) {
			_, err := c.SetMemberRole(ctx, connect.NewRequest(&memberv1.SetMemberRoleRequest{OrgId: org, UserId: userID, Role: "admin"}))
			assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "set")
			assert.Equal(t, apperror.CodeMemberNotFound, errCode(t, err), "set")
			_, err = c.RemoveMember(ctx, connect.NewRequest(&memberv1.RemoveMemberRequest{OrgId: org, UserId: userID}))
			assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "remove")
			assert.Equal(t, apperror.CodeMemberNotFound, errCode(t, err), "remove")
		})
	}
	assert.Equal(t, member.RoleMember, f.role(target))
}

// SECURITY: a key never changes or removes members, even inside its own org.
func TestMemberService_KeyCannotWrite(t *testing.T) {
	f := newCPFixture(t)
	f.add(member.RoleOwner, "owner@example.com")
	target := f.add(member.RoleMember, "member@example.com")
	c := f.client(keyIn(f.org))
	ctx := context.Background()

	resp, err := c.ListMembers(ctx, connect.NewRequest(&memberv1.ListMembersRequest{OrgId: f.orgPub()}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.GetMembers(), 2)

	// NOTE: the store fails every call, so reaching the service would answer Internal, not these codes.
	f.store.Err = assert.AnError
	_, err = c.SetMemberRole(ctx, connect.NewRequest(&memberv1.SetMemberRoleRequest{OrgId: f.orgPub(), UserId: f.user(target), Role: "admin"}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	_, err = c.RemoveMember(ctx, connect.NewRequest(&memberv1.RemoveMemberRequest{OrgId: f.orgPub(), UserId: f.user(target)}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	_, err = c.LeaveOrg(ctx, connect.NewRequest(&memberv1.LeaveOrgRequest{OrgId: f.orgPub()}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	f.store.Err = nil
	assert.Equal(t, member.RoleMember, f.role(target))
}

func TestMemberService_ActorRoleComesFromMembership(t *testing.T) {
	ctx := context.Background()
	set := func(c memberv1connect.MemberServiceClient, f *cpFixture, user uuid.UUID, role string) error {
		_, err := c.SetMemberRole(ctx, connect.NewRequest(&memberv1.SetMemberRoleRequest{OrgId: f.orgPub(), UserId: f.user(user), Role: role}))
		return err
	}

	t.Run("an owner promotes to owner", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		target := f.add(member.RoleMember, "member@example.com")
		resp, err := f.client(personIn(owner, f.org)).SetMemberRole(ctx,
			connect.NewRequest(&memberv1.SetMemberRoleRequest{OrgId: f.orgPub(), UserId: f.user(target), Role: "owner"}))
		require.NoError(t, err)
		assert.Equal(t, f.user(target), resp.Msg.GetMember().GetUserId())
		assert.Equal(t, "owner", resp.Msg.GetMember().GetRole())
		assert.Equal(t, "member@example.com", resp.Msg.GetMember().GetEmail())
		assert.Equal(t, member.RoleOwner, f.role(target))
	})
	t.Run("an admin with * in its session scopes still cannot touch an owner", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		admin := f.add(member.RoleAdmin, "admin@example.com")
		err := set(f.client(personIn(admin, f.org)), f, owner, "admin")
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.Equal(t, apperror.CodeMemberRoleNotAllowed, errCode(t, err))
		assert.Equal(t, member.RoleOwner, f.role(owner))
	})
	t.Run("a member cannot change roles", func(t *testing.T) {
		f := newCPFixture(t)
		f.add(member.RoleOwner, "owner@example.com")
		mem := f.add(member.RoleMember, "member@example.com")
		other := f.add(member.RoleMember, "other@example.com")
		err := set(f.client(personIn(mem, f.org)), f, other, "admin")
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.Equal(t, member.RoleMember, f.role(other))
	})
	t.Run("a person who is not a member cannot change roles", func(t *testing.T) {
		f := newCPFixture(t)
		f.add(member.RoleOwner, "owner@example.com")
		other := f.add(member.RoleMember, "other@example.com")
		err := set(f.client(personIn(uuid.New(), f.org)), f, other, "admin")
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.Equal(t, member.RoleMember, f.role(other))
	})
	t.Run("an operator acts as operator", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		f.add(member.RoleOwner, "second@example.com")
		require.NoError(t, set(f.client(operatorPrincipal()), f, owner, "member"))
		assert.Equal(t, member.RoleMember, f.role(owner))
	})
	t.Run("a self change goes through leave", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		f.add(member.RoleOwner, "second@example.com")
		err := set(f.client(personIn(owner, f.org)), f, owner, "member")
		assert.Equal(t, apperror.CodeMemberSelfChange, errCode(t, err))
	})
	t.Run("a role lookup failure is not a missing membership", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		target := f.add(member.RoleMember, "member@example.com")
		f.store.Err = assert.AnError
		err := set(f.client(personIn(owner, f.org)), f, target, "admin")
		assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	})
}

func TestMemberService_RemoveMember(t *testing.T) {
	ctx := context.Background()
	remove := func(c memberv1connect.MemberServiceClient, f *cpFixture, user uuid.UUID) error {
		_, err := c.RemoveMember(ctx, connect.NewRequest(&memberv1.RemoveMemberRequest{OrgId: f.orgPub(), UserId: f.user(user)}))
		return err
	}

	t.Run("an admin removes a member", func(t *testing.T) {
		f := newCPFixture(t)
		f.add(member.RoleOwner, "owner@example.com")
		admin := f.add(member.RoleAdmin, "admin@example.com")
		target := f.add(member.RoleMember, "member@example.com")
		require.NoError(t, remove(f.client(personIn(admin, f.org)), f, target))
		_, err := f.svc.Find(ctx, f.org, target)
		assert.True(t, member.IsNotFoundError(err))
	})
	t.Run("an admin cannot remove an owner", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		admin := f.add(member.RoleAdmin, "admin@example.com")
		err := remove(f.client(personIn(admin, f.org)), f, owner)
		assert.Equal(t, apperror.CodeMemberRoleNotAllowed, errCode(t, err))
		assert.Equal(t, member.RoleOwner, f.role(owner))
	})
	t.Run("an operator removes anyone but the last owner", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		err := remove(f.client(operatorPrincipal()), f, owner)
		assert.Equal(t, apperror.CodeMemberLastOwner, errCode(t, err))
	})
	t.Run("an unknown member is not found", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		err := remove(f.client(personIn(owner, f.org)), f, uuid.New())
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	})
}

func TestMemberService_LeaveOrg(t *testing.T) {
	ctx := context.Background()
	leave := func(c memberv1connect.MemberServiceClient, f *cpFixture) error {
		_, err := c.LeaveOrg(ctx, connect.NewRequest(&memberv1.LeaveOrgRequest{OrgId: f.orgPub()}))
		return err
	}

	t.Run("a member leaves as themselves", func(t *testing.T) {
		f := newCPFixture(t)
		f.add(member.RoleOwner, "owner@example.com")
		mem := f.add(member.RoleMember, "member@example.com")
		require.NoError(t, leave(f.client(personIn(mem, f.org)), f))
		_, err := f.svc.Find(ctx, f.org, mem)
		assert.True(t, member.IsNotFoundError(err))
	})
	t.Run("the last owner cannot leave", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		err := leave(f.client(personIn(owner, f.org)), f)
		assert.Equal(t, apperror.CodeMemberLastOwner, errCode(t, err))
	})
	t.Run("an operator has no membership to leave", func(t *testing.T) {
		f := newCPFixture(t)
		f.add(member.RoleOwner, "owner@example.com")
		err := leave(f.client(operatorPrincipal()), f)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})
}

func staffPerson(userID uuid.UUID) session.Principal {
	return session.Principal{Source: session.SourceLocal, UserID: userID, ActiveOrgID: uuid.New(), Scopes: []string{"*"}, Operator: true}
}

// SECURITY: staff (an operator who is a person) manage like an admin only outside their own orgs, and only where operator support is on.
func TestMemberService_OperatorPerson(t *testing.T) {
	ctx := context.Background()
	set := func(c memberv1connect.MemberServiceClient, f *cpFixture, user uuid.UUID, role string) error {
		_, err := c.SetMemberRole(ctx, connect.NewRequest(&memberv1.SetMemberRoleRequest{OrgId: f.orgPub(), UserId: f.user(user), Role: role}))
		return err
	}
	remove := func(c memberv1connect.MemberServiceClient, f *cpFixture, user uuid.UUID) error {
		_, err := c.RemoveMember(ctx, connect.NewRequest(&memberv1.RemoveMemberRequest{OrgId: f.orgPub(), UserId: f.user(user)}))
		return err
	}

	t.Run("outside their orgs with support on they manage like an admin", func(t *testing.T) {
		f := newCPFixture(t)
		f.svc.WithOperatorSupport(true)
		owner := f.add(member.RoleOwner, "owner@example.com")
		plain := f.add(member.RoleMember, "member@example.com")
		other := f.add(member.RoleMember, "other@example.com")
		c := f.client(staffPerson(uuid.New()))
		require.NoError(t, set(c, f, plain, "admin"))
		assert.Equal(t, member.RoleAdmin, f.role(plain))
		require.NoError(t, remove(c, f, other))
		assert.Equal(t, apperror.CodeMemberRoleNotAllowed, errCode(t, set(c, f, owner, "member")))
		assert.Equal(t, apperror.CodeMemberRoleNotAllowed, errCode(t, set(c, f, plain, "owner")))
		assert.Equal(t, apperror.CodeMemberRoleNotAllowed, errCode(t, remove(c, f, owner)))
		assert.Equal(t, member.RoleOwner, f.role(owner))
	})
	t.Run("outside their orgs with support off they are refused", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		target := f.add(member.RoleMember, "member@example.com")
		c := f.client(staffPerson(uuid.New()))
		err := set(c, f, target, "admin")
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(remove(c, f, owner)))
		assert.Equal(t, member.RoleMember, f.role(target))
	})
	for _, support := range []bool{false, true} {
		t.Run(fmt.Sprintf("inside their org they follow their role (support %v)", support), func(t *testing.T) {
			f := newCPFixture(t)
			f.svc.WithOperatorSupport(support)
			owner := f.add(member.RoleOwner, "owner@example.com")
			f.add(member.RoleOwner, "second@example.com")
			admin := f.add(member.RoleAdmin, "admin@example.com")
			plain := f.add(member.RoleMember, "member@example.com")
			c := f.client(staffPerson(admin))
			assert.Equal(t, apperror.CodeMemberSelfChange, errCode(t, set(c, f, admin, "member")))
			assert.Equal(t, apperror.CodeMemberRoleNotAllowed, errCode(t, set(c, f, owner, "admin")))
			assert.Equal(t, apperror.CodeMemberRoleNotAllowed, errCode(t, remove(c, f, owner)))
			assert.Equal(t, apperror.CodeMemberRoleNotAllowed, errCode(t, set(c, f, plain, "owner")))
			require.NoError(t, set(c, f, plain, "admin"), "an admin still manages members")
			assert.Equal(t, member.RoleOwner, f.role(owner))
			assert.Equal(t, member.RoleAdmin, f.role(admin))
		})
	}
}

func TestRequesterFrom(t *testing.T) {
	person := uuid.New()
	cases := []struct {
		name string
		p    session.Principal
		want member.Requester
		ok   bool
	}{
		{"person", session.Principal{UserID: person, Source: session.SourceLocal}, member.Requester{UserID: person}, true},
		{"staff", session.Principal{UserID: person, Source: session.SourceLocal, Operator: true}, member.Requester{UserID: person, Operator: true}, true},
		{"non-person operator", session.System(), member.Requester{Operator: true}, true},
		{"key", session.Principal{KeyID: uuid.New(), OwnerID: person, Source: session.SourceAPIKey}, member.Requester{}, false},
		{"nobody", session.Principal{}, member.Requester{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := member.RequesterFrom(session.PrincipalInto(context.Background(), tc.p))
			if !tc.ok {
				assert.True(t, authz.IsForbiddenError(err), "got %v", err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// Review Focus 4: with no request rules in front, the domain still refuses an unknown role.
func TestMemberService_DomainRefusesAnUnknownRoleWithoutTheRules(t *testing.T) {
	f := newCPFixture(t)
	owner := f.add(member.RoleOwner, "owner@example.com")
	target := f.add(member.RoleMember, "member@example.com")
	c := f.client(personIn(owner, f.org))
	_, err := c.SetMemberRole(context.Background(), connect.NewRequest(&memberv1.SetMemberRoleRequest{OrgId: f.orgPub(), UserId: f.user(target), Role: "root"}))
	assert.Equal(t, apperror.CodeMemberInvalidRole, errCode(t, err))
	assert.Equal(t, member.RoleMember, f.role(target))
}
