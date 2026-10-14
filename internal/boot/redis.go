package boot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	goredis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/health"
	"altalune.id/oagent/internal/kernel"
	redisconn "altalune.id/oagent/redis"
)

type redisOpener func(context.Context, redisconn.Config, trace.TracerProvider) (goredis.UniversalClient, error)

//nolint:gochecknoglobals // test seam.
var openRedisFn redisOpener = redisconn.Open

var _ health.Check = (*redisconn.Health)(nil)

func redisConfig(c config.RedisConfig) redisconn.Config {
	return redisconn.Config{
		URL:                c.URL,
		MaxConnections:     c.MaxConnections,
		MinIdleConnections: c.MinIdleConnections,
		MaxIdleTime:        c.MaxIdleTime,
		ConnectTimeout:     c.ConnectTimeout,
		ReadTimeout:        c.ReadTimeout,
		WriteTimeout:       c.WriteTimeout,
	}
}

func buildRedis(
	ctx context.Context, k *kernel.Kernel, cfg config.Config, tp trace.TracerProvider, log *slog.Logger,
) (goredis.UniversalClient, *redisconn.Health, error) {
	features := cfg.RedisFeatures()
	if len(features) == 0 {
		if cfg.Redis.URL != "" && log != nil {
			log.InfoContext(ctx, "boot: redis.url is set but no feature uses Redis; not connecting")
		}
		return nil, nil, nil
	}
	client, err := openRedisFn(ctx, redisConfig(cfg.Redis), tp)
	if err != nil {
		return nil, nil, fmt.Errorf("boot: %w", err)
	}
	k.AddCloser(client)
	if log != nil {
		log.InfoContext(ctx, "boot: redis connected", slog.String("used_by", strings.Join(features, ", ")))
	}
	return client, redisconn.NewHealth(client, cfg.Redis.ReadTimeout, log), nil
}
