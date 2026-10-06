// Command oagent is the authentication and organization service. See `oagent --help`.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/boot"
	"altalune.id/oagent/internal/cli"
	"altalune.id/oagent/internal/controlplane"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := cli.NewRootCmd(bootApp, remoteApp)
	if err := root.ExecuteContext(ctx); err != nil {
		cli.RenderError(root, err)
		return cli.ExitCodeFor(err)
	}
	return cli.ExitOK
}

func bootApp(ctx context.Context, cfg config.Config, log *slog.Logger, opts ...boot.Option) (cli.App, error) {
	return boot.Boot(ctx, cfg, log, opts...)
}

func remoteApp(ctx context.Context, cfg config.Config, log *slog.Logger, baseURL, token string) (*controlplane.Client, error) {
	r, err := boot.BootRemote(ctx, cfg, log, baseURL, token)
	if err != nil {
		return nil, err
	}
	return r.Conn, nil
}
