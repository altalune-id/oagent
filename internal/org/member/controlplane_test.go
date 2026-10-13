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
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/testutil/fakes"
)

type cpFixture struct {
	t     *testing.T
	store *fakes.MemberStore
	svc   *member.Service
	org   uuid.UUID
}

func newCPFixture(t *testing.T) *cpFixture {
	store := &fakes.MemberStore{}
	return &cpFixture{t: t, store: store, svc: newService(store), org: uuid.New()}
}

func (f *cpFixture) add(role member.Role, email string) uuid.UUID {
	f.t.Helper()
	m, err := f.svc.Add(context.Background(), f.org, uuid.New(), role)
	require.NoError(f.t, err)
	f.store.SetContact(m.UserID, fakes.MemberContact{Email: email, Name: "Name " + email})
	return m.UserID
}

func (f *cpFixture) client(p session.Principal) memberv1connect.MemberServiceClient {
	f.t.Helper()
	_, handler := memberv1connect.NewMemberServiceHandler(member.NewControlPlane(f.svc),
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
		connect.NewRequest(&memberv1.ListMembersRequest{OrgId: f.org.String()}))
	require.NoError(t, err)
	rows := resp.Msg.GetMembers()
	require.Len(t, rows, 2)
	assert.Equal(t, owner.String(), rows[0].GetUserId())
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
			connect.NewRequest(&memberv1.ListMembersRequest{OrgId: f.org.String()}))
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
		connect.NewRequest(&memberv1.ListMembersRequest{OrgId: f.org.String()}))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}

func TestMemberService_BadIDs(t *testing.T) {
	f := newCPFixture(t)
	c := f.client(operatorPrincipal())
	ctx := context.Background()
	org, user := f.org.String(), uuid.New().String()

	for name, call := range map[string]func() error{
		"list org": func() error {
			_, err := c.ListMembers(ctx, connect.NewRequest(&memberv1.ListMembersRequest{OrgId: "bad"}))
			return err
		},
		"set org": func() error {
			_, err := c.SetMemberRole(ctx, connect.NewRequest(&memberv1.SetMemberRoleRequest{OrgId: "bad", UserId: user, Role: "admin"}))
			return err
		},
		"remove org": func() error {
			_, err := c.RemoveMember(ctx, connect.NewRequest(&memberv1.RemoveMemberRequest{OrgId: "bad", UserId: user}))
			return err
		},
		"leave org": func() error {
			_, err := c.LeaveOrg(ctx, connect.NewRequest(&memberv1.LeaveOrgRequest{OrgId: "bad"}))
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := call()
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			assert.Equal(t, apperror.CodeValidation, errCode(t, err))
			assert.Equal(t, "organization_id", errDetail(t, err).GetMeta()["field"])
		})
	}
	for name, call := range map[string]func() error{
		"set user": func() error {
			_, err := c.SetMemberRole(ctx, connect.NewRequest(&memberv1.SetMemberRoleRequest{OrgId: org, UserId: "bad", Role: "admin"}))
			return err
		},
		"remove user": func() error {
			_, err := c.RemoveMember(ctx, connect.NewRequest(&memberv1.RemoveMemberRequest{OrgId: org, UserId: "bad"}))
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := call()
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			assert.Equal(t, apperror.CodeValidation, errCode(t, err))
			assert.Equal(t, "user_id", errDetail(t, err).GetMeta()["field"])
		})
	}
}

