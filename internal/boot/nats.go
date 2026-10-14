package boot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	natsgo "github.com/nats-io/nats.go"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/health"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/nats"
)

type natsOpener func(context.Context, nats.Config, *slog.Logger) (*natsgo.Conn, error)

//nolint:gochecknoglobals // test seam.
var openNATSFn natsOpener = nats.Open

var _ health.Check = (*nats.Health)(nil)

func natsConfig(c config.NATSConfig) nats.Config {
	return nats.Config{
		URL:            c.URL,
		Token:          c.Token,
		User:           c.User,
		Password:       c.Password,
		Name:           c.Name,
		ConnectTimeout: c.ConnectTimeout,
	}
}

func buildNATS(ctx context.Context, k *kernel.Kernel, cfg config.Config, log *slog.Logger) (*natsgo.Conn, *nats.Health, error) {
	features := cfg.NATSFeatures()
	if len(features) == 0 {
		if cfg.NATS.URL != "" && log != nil {
			log.InfoContext(ctx, "boot: nats.url is set but no feature uses NATS; not connecting")
		}
		return nil, nil, nil
	}
	nc, err := openNATSFn(ctx, natsConfig(cfg.NATS), log)
	if err != nil {
		return nil, nil, fmt.Errorf("boot: %w", err)
	}
	k.AddCloser(nats.Closer(nc))
	if log != nil {
		log.InfoContext(ctx, "boot: nats connected", slog.String("used_by", strings.Join(features, ", ")))
	}
	return nc, nats.NewHealth(nc, log), nil
}
