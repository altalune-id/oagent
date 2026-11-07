package channel

import (
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	channelv1 "altalune.id/oagent/gen/go/channel/v1"
	channelv1connect "altalune.id/oagent/gen/go/channel/v1/channelv1connect"
	"altalune.id/oagent/internal/render"
)

// ClientFn returns the Channel Connect client for a command; the caller resolves URL and token.
type ClientFn func(cmd *cobra.Command) (channelv1connect.ChannelServiceClient, error)

// NewCmd builds the `channel` command tree; it only lists, since connections carry secrets the CLI never takes.
func NewCmd(clientFn ClientFn) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "channel",
		Short:   "List messaging channel connections",
		GroupID: "domain",
	}
	cmd.AddCommand(newListCmd(clientFn))
	return cmd
}

func newListCmd(clientFn ClientFn) *cobra.Command {
	var projectID, agentID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List channel connections in a project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			req := &channelv1.ListConnectionsRequest{ProjectId: projectID, AgentId: agentID}
			resp, err := client.ListConnections(cmd.Context(), connect.NewRequest(req))
			if err != nil {
				return err
			}
			conns := resp.Msg.GetConnections()
			if isJSON(cmd) {
				out := make([]map[string]any, 0, len(conns))
				for _, c := range conns {
					out = append(out, connectionMap(c))
				}
				return render.JSON(cmd.OutOrStdout(), out)
			}
			rows := make([][]string, 0, len(conns))
			for _, c := range conns {
				rows = append(rows, []string{
					c.GetId(), c.GetAgentId(), c.GetProvider(), strconv.FormatBool(c.GetEnabled()), statusName(c.GetStatus()),
					secretState(c.GetSecretSet()), c.GetDeviceId(),
				})
			}
			return render.Table(cmd.OutOrStdout(), []string{"ID", "AGENT_ID", "PROVIDER", "ENABLED", "STATUS", "SECRET", "DEVICE_ID"}, rows)
		},
	}
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
	cmd.Flags().StringVar(&agentID, "agent-id", "", "Only this agent's connections (public id)")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func isJSON(cmd *cobra.Command) bool {
	switch render.Detect(cmd) {
	case render.FormatJSON, render.FormatNDJSON:
		return true
	default:
		return false
	}
}

func statusName(s channelv1.ConnectionStatus) string {
	return strings.ToLower(strings.TrimPrefix(s.String(), "CONNECTION_STATUS_"))
}

func secretState(set bool) string {
	if set {
		return "set"
	}
	return "missing"
}

func connectionMap(c *channelv1.Connection) map[string]any {
	return map[string]any{
		"id":        c.GetId(),
		"projectId": c.GetProjectId(),
		"agentId":   c.GetAgentId(),
		"provider":  c.GetProvider(),
		"enabled":   c.GetEnabled(),
		"status":    statusName(c.GetStatus()),
		"secretSet": c.GetSecretSet(),
		"deviceId":  c.GetDeviceId(),
		"baseUrl":   c.GetBaseUrl(),
	}
}