// SECURITY: a key never changes or removes members, even inside its own org.
func TestMemberService_KeyCannotWrite(t *testing.T) {
	f := newCPFixture(t)
	f.add(member.RoleOwner, "owner@example.com")
	target := f.add(member.RoleMember, "member@example.com")
	c := f.client(keyIn(f.org))
	ctx := context.Background()

	resp, err := c.ListMembers(ctx, connect.NewRequest(&memberv1.ListMembersRequest{OrgId: f.org.String()}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.GetMembers(), 2)

	// NOTE: the store fails every call, so reaching the service would answer Internal, not these codes.
	f.store.Err = assert.AnError
	_, err = c.SetMemberRole(ctx, connect.NewRequest(&memberv1.SetMemberRoleRequest{OrgId: f.org.String(), UserId: target.String(), Role: "admin"}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	_, err = c.RemoveMember(ctx, connect.NewRequest(&memberv1.RemoveMemberRequest{OrgId: f.org.String(), UserId: target.String()}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	_, err = c.LeaveOrg(ctx, connect.NewRequest(&memberv1.LeaveOrgRequest{OrgId: f.org.String()}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	f.store.Err = nil
	assert.Equal(t, member.RoleMember, f.role(target))
}

func TestMemberService_ActorRoleComesFromMembership(t *testing.T) {
	ctx := context.Background()
	set := func(c memberv1connect.MemberServiceClient, org, user uuid.UUID, role string) error {
		_, err := c.SetMemberRole(ctx, connect.NewRequest(&memberv1.SetMemberRoleRequest{OrgId: org.String(), UserId: user.String(), Role: role}))
		return err
	}

	t.Run("an owner promotes to owner", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		target := f.add(member.RoleMember, "member@example.com")
		resp, err := f.client(personIn(owner, f.org)).SetMemberRole(ctx,
			connect.NewRequest(&memberv1.SetMemberRoleRequest{OrgId: f.org.String(), UserId: target.String(), Role: "owner"}))
		require.NoError(t, err)
		assert.Equal(t, target.String(), resp.Msg.GetMember().GetUserId())
		assert.Equal(t, "owner", resp.Msg.GetMember().GetRole())
		assert.Equal(t, "member@example.com", resp.Msg.GetMember().GetEmail())
		assert.Equal(t, member.RoleOwner, f.role(target))
	})
	t.Run("an admin with * in its session scopes still cannot touch an owner", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		admin := f.add(member.RoleAdmin, "admin@example.com")
		err := set(f.client(personIn(admin, f.org)), f.org, owner, "admin")
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.Equal(t, apperror.CodeMemberRoleNotAllowed, errCode(t, err))
		assert.Equal(t, member.RoleOwner, f.role(owner))
	})
	t.Run("a member cannot change roles", func(t *testing.T) {
		f := newCPFixture(t)
		f.add(member.RoleOwner, "owner@example.com")
		mem := f.add(member.RoleMember, "member@example.com")
		other := f.add(member.RoleMember, "other@example.com")
		err := set(f.client(personIn(mem, f.org)), f.org, other, "admin")
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.Equal(t, member.RoleMember, f.role(other))
	})
	t.Run("a person who is not a member cannot change roles", func(t *testing.T) {
		f := newCPFixture(t)
		f.add(member.RoleOwner, "owner@example.com")
		other := f.add(member.RoleMember, "other@example.com")
		err := set(f.client(personIn(uuid.New(), f.org)), f.org, other, "admin")
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.Equal(t, member.RoleMember, f.role(other))
	})
	t.Run("an operator acts as operator", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		f.add(member.RoleOwner, "second@example.com")
		require.NoError(t, set(f.client(operatorPrincipal()), f.org, owner, "member"))
		assert.Equal(t, member.RoleMember, f.role(owner))
	})
	t.Run("a self change goes through leave", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		f.add(member.RoleOwner, "second@example.com")
		err := set(f.client(personIn(owner, f.org)), f.org, owner, "member")
		assert.Equal(t, apperror.CodeMemberSelfChange, errCode(t, err))
	})
	t.Run("a role lookup failure is not a missing membership", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		target := f.add(member.RoleMember, "member@example.com")
		f.store.Err = assert.AnError
		err := set(f.client(personIn(owner, f.org)), f.org, target, "admin")
		assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	})
}

func TestMemberService_RemoveMember(t *testing.T) {
	ctx := context.Background()
	remove := func(c memberv1connect.MemberServiceClient, org, user uuid.UUID) error {
		_, err := c.RemoveMember(ctx, connect.NewRequest(&memberv1.RemoveMemberRequest{OrgId: org.String(), UserId: user.String()}))
		return err
	}

	t.Run("an admin removes a member", func(t *testing.T) {
		f := newCPFixture(t)
		f.add(member.RoleOwner, "owner@example.com")
		admin := f.add(member.RoleAdmin, "admin@example.com")
		target := f.add(member.RoleMember, "member@example.com")
		require.NoError(t, remove(f.client(personIn(admin, f.org)), f.org, target))
		_, err := f.svc.Find(ctx, f.org, target)
		assert.True(t, member.IsNotFoundError(err))
	})
	t.Run("an admin cannot remove an owner", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		admin := f.add(member.RoleAdmin, "admin@example.com")
		err := remove(f.client(personIn(admin, f.org)), f.org, owner)
		assert.Equal(t, apperror.CodeMemberRoleNotAllowed, errCode(t, err))
		assert.Equal(t, member.RoleOwner, f.role(owner))
	})
	t.Run("an operator removes anyone but the last owner", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		err := remove(f.client(operatorPrincipal()), f.org, owner)
		assert.Equal(t, apperror.CodeMemberLastOwner, errCode(t, err))
	})
	t.Run("an unknown member is not found", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		err := remove(f.client(personIn(owner, f.org)), f.org, uuid.New())
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	})
}

