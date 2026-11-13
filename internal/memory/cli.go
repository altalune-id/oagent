package memory

import (
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	memoryv1 "altalune.id/oagent/gen/go/memory/v1"
	"altalune.id/oagent/gen/go/memory/v1/memoryv1connect"
	"altalune.id/oagent/internal/render"
)

// ClientFn returns the Memory Connect client for a command; the caller resolves URL and token.
type ClientFn func(cmd *cobra.Command) (memoryv1connect.MemoryServiceClient, error)

// NewCmd builds the `memory` command tree: a project's stores, and what agents remember about an end user in one.
func NewCmd(clientFn ClientFn) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "memory",
		Short:   "Manage memory stores and what agents remember about an end user",
		GroupID: "domain",
	}
	cmd.AddCommand(newStoreCmd(clientFn), newListCmd(clientFn), newDeleteCmd(clientFn), newClearCmd(clientFn))
	return cmd
}

func newListCmd(clientFn ClientFn) *cobra.Command {
	var store, endUser, agent string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List an end user's remembered facts, newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			req := &memoryv1.ListMemoriesRequest{StoreId: store, EndUserId: endUser, AgentId: agent}
			resp, err := client.ListMemories(cmd.Context(), connect.NewRequest(req))
			if err != nil {
				return err
			}
			list := resp.Msg.GetMemories()
			switch render.Detect(cmd) {
			case render.FormatJSON, render.FormatNDJSON:
				out := make([]map[string]any, 0, len(list))
				for _, m := range list {
					out = append(out, map[string]any{
						"id": m.GetId(), "agentId": m.GetAgentId(), "text": m.GetText(),
						"createdAt": m.GetCreatedAt().AsTime().Format(time.RFC3339), "sourceRunId": m.GetSourceRunId(),
					})
				}
				return render.JSON(cmd.OutOrStdout(), out)
			}
			rows := make([][]string, 0, len(list))
			for _, m := range list {
				rows = append(rows, []string{m.GetId(), m.GetAgentId(), m.GetCreatedAt().AsTime().Format(time.RFC3339), m.GetText()})
			}
			return render.Table(cmd.OutOrStdout(), []string{"ID", "AGENT_ID", "CREATED_AT", "TEXT"}, rows)
		},
	}
	cmd.Flags().StringVar(&store, "store", "", "Memory store public id")
	cmd.Flags().StringVar(&endUser, "end-user", "", "End user public id")
	cmd.Flags().StringVar(&agent, "agent", "", "Agent public id; empty lists every agent's facts")
	_ = cmd.MarkFlagRequired("store")
	_ = cmd.MarkFlagRequired("end-user")
	return cmd
}

func newDeleteCmd(clientFn ClientFn) *cobra.Command {
	var store, endUser string
	cmd := &cobra.Command{
		Use:   "delete <fact-id>",
		Short: "Delete one remembered fact of an end user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			req := &memoryv1.DeleteMemoryRequest{StoreId: store, EndUserId: endUser, Id: args[0]}
			_, err = client.DeleteMemory(cmd.Context(), connect.NewRequest(req))
			return err
		},
	}
	cmd.Flags().StringVar(&store, "store", "", "Memory store public id")
	cmd.Flags().StringVar(&endUser, "end-user", "", "End user public id")
	_ = cmd.MarkFlagRequired("store")
	_ = cmd.MarkFlagRequired("end-user")
	return cmd
}

func newClearCmd(clientFn ClientFn) *cobra.Command {
	var store, endUser, agent string
	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Delete every remembered fact of an end user, or one agent's",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := clientFn(cmd)
			if err != nil {
				return err
			}
			req := &memoryv1.ClearMemoriesRequest{StoreId: store, EndUserId: endUser, AgentId: agent}
			resp, err := client.ClearMemories(cmd.Context(), connect.NewRequest(req))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "deleted %d\n", resp.Msg.GetDeleted())
			return err
		},
	}
	cmd.Flags().StringVar(&store, "store", "", "Memory store public id")
	cmd.Flags().StringVar(&endUser, "end-user", "", "End user public id")
	cmd.Flags().StringVar(&agent, "agent", "", "Agent public id; empty clears every agent's facts")
	_ = cmd.MarkFlagRequired("store")
	_ = cmd.MarkFlagRequired("end-user")
	return cmd
}
