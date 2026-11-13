package memory

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	memoryv1 "altalune.id/oagent/gen/go/memory/v1"
	"altalune.id/oagent/internal/render"
)

func newStoreCmd(clientFn ClientFn) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "store",
		Short: "List, create and delete a project's memory stores",
	}
	cmd.AddCommand(newStoreListCmd(clientFn), newStoreGetCmd(clientFn), newStoreCreateCmd(clientFn), newStoreDeleteCmd(clientFn))
	return cmd
}

func newStoreListCmd(clientFn ClientFn) *cobra.Command {
	var projectID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List memory stores in a project, oldest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.ListMemoryStores(cmd.Context(), connect.NewRequest(&memoryv1.ListMemoryStoresRequest{ProjectId: projectID}))
			if err != nil {
				return err
			}
			list := resp.Msg.GetStores()
			switch render.Detect(cmd) {
			case render.FormatJSON, render.FormatNDJSON:
				out := make([]map[string]any, 0, len(list))
				for _, s := range list {
					out = append(out, storeMap(s))
				}
				return render.JSON(cmd.OutOrStdout(), out)
			}
			rows := make([][]string, 0, len(list))
			for _, s := range list {
				rows = append(rows, []string{s.GetId(), s.GetName(), backendWord(s.GetBackend()), strings.Join(agentNames(s), ", ")})
			}
			return render.Table(cmd.OutOrStdout(), []string{"ID", "NAME", "BACKEND", "USED_BY"}, rows)
		},
	}
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func newStoreGetCmd(clientFn ClientFn) *cobra.Command {
	return &cobra.Command{
		Use:   "get <store-id>",
		Short: "Show a memory store and the agents that use it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			resp, err := client.GetMemoryStore(cmd.Context(), connect.NewRequest(&memoryv1.GetMemoryStoreRequest{Id: args[0]}))
			if err != nil {
				return err
			}
			s := resp.Msg.GetStore()
			switch render.Detect(cmd) {
			case render.FormatJSON, render.FormatNDJSON:
				return render.JSON(cmd.OutOrStdout(), storeMap(s))
			}
			return render.Table(cmd.OutOrStdout(), []string{"ID", "NAME", "BACKEND", "API_USERS", "USED_BY"}, [][]string{{
				s.GetId(), s.GetName(), backendWord(s.GetBackend()), strconv.FormatBool(s.GetRememberApiUsers()), strings.Join(agentNames(s), ", "),
			}})
		},
	}
}

func newStoreCreateCmd(clientFn ClientFn) *cobra.Command {
	var orgID, projectID string
	var apiUsers bool
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a memory store in a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			req := &memoryv1.CreateMemoryStoreRequest{OrgId: orgID, ProjectId: projectID, Name: args[0], RememberApiUsers: apiUsers}
			resp, err := client.CreateMemoryStore(cmd.Context(), connect.NewRequest(req))
			if err != nil {
				return err
			}
			switch render.Detect(cmd) {
			case render.FormatJSON, render.FormatNDJSON:
				return render.JSON(cmd.OutOrStdout(), storeMap(resp.Msg.GetStore()))
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), resp.Msg.GetStore().GetId())
			return err
		},
	}
	cmd.Flags().StringVar(&orgID, "org-id", "", "Organization public id")
	cmd.Flags().StringVar(&projectID, "project-id", "", "Project public id")
	cmd.Flags().BoolVar(&apiUsers, "remember-api-users", false,
		"Remember named API users; only when every API key that runs agents on the store belongs to one integration")
	_ = cmd.MarkFlagRequired("org-id")
	_ = cmd.MarkFlagRequired("project-id")
	return cmd
}

func newStoreDeleteCmd(clientFn ClientFn) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <store-id>",
		Short: "Delete a memory store and every fact in it; refused while a published agent version uses it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			_, err = client.DeleteMemoryStore(cmd.Context(), connect.NewRequest(&memoryv1.DeleteMemoryStoreRequest{Id: args[0]}))
			return err
		},
	}
}

func storeMap(s *memoryv1.MemoryStore) map[string]any {
	return map[string]any{
		"id": s.GetId(), "orgId": s.GetOrgId(), "projectId": s.GetProjectId(), "name": s.GetName(), "backend": backendWord(s.GetBackend()),
		"usedBy": agentNames(s), "rememberApiUsers": s.GetRememberApiUsers(), "createdAt": s.GetCreatedAt().AsTime().Format(time.RFC3339),
	}
}

func agentNames(s *memoryv1.MemoryStore) []string {
	out := make([]string, 0, len(s.GetUsedBy()))
	for _, u := range s.GetUsedBy() {
		out = append(out, u.GetAgentName())
	}
	return out
}

func backendWord(b memoryv1.MemoryBackend) string {
	return strings.ToLower(strings.TrimPrefix(b.String(), "MEMORY_BACKEND_"))
}
