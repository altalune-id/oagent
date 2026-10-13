package member

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	memberv1 "altalune.id/oagent/gen/go/member/v1"
	memberv1connect "altalune.id/oagent/gen/go/member/v1/memberv1connect"
	"altalune.id/oagent/internal/prompt"
	"altalune.id/oagent/internal/render"
)

// ClientFn returns the Member Connect client for a command; the caller resolves URL and token.
type ClientFn func(cmd *cobra.Command) (memberv1connect.MemberServiceClient, error)

// AddFn adds the existing user with email to the org orgPublicID as the operator; the caller boots the server graph.
type AddFn func(ctx context.Context, email, orgPublicID string, role Role) (*Member, error)

// NewCmd builds the `member` command tree.
func NewCmd(clientFn ClientFn) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "member",
		Short:   "Manage organization members",
		GroupID: "tenancy",
	}
	cmd.AddCommand(
		newListCmd(clientFn),
		newRoleCmd(clientFn),
		newRemoveCmd(clientFn),
		newLeaveCmd(clientFn),
	)
	return cmd
}

// NewAddCmd builds `member add`, the operator command that needs database access, not a remote server.
func NewAddCmd(add AddFn) *cobra.Command {
	var email, orgID, role string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add an existing user to an organization (operator, needs database access)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, err := add(cmd.Context(), email, orgID, Role(role))
			if err != nil {
				return err
			}
			if isJSON(cmd) {
				return render.JSON(cmd.OutOrStdout(), map[string]any{
					"email":    email,
					"orgId":    orgID,
					"userId":   m.UserPublicID,
					"role":     string(m.Role),
					"joinedAt": m.CreatedAt.UTC().Format(time.RFC3339),
				})
			}
			cmd.Printf("Added %s to %s as %s\n", email, orgID, m.Role)
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "The user's email")
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&role, "role", string(RoleMember), "Role: owner|admin|member")
	_ = cmd.MarkFlagRequired("email")
	_ = cmd.MarkFlagRequired("org-id")
	return cmd
}

func newListCmd(clientFn ClientFn) *cobra.Command {
	var orgID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List an organization's members",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.ListMembers(cmd.Context(), connect.NewRequest(&memberv1.ListMembersRequest{OrgId: orgID}))
			if err != nil {
				return err
			}
			ms := resp.Msg.GetMembers()
			if isJSON(cmd) {
				out := make([]map[string]any, 0, len(ms))
				for _, m := range ms {
					out = append(out, memberMap(m))
				}
				return render.JSON(cmd.OutOrStdout(), out)
			}
			rows := make([][]string, 0, len(ms))
			for _, m := range ms {
				rows = append(rows, []string{m.GetUserId(), m.GetName(), m.GetEmail(), m.GetRole()})
			}
			return render.Table(cmd.OutOrStdout(), []string{"USER ID", "NAME", "EMAIL", "ROLE"}, rows)
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	_ = cmd.MarkFlagRequired("org-id")
	return cmd
}

func newRoleCmd(clientFn ClientFn) *cobra.Command {
	var orgID, userID, role string
	cmd := &cobra.Command{
		Use:   "role",
		Short: "Change a member's role",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.SetMemberRole(cmd.Context(), connect.NewRequest(&memberv1.SetMemberRoleRequest{
				OrgId: orgID, UserId: userID, Role: role,
			}))
			if err != nil {
				return err
			}
			m := resp.Msg.GetMember()
			if isJSON(cmd) {
				return render.JSON(cmd.OutOrStdout(), memberMap(m))
			}
			cmd.Printf("%s is now %s\n", label(m), m.GetRole())
			return nil
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&userID, "user-id", "", "The member's user public id")
	cmd.Flags().StringVar(&role, "role", "", "New role: owner|admin|member")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("user-id")
	_ = cmd.MarkFlagRequired("role")
	return cmd
}

func newRemoveCmd(clientFn ClientFn) *cobra.Command {
	var orgID, userID string
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a member from an organization",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			who := userID
			if !yes {
				if who, err = memberLabel(cmd, client, orgID, userID); err != nil {
					return err
				}
				if err := prompt.YesNo(cmd, fmt.Sprintf("Remove %s from the org? [y/N] ", who)); err != nil {
					return err
				}
			}
			_, err = client.RemoveMember(cmd.Context(), connect.NewRequest(&memberv1.RemoveMemberRequest{OrgId: orgID, UserId: userID}))
			if err != nil {
				return err
			}
			cmd.Printf("Removed %s\n", who)
			return nil
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&userID, "user-id", "", "The member's user public id")
	cmd.Flags().BoolVar(&yes, "yes", false, "Do not ask for confirmation")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("user-id")
	return cmd
}

func newLeaveCmd(clientFn ClientFn) *cobra.Command {
	var orgID string
	var yes bool
	cmd := &cobra.Command{
		Use:   "leave",
		Short: "Leave an organization",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			if !yes {
				if err := prompt.YesNo(cmd, fmt.Sprintf("Leave organization %s? [y/N] ", orgID)); err != nil {
					return err
				}
			}
			if _, err := client.LeaveOrg(cmd.Context(), connect.NewRequest(&memberv1.LeaveOrgRequest{OrgId: orgID})); err != nil {
				return err
			}
			cmd.Printf("You left organization %s\n", orgID)
			return nil
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().BoolVar(&yes, "yes", false, "Do not ask for confirmation")
	_ = cmd.MarkFlagRequired("org-id")
	return cmd
}

func memberLabel(cmd *cobra.Command, client memberv1connect.MemberServiceClient, orgID, userID string) (string, error) {
	resp, err := client.ListMembers(cmd.Context(), connect.NewRequest(&memberv1.ListMembersRequest{OrgId: orgID}))
	if err != nil {
		return "", err
	}
	for _, m := range resp.Msg.GetMembers() {
		if m.GetUserId() == userID {
			return label(m), nil
		}
	}
	return userID, nil
}

func label(m *memberv1.Member) string {
	if m.GetEmail() != "" {
		return m.GetEmail()
	}
	return m.GetUserId()
}

func isJSON(cmd *cobra.Command) bool {
	switch render.Detect(cmd) {
	case render.FormatJSON, render.FormatNDJSON:
		return true
	default:
		return false
	}
}

func memberMap(m *memberv1.Member) map[string]any {
	return map[string]any{
		"userId":   m.GetUserId(),
		"email":    m.GetEmail(),
		"name":     m.GetName(),
		"role":     m.GetRole(),
		"joinedAt": m.GetJoinedAt().AsTime().UTC().Format(time.RFC3339),
	}
}
