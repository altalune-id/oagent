package member_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	memberv1 "altalune.id/oagent/gen/go/member/v1"
	memberv1connect "altalune.id/oagent/gen/go/member/v1/memberv1connect"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/prompt"
)

const (
	cliOrg  = "8c7a5b52-2e0e-4a52-9f43-0f1f6c0b8f11"
	cliUser = "1b0e4b0c-7a77-4f0e-9a43-5d8b3e2c9d22"
)

type fakeMemberClient struct {
	members  []*memberv1.Member
	err      error
	listReqs []*memberv1.ListMembersRequest
	setReqs  []*memberv1.SetMemberRoleRequest
	rmReqs   []*memberv1.RemoveMemberRequest
	lvReqs   []*memberv1.LeaveOrgRequest
}

var _ memberv1connect.MemberServiceClient = (*fakeMemberClient)(nil)

func (f *fakeMemberClient) ListMembers(_ context.Context, req *connect.Request[memberv1.ListMembersRequest]) (*connect.Response[memberv1.ListMembersResponse], error) {
	f.listReqs = append(f.listReqs, req.Msg)
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&memberv1.ListMembersResponse{Members: f.members}), nil
}

func (f *fakeMemberClient) SetMemberRole(_ context.Context, req *connect.Request[memberv1.SetMemberRoleRequest]) (*connect.Response[memberv1.SetMemberRoleResponse], error) {
	f.setReqs = append(f.setReqs, req.Msg)
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&memberv1.SetMemberRoleResponse{Member: &memberv1.Member{
		UserId: req.Msg.GetUserId(), Email: "ada@example.com", Name: "Ada", Role: req.Msg.GetRole(), JoinedAt: timestamppb.Now(),
	}}), nil
}

func (f *fakeMemberClient) RemoveMember(_ context.Context, req *connect.Request[memberv1.RemoveMemberRequest]) (*connect.Response[memberv1.RemoveMemberResponse], error) {
	f.rmReqs = append(f.rmReqs, req.Msg)
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&memberv1.RemoveMemberResponse{}), nil
}

func (f *fakeMemberClient) LeaveOrg(_ context.Context, req *connect.Request[memberv1.LeaveOrgRequest]) (*connect.Response[memberv1.LeaveOrgResponse], error) {
	f.lvReqs = append(f.lvReqs, req.Msg)
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&memberv1.LeaveOrgResponse{}), nil
}

func ada() *memberv1.Member {
	return &memberv1.Member{
		UserId: cliUser, Email: "ada@example.com", Name: "Ada", Role: "admin",
		JoinedAt: timestamppb.New(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)),
	}
}

func runMember(t *testing.T, client memberv1connect.MemberServiceClient, stdin io.Reader, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	root := &cobra.Command{Use: "x", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("output", "", "")
	root.AddGroup(&cobra.Group{ID: "tenancy", Title: "Tenancy:"})
	root.AddCommand(member.NewCmd(func(*cobra.Command) (memberv1connect.MemberServiceClient, error) { return client, nil }))
	root.SetOut(buf)
	root.SetErr(buf)
	if stdin != nil {
		root.SetIn(stdin)
	}
	root.SetArgs(append([]string{"member"}, args...))
	return buf, root.ExecuteContext(t.Context())
}

func TestMemberList_Text(t *testing.T) {
	c := &fakeMemberClient{members: []*memberv1.Member{ada()}}
	buf, err := runMember(t, c, nil, "list", "--org-id", cliOrg, "--output", "text")

	require.NoError(t, err)
	require.Len(t, c.listReqs, 1)
	assert.Equal(t, cliOrg, c.listReqs[0].GetOrgId())
	out := buf.String()
	for _, want := range []string{"USER ID", "NAME", "EMAIL", "ROLE", cliUser, "Ada", "ada@example.com", "admin"} {
		assert.Contains(t, out, want)
	}
}

func TestMemberList_JSON(t *testing.T) {
	buf, err := runMember(t, &fakeMemberClient{members: []*memberv1.Member{ada()}}, nil, "list", "--org-id", cliOrg, "--output", "json")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"email": "ada@example.com"`)
	assert.Contains(t, buf.String(), `"role": "admin"`)
	assert.Contains(t, buf.String(), `"joinedAt": "2026-10-01T00:00:00Z"`)
}

func TestMemberList_Error(t *testing.T) {
	_, err := runMember(t, &fakeMemberClient{err: connect.NewError(connect.CodeNotFound, errors.New("nope"))}, nil,
		"list", "--org-id", cliOrg)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestMemberRole(t *testing.T) {
	c := &fakeMemberClient{}
	buf, err := runMember(t, c, nil, "role", "--org-id", cliOrg, "--user-id", cliUser, "--role", "owner", "--output", "text")

	require.NoError(t, err)
	require.Len(t, c.setReqs, 1)
	assert.Equal(t, &memberv1.SetMemberRoleRequest{OrgId: cliOrg, UserId: cliUser, Role: "owner"}, c.setReqs[0])
	assert.Contains(t, buf.String(), "ada@example.com is now owner")
}

func TestMemberRole_JSON(t *testing.T) {
	buf, err := runMember(t, &fakeMemberClient{}, nil, "role", "--org-id", cliOrg, "--user-id", cliUser, "--role", "admin", "--output", "json")

	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"role": "admin"`)
}

