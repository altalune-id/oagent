// Command auth-server is the authentication and organization service. See `auth-server --help`.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"altalune.id/auth-server/config"
	"altalune.id/auth-server/internal/boot"
	"altalune.id/auth-server/internal/cli"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := cli.NewRootCmd(bootApp)
	if err := root.ExecuteContext(ctx); err != nil {
		slog.ErrorContext(ctx, "auth-server", "error", err)
		return cli.ExitCodeFor(err)
	}
	return cli.ExitOK
}

func bootApp(ctx context.Context, cfg config.Config, log *slog.Logger, opts ...boot.Option) (cli.App, error) {
	return boot.Boot(ctx, cfg, log, opts...)
}
