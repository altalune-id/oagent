package webhook

import (
	"strings"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	webhookv1 "altalune.id/oagent/gen/go/webhook/v1"
	webhookv1connect "altalune.id/oagent/gen/go/webhook/v1/webhookv1connect"
	"altalune.id/oagent/internal/render"
)

// ClientFn returns the Webhook Connect client for a command; the caller resolves URL and token.
type ClientFn func(cmd *cobra.Command) (webhookv1connect.WebhookServiceClient, error)

// NewCmd builds the `webhook` command tree.
func NewCmd(clientFn ClientFn) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "webhook",
		Short:   "Manage outbound webhook endpoints",
		GroupID: "domain",
	}
	cmd.AddCommand(
		newCreateCmd(clientFn),
		newGetCmd(clientFn),
		newListCmd(clientFn),
		newUpdateCmd(clientFn),
		newDeleteCmd(clientFn),
		newRotateCmd(clientFn),
		newPingCmd(clientFn),
	)
	return cmd
}

func newCreateCmd(clientFn ClientFn) *cobra.Command {
	var orgID, projectID, endpointURL, description string
	var types []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a webhook endpoint",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.CreateWebhook(cmd.Context(), connect.NewRequest(&webhookv1.CreateWebhookRequest{
				OrgId:       orgID,
				ProjectId:   projectID,
				Url:         endpointURL,
				Description: description,
				EventTypes:  types,
			}))
			if err != nil {
				return err
			}
			if isJSON(cmd) {
				return render.JSON(cmd.OutOrStdout(), map[string]any{
					"webhook": webhookMap(resp.Msg.GetWebhook()),
					"secret":  resp.Msg.GetSecret(),
				})
			}
			cmd.Printf("Created webhook %s\nSecret (shown once): %s\n", resp.Msg.GetWebhook().GetId(), resp.Msg.GetSecret())
			return nil
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project id")
	cmd.Flags().StringVar(&endpointURL, "endpoint-url", "", "HTTPS endpoint URL")
	cmd.Flags().StringVar(&description, "description", "", "Description")
	cmd.Flags().StringSliceVar(&types, "event", nil, "Event type (repeatable), e.g. agent.created")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("project-id")
	_ = cmd.MarkFlagRequired("url")
	_ = cmd.MarkFlagRequired("event")
	return cmd
}

func newGetCmd(clientFn ClientFn) *cobra.Command {
	var orgID, projectID string
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a webhook endpoint by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			req := &webhookv1.GetWebhookRequest{OrgId: orgID, ProjectId: projectID, Id: args[0]}
			resp, err := client.GetWebhook(cmd.Context(), connect.NewRequest(req))
			if err != nil {
				return err
			}
			e := resp.Msg.GetWebhook()
			if isJSON(cmd) {
				return render.JSON(cmd.OutOrStdout(), webhookMap(e))
			}
			return render.Table(cmd.OutOrStdout(), webhookColumns(), [][]string{webhookRow(e)})
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project id")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func newListCmd(clientFn ClientFn) *cobra.Command {
	var orgID, projectID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List a project's webhook endpoints",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.ListWebhooks(cmd.Context(), connect.NewRequest(&webhookv1.ListWebhooksRequest{OrgId: orgID, ProjectId: projectID}))
			if err != nil {
				return err
			}
			hooks := resp.Msg.GetWebhooks()
			if isJSON(cmd) {
				out := make([]map[string]any, 0, len(hooks))
				for _, e := range hooks {
					out = append(out, webhookMap(e))
				}
				return render.JSON(cmd.OutOrStdout(), out)
			}
			rows := make([][]string, 0, len(hooks))
			for _, e := range hooks {
				rows = append(rows, webhookRow(e))
			}
			return render.Table(cmd.OutOrStdout(), webhookColumns(), rows)
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project id")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func newUpdateCmd(clientFn ClientFn) *cobra.Command {
	var orgID, projectID, endpointURL, description string
	var types []string
	var active bool
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a webhook endpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.UpdateWebhook(cmd.Context(), connect.NewRequest(&webhookv1.UpdateWebhookRequest{
				OrgId:       orgID,
				ProjectId:   projectID,
				Id:          args[0],
				Url:         endpointURL,
				Description: description,
				EventTypes:  types,
				Active:      active,
			}))
			if err != nil {
				return err
			}
			if isJSON(cmd) {
				return render.JSON(cmd.OutOrStdout(), webhookMap(resp.Msg.GetWebhook()))
			}
			cmd.Printf("Updated webhook %s\n", resp.Msg.GetWebhook().GetId())
			return nil
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project id")
	cmd.Flags().StringVar(&endpointURL, "endpoint-url", "", "HTTPS endpoint URL")
	cmd.Flags().StringVar(&description, "description", "", "Description")
	cmd.Flags().StringSliceVar(&types, "event", nil, "Event type (repeatable)")
	cmd.Flags().BoolVar(&active, "active", true, "Whether the endpoint is active")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("project-id")
	_ = cmd.MarkFlagRequired("url")
	_ = cmd.MarkFlagRequired("event")
	return cmd
}

func newDeleteCmd(clientFn ClientFn) *cobra.Command {
	var orgID, projectID string
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a webhook endpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			req := &webhookv1.DeleteWebhookRequest{OrgId: orgID, ProjectId: projectID, Id: args[0]}
			_, err = client.DeleteWebhook(cmd.Context(), connect.NewRequest(req))
			if err != nil {
				return err
			}
			cmd.Printf("Deleted webhook %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project id")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func newRotateCmd(clientFn ClientFn) *cobra.Command {
	var orgID, projectID string
	cmd := &cobra.Command{
		Use:   "rotate <id>",
		Short: "Rotate a webhook endpoint's signing secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			req := &webhookv1.RotateWebhookSecretRequest{OrgId: orgID, ProjectId: projectID, Id: args[0]}
			resp, err := client.RotateWebhookSecret(cmd.Context(), connect.NewRequest(req))
			if err != nil {
				return err
			}
			cmd.Printf("New secret (shown once): %s\n", resp.Msg.GetSecret())
			return nil
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project id")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func newPingCmd(clientFn ClientFn) *cobra.Command {
	var orgID, projectID string
	cmd := &cobra.Command{
		Use:   "ping <id>",
		Short: "Queue a webhook.ping to an endpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			req := &webhookv1.PingWebhookRequest{OrgId: orgID, ProjectId: projectID, Id: args[0]}
			_, err = client.PingWebhook(cmd.Context(), connect.NewRequest(req))
			if err != nil {
				return err
			}
			cmd.Printf("Queued ping to webhook %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project id")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func webhookColumns() []string { return []string{"ID", "PROJECT_ID", "URL", "ACTIVE", "EVENT_TYPES"} }

func webhookRow(e *webhookv1.Webhook) []string {
	return []string{e.GetId(), e.GetProjectId(), e.GetUrl(), boolText(e.GetActive()), strings.Join(e.GetEventTypes(), ",")}
}

func webhookMap(e *webhookv1.Webhook) map[string]any {
	return map[string]any{
		"id":          e.GetId(),
		"orgId":       e.GetOrgId(),
		"projectId":   e.GetProjectId(),
		"url":         e.GetUrl(),
		"description": e.GetDescription(),
		"eventTypes":  e.GetEventTypes(),
		"active":      e.GetActive(),
	}
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func isJSON(cmd *cobra.Command) bool {
	switch render.Detect(cmd) {
	case render.FormatJSON, render.FormatNDJSON:
		return true
	default:
		return false
	}
}
