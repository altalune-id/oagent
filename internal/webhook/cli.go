package webhook

import (
	"strconv"
	"strings"
	"time"

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
		newRetryCmd(clientFn),
		newRetryFailedCmd(clientFn),
		newDeliveriesCmd(clientFn),
		newAttemptsCmd(clientFn),
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
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
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
		Use:   "get <public-id>",
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
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
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
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func newUpdateCmd(clientFn ClientFn) *cobra.Command {
	var orgID, projectID, endpointURL, description string
	var types []string
	var active bool
	cmd := &cobra.Command{
		Use:   "update <public-id>",
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
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
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
		Use:   "delete <public-id>",
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
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func newRotateCmd(clientFn ClientFn) *cobra.Command {
	var orgID, projectID string
	cmd := &cobra.Command{
		Use:   "rotate <public-id>",
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
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func newPingCmd(clientFn ClientFn) *cobra.Command {
	var orgID, projectID string
	cmd := &cobra.Command{
		Use:   "ping <public-id>",
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
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func newRetryCmd(clientFn ClientFn) *cobra.Command {
	var orgID, projectID string
	cmd := &cobra.Command{
		Use:   "retry <public-id> <delivery-public-id>",
		Short: "Requeue one failed delivery of an endpoint",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			req := &webhookv1.RetryWebhookDeliveryRequest{OrgId: orgID, ProjectId: projectID, Id: args[0], DeliveryId: args[1]}
			if _, err = client.RetryWebhookDelivery(cmd.Context(), connect.NewRequest(req)); err != nil {
				return err
			}
			cmd.Printf("Requeued delivery %s\n", args[1])
			return nil
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func newRetryFailedCmd(clientFn ClientFn) *cobra.Command {
	var orgID, projectID string
	cmd := &cobra.Command{
		Use:   "retry-failed <public-id>",
		Short: "Requeue every failed delivery of an endpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			req := &webhookv1.RetryFailedWebhookDeliveriesRequest{OrgId: orgID, ProjectId: projectID, Id: args[0]}
			resp, err := client.RetryFailedWebhookDeliveries(cmd.Context(), connect.NewRequest(req))
			if err != nil {
				return err
			}
			cmd.Printf("Requeued %d deliveries\n", resp.Msg.GetRequeued())
			return nil
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func newDeliveriesCmd(clientFn ClientFn) *cobra.Command {
	var orgID, projectID string
	var limit int
	cmd := &cobra.Command{
		Use:   "deliveries <public-id>",
		Short: "List a webhook endpoint's recent deliveries",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			req := &webhookv1.ListWebhookDeliveriesRequest{
				OrgId: orgID, ProjectId: projectID, Id: args[0],
				Limit: int32(limit), //nolint:gosec // G115: bounded by the flag
			}
			resp, err := client.ListWebhookDeliveries(cmd.Context(), connect.NewRequest(req))
			if err != nil {
				return err
			}
			items := resp.Msg.GetDeliveries()
			if isJSON(cmd) {
				out := make([]map[string]any, 0, len(items))
				for _, d := range items {
					out = append(out, deliveryMap(d))
				}
				return render.JSON(cmd.OutOrStdout(), out)
			}
			rows := make([][]string, 0, len(items))
			for _, d := range items {
				rows = append(rows, []string{
					d.GetId(), d.GetEventType(), d.GetStatus(),
					strconv.Itoa(int(d.GetAttempt())), d.GetCreatedAt().AsTime().Format(time.RFC3339),
				})
			}
			return render.Table(cmd.OutOrStdout(), []string{"ID", "EVENT", "STATUS", "ATTEMPT", "CREATED_AT"}, rows)
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
	cmd.Flags().IntVar(&limit, "limit", 20, "Max deliveries")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func newAttemptsCmd(clientFn ClientFn) *cobra.Command {
	var orgID, projectID string
	cmd := &cobra.Command{
		Use:   "attempts <public-id> <delivery-public-id>",
		Short: "List the attempts of one delivery",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			req := &webhookv1.ListWebhookDeliveryAttemptsRequest{OrgId: orgID, ProjectId: projectID, Id: args[0], DeliveryId: args[1]}
			resp, err := client.ListWebhookDeliveryAttempts(cmd.Context(), connect.NewRequest(req))
			if err != nil {
				return err
			}
			items := resp.Msg.GetAttempts()
			if isJSON(cmd) {
				out := make([]map[string]any, 0, len(items))
				for _, a := range items {
					out = append(out, attemptMap(a))
				}
				return render.JSON(cmd.OutOrStdout(), out)
			}
			rows := make([][]string, 0, len(items))
			for _, a := range items {
				rows = append(rows, []string{
					strconv.Itoa(int(a.GetAttempt())), strconv.Itoa(int(a.GetStatusCode())),
					strconv.Itoa(int(a.GetDurationMs())) + "ms", a.GetError(),
				})
			}
			return render.Table(cmd.OutOrStdout(), []string{"ATTEMPT", "STATUS", "DURATION", "ERROR"}, rows)
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
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

func deliveryMap(d *webhookv1.WebhookDelivery) map[string]any {
	return map[string]any{
		"id":        d.GetId(),
		"eventId":   d.GetEventId(),
		"eventType": d.GetEventType(),
		"status":    d.GetStatus(),
		"attempt":   d.GetAttempt(),
		"lastError": d.GetLastError(),
		"payload":   d.GetPayload(),
		"createdAt": d.GetCreatedAt().AsTime().Format(time.RFC3339),
	}
}

func attemptMap(a *webhookv1.WebhookAttempt) map[string]any {
	headers := make([]map[string]string, 0, len(a.GetResponseHeaders()))
	for _, h := range a.GetResponseHeaders() {
		headers = append(headers, map[string]string{"name": h.GetName(), "value": h.GetValue()})
	}
	return map[string]any{
		"attempt":           a.GetAttempt(),
		"statusCode":        a.GetStatusCode(),
		"error":             a.GetError(),
		"durationMs":        a.GetDurationMs(),
		"responseBody":      a.GetResponseBody(),
		"responseTruncated": a.GetResponseTruncated(),
		"responseHeaders":   headers,
		"createdAt":         a.GetCreatedAt().AsTime().Format(time.RFC3339),
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
