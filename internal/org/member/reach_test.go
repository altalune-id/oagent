package member_test

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	memberv1 "altalune.id/oagent/gen/go/member/v1"
	"altalune.id/oagent/internal/kernel/session"
	"altalune.id/oagent/internal/org/member"
)

func TestMemberService_OutOfReachIsNotFound(t *testing.T) {
	f := newCPFixture(t)
	owner := f.add(member.RoleOwner, "owner@example.com")
	target := f.add(member.RoleMember, "member@example.com")
	ctx := context.Background()

	for name, p := range map[string]session.Principal{
		"person of another org": personIn(owner, uuid.New()),
		"key of another org":    keyIn(uuid.New()),
		"no principal":          {},
	} {
		t.Run(name, func(t *testing.T) {
			c := f.client(p)
			_, err := c.ListMembers(ctx, connect.NewRequest(&memberv1.ListMembersRequest{OrgId: f.org.String()}))
			assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "list")
			_, err = c.SetMemberRole(ctx, connect.NewRequest(&memberv1.SetMemberRoleRequest{OrgId: f.org.String(), UserId: target.String(), Role: "admin"}))
			assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "set")
			_, err = c.RemoveMember(ctx, connect.NewRequest(&memberv1.RemoveMemberRequest{OrgId: f.org.String(), UserId: target.String()}))
			assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "remove")
			_, err = c.LeaveOrg(ctx, connect.NewRequest(&memberv1.LeaveOrgRequest{OrgId: f.org.String()}))
			assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "leave")
		})
	}
	assert.Equal(t, member.RoleMember, f.role(target))
	assert.Equal(t, member.RoleOwner, f.role(owner))
}
