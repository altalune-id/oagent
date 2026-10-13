package agent

import (
	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	agentv1connect "altalune.id/oagent/gen/go/agent/v1/agentv1connect"
	"altalune.id/oagent/internal/render"
)

// ClientFn returns the Agent Connect client for a command; the caller resolves URL and token.
type ClientFn func(cmd *cobra.Command) (agentv1connect.AgentServiceClient, error)

// NewCmd builds the `agent` command tree.
func NewCmd(clientFn ClientFn) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "agent",
		Short:   "Manage agents",
		GroupID: "domain",
	}
	cmd.AddCommand(
		newCreateCmd(clientFn),
		newGetCmd(clientFn),
		newListCmd(clientFn),
		newUpdateCmd(clientFn),
		newDeleteCmd(clientFn),
	)
	return cmd
}

func newCreateCmd(clientFn ClientFn) *cobra.Command {
	var orgID, projectID, slug, name, description string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an agent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.CreateAgent(cmd.Context(), connect.NewRequest(&agentv1.CreateAgentRequest{
				OrgId: orgID, ProjectId: projectID, Slug: slug, Name: name, Description: description,
			}))
			if err != nil {
				return err
			}
			a := resp.Msg.GetAgent()
			if isJSON(cmd) {
				return render.JSON(cmd.OutOrStdout(), agentMap(a))
			}
			cmd.Printf("Created agent %s (%s)\n", a.GetSlug(), a.GetId())
			return nil
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
	cmd.Flags().StringVar(&slug, "slug", "", "URL-safe agent slug (generated when omitted)")
	cmd.Flags().StringVar(&name, "name", "", "Agent name")
	cmd.Flags().StringVar(&description, "description", "", "Agent description")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("project-id")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newGetCmd(clientFn ClientFn) *cobra.Command {
	return &cobra.Command{
		Use:   "get <public-id>",
		Short: "Get an agent by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.GetAgent(cmd.Context(), connect.NewRequest(&agentv1.GetAgentRequest{Id: args[0]}))
			if err != nil {
				return err
			}
			a := resp.Msg.GetAgent()
			if isJSON(cmd) {
				return render.JSON(cmd.OutOrStdout(), agentMap(a))
			}
			return render.Table(cmd.OutOrStdout(), []string{"ID", "PROJECT_ID", "SLUG", "NAME"},
				[][]string{{a.GetId(), a.GetProjectId(), a.GetSlug(), a.GetName()}})
		},
	}
}

func newListCmd(clientFn ClientFn) *cobra.Command {
	var projectID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agents in a project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.ListAgents(cmd.Context(), connect.NewRequest(&agentv1.ListAgentsRequest{ProjectId: projectID}))
			if err != nil {
				return err
			}
			agents := resp.Msg.GetAgents()
			if isJSON(cmd) {
				out := make([]map[string]any, 0, len(agents))
				for _, a := range agents {
					out = append(out, agentMap(a))
				}
				return render.JSON(cmd.OutOrStdout(), out)
			}
			rows := make([][]string, 0, len(agents))
			for _, a := range agents {
				rows = append(rows, []string{a.GetId(), a.GetProjectId(), a.GetSlug(), a.GetName()})
			}
			return render.Table(cmd.OutOrStdout(), []string{"ID", "PROJECT_ID", "SLUG", "NAME"}, rows)
		},
	}
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func newUpdateCmd(clientFn ClientFn) *cobra.Command {
	var name, description string
	cmd := &cobra.Command{
		Use:   "update <public-id>",
		Short: "Update an agent's name and description",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.UpdateAgent(
				cmd.Context(),
				connect.NewRequest(&agentv1.UpdateAgentRequest{Id: args[0], Name: name, Description: description}),
			)
			if err != nil {
				return err
			}
			a := resp.Msg.GetAgent()
			if isJSON(cmd) {
				return render.JSON(cmd.OutOrStdout(), agentMap(a))
			}
			cmd.Printf("Updated agent %s (%s)\n", a.GetSlug(), a.GetId())
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Agent name")
	cmd.Flags().StringVar(&description, "description", "", "Agent description")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newDeleteCmd(clientFn ClientFn) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <public-id>",
		Short: "Delete an agent by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			if _, err := client.DeleteAgent(cmd.Context(), connect.NewRequest(&agentv1.DeleteAgentRequest{Id: args[0]})); err != nil {
				return err
			}
			cmd.Printf("Deleted agent %s\n", args[0])
			return nil
		},
	}
}

func isJSON(cmd *cobra.Command) bool {
	switch render.Detect(cmd) {
	case render.FormatJSON, render.FormatNDJSON:
		return true
	default:
		return false
	}
}

func agentMap(a *agentv1.Agent) map[string]any {
	return map[string]any{
		"id":          a.GetId(),
		"orgId":       a.GetOrgId(),
		"projectId":   a.GetProjectId(),
		"slug":        a.GetSlug(),
		"name":        a.GetName(),
		"description": a.GetDescription(),
	}
}
