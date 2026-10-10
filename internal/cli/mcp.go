package cli

import (
	"context"

	"github.com/spf13/cobra"

	"altalune.id/oagent/internal/boot"
)

func newMCPCmd(bootServer ServerBootFn) *cobra.Command {
	return &cobra.Command{
		Use:     "mcp",
		Short:   "Run the MCP server over stdio",
		GroupID: "runtime",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := runtimeFromCtx(cmd.Context())
			if err != nil {
				return err
			}
			app, err := bootServer(cmd.Context(), *rt.cfg, rt.log, boot.WithInstallURL(false))
			if err != nil {
				return err
			}
			defer func() { _ = app.Close(context.WithoutCancel(cmd.Context())) }()
			return app.RunMCP(cmd.Context())
		},
	}
}
