package approval

import (
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	approvalv1 "altalune.id/oagent/gen/go/approval/v1"
	approvalv1connect "altalune.id/oagent/gen/go/approval/v1/approvalv1connect"
	"altalune.id/oagent/internal/render"
)

// ClientFn returns the Approval Connect client for a command; the caller resolves URL and token.
type ClientFn func(cmd *cobra.Command) (approvalv1connect.ApprovalServiceClient, error)

// NewCmd builds the `approval` command tree; it only lists, and decisions stay with the console, WhatsApp, the API and MCP.
func NewCmd(clientFn ClientFn) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "approval",
		Short:   "List the tool calls waiting on a person's approval",
		GroupID: "domain",
	}
	cmd.AddCommand(newListCmd(clientFn))
	return cmd
}

func newListCmd(clientFn ClientFn) *cobra.Command {
	var projectID, agentID, status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List approvals in a project, newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			req := &approvalv1.ListApprovalsRequest{ProjectId: projectID, AgentId: agentID, Status: status}
			resp, err := client.ListApprovals(cmd.Context(), connect.NewRequest(req))
			if err != nil {
				return err
			}
			list := resp.Msg.GetApprovals()
			switch render.Detect(cmd) {
			case render.FormatJSON, render.FormatNDJSON:
				out := make([]map[string]any, 0, len(list))
				for _, a := range list {
					out = append(out, approvalMap(a))
				}
				return render.JSON(cmd.OutOrStdout(), out)
			}
			rows := make([][]string, 0, len(list))
			for _, a := range list {
				rows = append(rows, []string{
					a.GetId(), a.GetAgentId(), a.GetStatus(), a.GetChannel(), strings.Join(a.GetTools(), ","), a.GetRequesterLabel(),
					a.GetExpiresAt().AsTime().UTC().Format(time.RFC3339),
				})
			}
			return render.Table(cmd.OutOrStdout(), []string{"ID", "AGENT_ID", "STATUS", "CHANNEL", "TOOLS", "REQUESTER", "EXPIRES_AT"}, rows)
		},
	}
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
	cmd.Flags().StringVar(&agentID, "agent-id", "", "Only this agent's approvals (public id)")
	cmd.Flags().StringVar(&status, "status", "", "Only this status: pending, approved, rejected or expired")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func approvalMap(a *approvalv1.Approval) map[string]any {
	return map[string]any{
		"id":             a.GetId(),
		"projectId":      a.GetProjectId(),
		"agentId":        a.GetAgentId(),
		"threadId":       a.GetThreadId(),
		"runId":          a.GetRunId(),
		"status":         a.GetStatus(),
		"channel":        a.GetChannel(),
		"tools":          a.GetTools(),
		"preview":        a.GetPreview(),
		"requesterLabel": a.GetRequesterLabel(),
		"decidedByLabel": a.GetDecidedByLabel(),
		"expiresAt":      a.GetExpiresAt().AsTime().UTC().Format(time.RFC3339),
	}
}
