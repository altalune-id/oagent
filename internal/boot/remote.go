package boot

import (
	"context"
	"log/slog"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/apiclient"
)

// Remote is the CLI-side graph used by remote admin commands: config, logger and a Connect client, no DB.
type Remote struct {
	Cfg  *config.Config
	Log  *slog.Logger
	Conn *apiclient.Client
}

// BootRemote builds the remote graph pointed at baseURL with the given credential.
func BootRemote(_ context.Context, cfg config.Config, log *slog.Logger, baseURL string, cred apiclient.Credential) (*Remote, error) {
	return &Remote{Cfg: &cfg, Log: log, Conn: apiclient.New(baseURL, cred)}, nil
}
