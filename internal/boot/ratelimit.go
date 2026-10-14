package boot

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"altalune.id/oagent/config"
	"altalune.id/oagent/ratelimit"
	"altalune.id/oagent/ratelimit/memory"
	ratelimitredis "altalune.id/oagent/ratelimit/redis"
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
	var store ratelimit.Limiter
	switch r.Store {
	case "", config.RateLimitStoreMemory:
		m := memory.New(r.MaxKeys, memory.WithLogger(log))
		store, out.janitor = m, m
	case config.RateLimitStoreRedis:
		if rdb == nil {
			return limiters{}, errors.New("boot: rateLimit.store is redis but no Redis connection was opened")
		}
		store = ratelimitredis.New(rdb, cfg.Redis.KeyPrefix+"rl:")
	default:
		return limiters{}, fmt.Errorf("boot: rateLimit.store %q is not supported", r.Store)
	}
	if r.Auth.Enabled {
		out.auth = store
	}
	return out, nil
}
