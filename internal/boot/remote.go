package boot

import (
	"context"
	"log/slog"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/controlplane"
)

// Remote is the CLI-side graph used by remote admin commands: config, logger and a Connect client, no DB.
type Remote struct {
	Cfg  *config.Config
	Log  *slog.Logger
	Conn *controlplane.Client
}

// BootRemote builds the remote graph pointed at baseURL with the given bearer token.
func BootRemote(_ context.Context, cfg config.Config, log *slog.Logger, baseURL, token string) (*Remote, error) {
	return &Remote{Cfg: &cfg, Log: log, Conn: controlplane.NewClient(baseURL, token)}, nil
}
