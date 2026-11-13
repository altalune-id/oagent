package knowledge

import (
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	knowledgev1 "altalune.id/oagent/gen/go/knowledge/v1"
	"altalune.id/oagent/gen/go/knowledge/v1/knowledgev1connect"
	"altalune.id/oagent/internal/render"
)

// ClientFn returns the Knowledge Connect client for a command; the caller resolves URL and token.
type ClientFn func(cmd *cobra.Command) (knowledgev1connect.KnowledgeServiceClient, error)

// NewCmd builds the `knowledge` command tree; it only lists, and sources and uploads are managed in the console, the API and MCP.
func NewCmd(clientFn ClientFn) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "knowledge",
		Short:   "List a project's knowledge sources",
		GroupID: "domain",
	}
	cmd.AddCommand(newListCmd(clientFn))
	return cmd
}

func newListCmd(clientFn ClientFn) *cobra.Command {
	var projectID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List knowledge sources in a project, oldest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			req := &knowledgev1.ListKnowledgeSourcesRequest{ProjectId: projectID}
			resp, err := client.ListKnowledgeSources(cmd.Context(), connect.NewRequest(req))
			if err != nil {
				return err
			}
			list := resp.Msg.GetSources()
			switch render.Detect(cmd) {
			case render.FormatJSON, render.FormatNDJSON:
				out := make([]map[string]any, 0, len(list))
				for _, s := range list {
					out = append(out, sourceMap(s))
				}
				return render.JSON(cmd.OutOrStdout(), out)
			}
			rows := make([][]string, 0, len(list))
			for _, s := range list {
				rows = append(rows, []string{
					s.GetId(), s.GetName(), enumWord(s.GetKind().String(), "KNOWLEDGE_SOURCE_KIND_"),
					enumWord(s.GetStatus().String(), "KNOWLEDGE_SOURCE_STATUS_"),
					strconv.Itoa(int(s.GetReadyCount())) + "/" + strconv.Itoa(int(s.GetDocumentCount())), s.GetSeedUrl(),
				})
			}
			return render.Table(cmd.OutOrStdout(), []string{"ID", "NAME", "KIND", "STATUS", "READY", "SEED_URL"}, rows)
		},
	}
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func enumWord(name, prefix string) string { return strings.ToLower(strings.TrimPrefix(name, prefix)) }

func sourceMap(s *knowledgev1.KnowledgeSource) map[string]any {
	out := map[string]any{
		"id":            s.GetId(),
		"projectId":     s.GetProjectId(),
		"name":          s.GetName(),
		"kind":          enumWord(s.GetKind().String(), "KNOWLEDGE_SOURCE_KIND_"),
		"backend":       enumWord(s.GetBackend().String(), "KNOWLEDGE_BACKEND_"),
		"status":        enumWord(s.GetStatus().String(), "KNOWLEDGE_SOURCE_STATUS_"),
		"seedUrl":       s.GetSeedUrl(),
		"documentCount": s.GetDocumentCount(),
		"readyCount":    s.GetReadyCount(),
		"failedCount":   s.GetFailedCount(),
		"createdAt":     s.GetCreatedAt().AsTime().UTC().Format(time.RFC3339),
	}
	if f := s.GetFailure(); f != nil {
		out["failure"] = map[string]string{"code": f.GetCode(), "reason": f.GetReason()}
	}
	return out
}
