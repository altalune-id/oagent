package cli

import (
	"context"

	"github.com/spf13/cobra"

	"altalune.id/oagent/internal/boot"
)

func newServeCmd(bootServer ServerBootFn) *cobra.Command {
	var noScheduler, schedulerOnly, noConsumer, consumerOnly bool

	cmd := &cobra.Command{
		Use:     "serve",
		Short:   "Run the oagent HTTP server and workers",
		GroupID: "runtime",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := runtimeFromCtx(cmd.Context())
			if err != nil {
				return err
			}
			app, err := bootServer(cmd.Context(), *rt.cfg, rt.log,
				boot.WithScheduler(!noScheduler),
				boot.WithSchedulerOnly(schedulerOnly),
				boot.WithConsumer(!noConsumer),
				boot.WithConsumerOnly(consumerOnly),
			)
			if err != nil {
				return err
			}
			defer func() { _ = app.Close(context.WithoutCancel(cmd.Context())) }()

			cmd.Printf("oagent: listening on %s (basePath=%q, mode=%s)\n",
				rt.cfg.HTTP.Addr, rt.cfg.HTTP.BasePath, rt.cfg.Mode)
			return app.Run(cmd.Context())
		},
	}

	cmd.Flags().BoolVar(&noScheduler, "no-scheduler", false, "do not start the periodic-job runner")
	cmd.Flags().BoolVar(&schedulerOnly, "scheduler-only", false, "run only the scheduler and a health endpoint")
	cmd.Flags().BoolVar(&noConsumer, "no-consumer", false, "do not start the queue job consumer")
	cmd.Flags().BoolVar(&consumerOnly, "consumer-only", false, "run only the queue consumer and a health endpoint")
	cmd.MarkFlagsMutuallyExclusive("no-scheduler", "scheduler-only")
	cmd.MarkFlagsMutuallyExclusive("no-consumer", "consumer-only")
	cmd.MarkFlagsMutuallyExclusive("scheduler-only", "consumer-only")

	return cmd
}