func TestMemberRole_RequiresFlags(t *testing.T) {
	c := &fakeMemberClient{}
	_, err := runMember(t, c, nil, "role", "--org-id", cliOrg, "--user-id", cliUser)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "role")
	assert.Empty(t, c.setReqs)
}

func TestMemberRole_Error(t *testing.T) {
	_, err := runMember(t, &fakeMemberClient{err: connect.NewError(connect.CodePermissionDenied, errors.New("no"))}, nil,
		"role", "--org-id", cliOrg, "--user-id", cliUser, "--role", "admin")
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}

func TestMemberRemove_AsksAndNamesThePerson(t *testing.T) {
	for _, answer := range []string{"y\n", "yes\n", "Y", " YES \r\n"} {
		c := &fakeMemberClient{members: []*memberv1.Member{ada()}}
		buf, err := runMember(t, c, strings.NewReader(answer), "remove", "--org-id", cliOrg, "--user-id", cliUser)

		require.NoError(t, err, answer)
		assert.Contains(t, buf.String(), "Remove ada@example.com from the org? [y/N]", answer)
		require.Len(t, c.rmReqs, 1, answer)
		assert.Equal(t, &memberv1.RemoveMemberRequest{OrgId: cliOrg, UserId: cliUser}, c.rmReqs[0])
		assert.Contains(t, buf.String(), "Removed ada@example.com", answer)
	}
}

func TestMemberRemove_AnythingElseCancels(t *testing.T) {
	for _, answer := range []string{"\n", "n\n", "no\n", "sure\n", ""} {
		c := &fakeMemberClient{members: []*memberv1.Member{ada()}}
		buf, err := runMember(t, c, strings.NewReader(answer), "remove", "--org-id", cliOrg, "--user-id", cliUser)

		assert.ErrorIs(t, err, prompt.ErrCancelled, answer)
		assert.Empty(t, c.rmReqs, answer)
		assert.NotContains(t, buf.String(), "Removed", answer)
	}
}

func TestMemberRemove_YesSkipsTheQuestion(t *testing.T) {
	c := &fakeMemberClient{}
	buf, err := runMember(t, c, strings.NewReader(""), "remove", "--org-id", cliOrg, "--user-id", cliUser, "--yes")

	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "[y/N]")
	assert.Empty(t, c.listReqs)
	require.Len(t, c.rmReqs, 1)
	assert.Contains(t, buf.String(), "Removed "+cliUser)
}

func TestMemberRemove_UnknownPersonIsNamedByID(t *testing.T) {
	c := &fakeMemberClient{}
	buf, err := runMember(t, c, strings.NewReader("y\n"), "remove", "--org-id", cliOrg, "--user-id", cliUser)

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Remove "+cliUser+" from the org? [y/N]")
}

func TestMemberRemove_Errors(t *testing.T) {
	denied := connect.NewError(connect.CodePermissionDenied, errors.New("no"))
	_, err := runMember(t, &fakeMemberClient{err: denied}, nil, "remove", "--org-id", cliOrg, "--user-id", cliUser, "--yes")
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))

	_, err = runMember(t, &fakeMemberClient{err: denied}, strings.NewReader("y\n"), "remove", "--org-id", cliOrg, "--user-id", cliUser)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "the lookup fails")

	_, err = runMember(t, &fakeMemberClient{}, errReader{}, "remove", "--org-id", cliOrg, "--user-id", cliUser)
	assert.ErrorIs(t, err, errRead)
}

func TestMemberLeave(t *testing.T) {
	c := &fakeMemberClient{}
	buf, err := runMember(t, c, strings.NewReader("y\n"), "leave", "--org-id", cliOrg)

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Leave organization "+cliOrg+"? [y/N]")
	require.Len(t, c.lvReqs, 1)
	assert.Equal(t, cliOrg, c.lvReqs[0].GetOrgId())
	assert.Contains(t, buf.String(), "You left organization "+cliOrg)
}

func TestMemberLeave_CancelYesAndError(t *testing.T) {
	c := &fakeMemberClient{}
	_, err := runMember(t, c, strings.NewReader("n\n"), "leave", "--org-id", cliOrg)
	assert.ErrorIs(t, err, prompt.ErrCancelled)
	assert.Empty(t, c.lvReqs)

	buf, err := runMember(t, c, strings.NewReader(""), "leave", "--org-id", cliOrg, "--yes")
	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "[y/N]")
	assert.Len(t, c.lvReqs, 1)

	_, err = runMember(t, &fakeMemberClient{err: connect.NewError(connect.CodeFailedPrecondition, errors.New("last owner"))}, nil,
		"leave", "--org-id", cliOrg, "--yes")
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	_, err = runMember(t, &fakeMemberClient{}, errReader{}, "leave", "--org-id", cliOrg)
	assert.ErrorIs(t, err, errRead)
}

