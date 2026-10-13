package invite

import (
	"fmt"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	invitev1 "altalune.id/oagent/gen/go/invite/v1"
	invitev1connect "altalune.id/oagent/gen/go/invite/v1/invitev1connect"
	"altalune.id/oagent/internal/org/member"
	"altalune.id/oagent/internal/prompt"
	"altalune.id/oagent/internal/render"
)

// ClientFn returns the Invite Connect client for a command; the caller resolves URL and token.
type ClientFn func(cmd *cobra.Command) (invitev1connect.InviteServiceClient, error)

const (
	orgFlagHelp    = "Organization public id"
	inviteFlagHelp = "Invite public id"
)

// NewCmd builds the `invite` command tree.
func NewCmd(clientFn ClientFn) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "invite",
		Short:   "Invite people into an organization",
		GroupID: "tenancy",
	}
	cmd.AddCommand(newCreateCmd(clientFn), newListCmd(clientFn), newResendCmd(clientFn), newRevokeCmd(clientFn))
	return cmd
}

func newCreateCmd(clientFn ClientFn) *cobra.Command {
	var orgID, email, role string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Invite a person by email and print the link",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			req := &invitev1.CreateInviteRequest{OrgId: orgID, Email: email, Role: role}
			resp, err := client.CreateInvite(cmd.Context(), connect.NewRequest(req))
			if err != nil {
				return err
			}
			return printSent(cmd, resp.Msg.GetInvite(), resp.Msg.GetLink())
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", orgFlagHelp)
	cmd.Flags().StringVar(&email, "email", "", "The person's email")
	cmd.Flags().StringVar(&role, "role", string(member.RoleMember), "Role: owner|admin|member")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

func newListCmd(clientFn ClientFn) *cobra.Command {
	var orgID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pending invites and drafts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.ListInvites(cmd.Context(), connect.NewRequest(&invitev1.ListInvitesRequest{OrgId: orgID}))
			if err != nil {
				return err
			}
			all := slices.Concat(resp.Msg.GetInvites(), resp.Msg.GetDrafts())
			if isJSON(cmd) {
				out := make([]map[string]any, 0, len(all))
				for _, i := range all {
					out = append(out, inviteMap(i, ""))
				}
				return render.JSON(cmd.OutOrStdout(), out)
			}
			rows := make([][]string, 0, len(all))
			for _, i := range all {
				rows = append(rows, []string{
					i.GetId(), i.GetEmail(), i.GetRole(), i.GetStatus(), i.GetInvitedByName(),
					i.GetExpiresAt().AsTime().UTC().Format(time.RFC3339),
				})
			}
			return render.Table(cmd.OutOrStdout(), []string{"ID", "EMAIL", "ROLE", "STATUS", "INVITED BY", "EXPIRES"}, rows)
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", orgFlagHelp)
	_ = cmd.MarkFlagRequired("org-id")
	return cmd
}

func newResendCmd(clientFn ClientFn) *cobra.Command {
	var orgID, id string
	cmd := &cobra.Command{
		Use:   "resend",
		Short: "Send an invite again with a new link",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.ResendInvite(cmd.Context(), connect.NewRequest(&invitev1.ResendInviteRequest{OrgId: orgID, InviteId: id}))
			if err != nil {
				return err
			}
			return printSent(cmd, resp.Msg.GetInvite(), resp.Msg.GetLink())
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", orgFlagHelp)
	cmd.Flags().StringVar(&id, "id", "", inviteFlagHelp)
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func newRevokeCmd(clientFn ClientFn) *cobra.Command {
	var orgID, id string
	var yes bool
	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Cancel an invite",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			if !yes {
				if err := prompt.YesNo(cmd, fmt.Sprintf("Cancel invite %s? [y/N] ", id)); err != nil {
					return err
				}
			}
			req := &invitev1.RevokeInviteRequest{OrgId: orgID, InviteId: id}
			if _, err := client.RevokeInvite(cmd.Context(), connect.NewRequest(req)); err != nil {
				return err
			}
			cmd.Printf("Invite %s cancelled\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", orgFlagHelp)
	cmd.Flags().StringVar(&id, "id", "", inviteFlagHelp)
	cmd.Flags().BoolVar(&yes, "yes", false, "Do not ask for confirmation")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func printSent(cmd *cobra.Command, i *invitev1.Invite, link string) error {
	if isJSON(cmd) {
		return render.JSON(cmd.OutOrStdout(), inviteMap(i, link))
	}
	cmd.Printf("Invite sent to %s. Share this link if the email doesn't arrive: %s\n", i.GetEmail(), link)
	return nil
}

func isJSON(cmd *cobra.Command) bool {
	switch render.Detect(cmd) {
	case render.FormatJSON, render.FormatNDJSON:
		return true
	default:
		return false
	}
}

func inviteMap(i *invitev1.Invite, link string) map[string]any {
	out := map[string]any{
		"id":            i.GetId(),
		"email":         i.GetEmail(),
		"role":          i.GetRole(),
		"status":        i.GetStatus(),
		"invitedByName": i.GetInvitedByName(),
		"expiresAt":     i.GetExpiresAt().AsTime().UTC().Format(time.RFC3339),
		"createdAt":     i.GetCreatedAt().AsTime().UTC().Format(time.RFC3339),
	}
	if link != "" {
		out["link"] = link
	}
	return out
}
