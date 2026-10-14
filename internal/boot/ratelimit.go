package boot

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/kernel/ratelimit/memory"
	ratelimitredis "altalune.id/oagent/internal/kernel/ratelimit/redis"
	"altalune.id/oagent/worker"
)

type limiters struct {
	auth    ratelimit.Limiter
	janitor worker.Worker
}

func buildLimiters(cfg config.Config, rdb redis.UniversalClient, log *slog.Logger) (limiters, error) {
	out := limiters{auth: ratelimit.Disabled()}
	r := cfg.RateLimit
	if !r.AnyEnabled() {
		return out, nil
	}
	var limiter ratelimit.Limiter
	switch r.Driver {
	case "", config.RateLimitDriverMemory:
		m := memory.New(r.MaxKeys, memory.WithLogger(log))
		limiter, out.janitor = m, m
	case config.RateLimitDriverRedis:
		if rdb == nil {
			return limiters{}, errors.New("boot: rateLimit.driver is redis but no Redis connection was opened")
		}
		limiter = ratelimitredis.New(rdb, cfg.Redis.KeyPrefix+"rl:")
	default:
		return limiters{}, fmt.Errorf("boot: rateLimit.driver %q is not supported", r.Driver)
	}
	if r.Auth.Enabled {
		out.auth = limiter
	}
	return out, nil
}