func TestMemberCmd_ClientError(t *testing.T) {
	root := &cobra.Command{Use: "x", SilenceUsage: true, SilenceErrors: true}
	root.AddGroup(&cobra.Group{ID: "tenancy", Title: "Tenancy:"})
	boom := errors.New("no client")
	root.AddCommand(member.NewCmd(func(*cobra.Command) (memberv1connect.MemberServiceClient, error) { return nil, boom }))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	for _, args := range [][]string{
		{"list", "--org-id", cliOrg},
		{"role", "--org-id", cliOrg, "--user-id", cliUser, "--role", "admin"},
		{"remove", "--org-id", cliOrg, "--user-id", cliUser, "--yes"},
		{"leave", "--org-id", cliOrg, "--yes"},
	} {
		root.SetArgs(append([]string{"member"}, args...))
		assert.ErrorIs(t, root.ExecuteContext(t.Context()), boom, args[0])
	}
}

var errRead = errors.New("read failed")

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errRead }

var addOrgUUID, addUserUUID = uuid.New(), uuid.New()

type addCall struct {
	email, org string
	role       member.Role
}

func runMemberAdd(t *testing.T, err error, args ...string) (*bytes.Buffer, []addCall, error) {
	t.Helper()
	var calls []addCall
	add := func(_ context.Context, email, orgPublicID string, role member.Role) (*member.Member, error) {
		calls = append(calls, addCall{email, orgPublicID, role})
		if err != nil {
			return nil, err
		}
		return &member.Member{
			Membership:   member.Membership{OrgID: addOrgUUID, UserID: addUserUUID, Role: role, CreatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
			UserPublicID: "u5tQ8hJ2mK9pR3wX7yZaB",
		}, nil
	}
	buf := new(bytes.Buffer)
	root := &cobra.Command{Use: "x", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("output", "", "")
	root.AddGroup(&cobra.Group{ID: "tenancy", Title: "Tenancy:"})
	memberCmd := member.NewCmd(func(*cobra.Command) (memberv1connect.MemberServiceClient, error) {
		return nil, errors.New("member add must not open a remote client")
	})
	memberCmd.AddCommand(member.NewAddCmd(add))
	root.AddCommand(memberCmd)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(append([]string{"member", "add"}, args...))
	return buf, calls, root.ExecuteContext(t.Context())
}

func TestMemberAdd_DefaultRoleIsMember(t *testing.T) {
	buf, calls, err := runMemberAdd(t, nil, "--email", "ada@example.com", "--org-id", "V1StGXR8Z5jdHi6BmyT2a", "--output", "text")

	require.NoError(t, err)
	assert.Equal(t, []addCall{{"ada@example.com", "V1StGXR8Z5jdHi6BmyT2a", member.RoleMember}}, calls)
	assert.Equal(t, "Added ada@example.com to V1StGXR8Z5jdHi6BmyT2a as member\n", buf.String())
}

func TestMemberAdd_Role(t *testing.T) {
	buf, calls, err := runMemberAdd(t, nil, "--email", "ada@example.com", "--org-id", "V1StGXR8Z5jdHi6BmyT2a", "--role", "admin", "--output", "text")

	require.NoError(t, err)
	assert.Equal(t, []addCall{{"ada@example.com", "V1StGXR8Z5jdHi6BmyT2a", member.RoleAdmin}}, calls)
	assert.Contains(t, buf.String(), "Added ada@example.com to V1StGXR8Z5jdHi6BmyT2a as admin")
}

func TestMemberAdd_JSON(t *testing.T) {
	buf, _, err := runMemberAdd(t, nil, "--email", "ada@example.com", "--org-id", "V1StGXR8Z5jdHi6BmyT2a", "--output", "json")

	require.NoError(t, err)
	for _, want := range []string{
		`"email": "ada@example.com"`, `"orgId": "V1StGXR8Z5jdHi6BmyT2a"`, `"userId": "u5tQ8hJ2mK9pR3wX7yZaB"`,
		`"role": "member"`, `"joinedAt": "2026-10-01T00:00:00Z"`,
	} {
		assert.Contains(t, buf.String(), want)
	}
	assert.NotContains(t, buf.String(), addOrgUUID.String())
	assert.NotContains(t, buf.String(), addUserUUID.String())
}

func TestMemberAdd_RequiresEmailAndOrg(t *testing.T) {
	_, calls, err := runMemberAdd(t, nil, "--email", "ada@example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "org")
	assert.Empty(t, calls)

	_, calls, err = runMemberAdd(t, nil, "--org-id", "V1StGXR8Z5jdHi6BmyT2a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "email")
	assert.Empty(t, calls)
}

func TestMemberAdd_Error(t *testing.T) {
	buf, _, err := runMemberAdd(t, &member.AlreadyMemberError{}, "--email", "ada@example.com", "--org-id", "V1StGXR8Z5jdHi6BmyT2a")

	assert.True(t, member.IsAlreadyMemberError(err))
	assert.NotContains(t, buf.String(), "Added")
}