func TestMemberService_LeaveOrg(t *testing.T) {
	ctx := context.Background()
	leave := func(c memberv1connect.MemberServiceClient, org uuid.UUID) error {
		_, err := c.LeaveOrg(ctx, connect.NewRequest(&memberv1.LeaveOrgRequest{OrgId: org.String()}))
		return err
	}

	t.Run("a member leaves as themselves", func(t *testing.T) {
		f := newCPFixture(t)
		f.add(member.RoleOwner, "owner@example.com")
		mem := f.add(member.RoleMember, "member@example.com")
		require.NoError(t, leave(f.client(personIn(mem, f.org)), f.org))
		_, err := f.svc.Find(ctx, f.org, mem)
		assert.True(t, member.IsNotFoundError(err))
	})
	t.Run("the last owner cannot leave", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		err := leave(f.client(personIn(owner, f.org)), f.org)
		assert.Equal(t, apperror.CodeMemberLastOwner, errCode(t, err))
	})
	t.Run("an operator has no membership to leave", func(t *testing.T) {
		f := newCPFixture(t)
		f.add(member.RoleOwner, "owner@example.com")
		err := leave(f.client(operatorPrincipal()), f.org)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})
}

func staffPerson(userID uuid.UUID) session.Principal {
	return session.Principal{Source: session.SourceLocal, UserID: userID, ActiveOrgID: uuid.New(), Scopes: []string{"*"}, Operator: true}
}

// SECURITY: staff (an operator who is a person) manage like an admin only outside their own orgs, and only where operator support is on.
func TestMemberService_OperatorPerson(t *testing.T) {
	ctx := context.Background()
	set := func(c memberv1connect.MemberServiceClient, org, user uuid.UUID, role string) error {
		_, err := c.SetMemberRole(ctx, connect.NewRequest(&memberv1.SetMemberRoleRequest{OrgId: org.String(), UserId: user.String(), Role: role}))
		return err
	}
	remove := func(c memberv1connect.MemberServiceClient, org, user uuid.UUID) error {
		_, err := c.RemoveMember(ctx, connect.NewRequest(&memberv1.RemoveMemberRequest{OrgId: org.String(), UserId: user.String()}))
		return err
	}

	t.Run("outside their orgs with support on they manage like an admin", func(t *testing.T) {
		f := newCPFixture(t)
		f.svc.WithOperatorSupport(true)
		owner := f.add(member.RoleOwner, "owner@example.com")
		plain := f.add(member.RoleMember, "member@example.com")
		other := f.add(member.RoleMember, "other@example.com")
		c := f.client(staffPerson(uuid.New()))
		require.NoError(t, set(c, f.org, plain, "admin"))
		assert.Equal(t, member.RoleAdmin, f.role(plain))
		require.NoError(t, remove(c, f.org, other))
		assert.Equal(t, apperror.CodeMemberRoleNotAllowed, errCode(t, set(c, f.org, owner, "member")))
		assert.Equal(t, apperror.CodeMemberRoleNotAllowed, errCode(t, set(c, f.org, plain, "owner")))
		assert.Equal(t, apperror.CodeMemberRoleNotAllowed, errCode(t, remove(c, f.org, owner)))
		assert.Equal(t, member.RoleOwner, f.role(owner))
	})
	t.Run("outside their orgs with support off they are refused", func(t *testing.T) {
		f := newCPFixture(t)
		owner := f.add(member.RoleOwner, "owner@example.com")
		target := f.add(member.RoleMember, "member@example.com")
		c := f.client(staffPerson(uuid.New()))
		err := set(c, f.org, target, "admin")
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(remove(c, f.org, owner)))
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
			assert.Equal(t, apperror.CodeMemberSelfChange, errCode(t, set(c, f.org, admin, "member")))
			assert.Equal(t, apperror.CodeMemberRoleNotAllowed, errCode(t, set(c, f.org, owner, "admin")))
			assert.Equal(t, apperror.CodeMemberRoleNotAllowed, errCode(t, remove(c, f.org, owner)))
			assert.Equal(t, apperror.CodeMemberRoleNotAllowed, errCode(t, set(c, f.org, plain, "owner")))
			require.NoError(t, set(c, f.org, plain, "admin"), "an admin still manages members")
			assert.Equal(t, member.RoleOwner, f.role(owner))
			assert.Equal(t, member.RoleAdmin, f.role(admin))
		})
	}
}
