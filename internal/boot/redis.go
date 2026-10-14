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
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/nats"
	"altalune.id/oagent/internal/kernel/redis"
)

type redisOpener func(context.Context, redis.Config, trace.TracerProvider) (goredis.UniversalClient, error)

//nolint:gochecknoglobals // test seam.
var openRedisFn redisOpener = redis.Open

var _ health.Check = (*redis.Health)(nil)

func redisConfig(c config.RedisConfig) redis.Config {
	return redis.Config{
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
) (goredis.UniversalClient, *redis.Health, error) {
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
	return client, redis.NewHealth(client, cfg.Redis.ReadTimeout, log), nil
}

func readinessChecks(dbHealth *db.HealthMonitor, rh *redis.Health, nh *nats.Health) []health.Check {
	checks := []health.Check{dbHealth}
	if rh != nil {
		checks = append(checks, rh)
	}
	if nh != nil {
		checks = append(checks, nh)
	}
	return checks
}
