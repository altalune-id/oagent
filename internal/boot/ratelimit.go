package boot

import (
	"cmp"
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

const defaultRunLimitKeys = 100_000

type limiters struct {
	auth      ratelimit.Limiter
	dataPlane ratelimit.Limiter
	ingest    ratelimit.Limiter
	// shared is the store every limited feature counts in, or nil when no rate limit is on.
	shared  ratelimit.Limiter
	janitor worker.Worker
}

func buildLimiters(cfg config.Config, rdb redis.UniversalClient, log *slog.Logger) (limiters, error) {
	out := limiters{auth: ratelimit.Disabled(), dataPlane: ratelimit.Disabled(), ingest: ratelimit.Disabled()}
	r := cfg.RateLimit
	if !r.AnyEnabled() {
		m := memory.New(cmp.Or(r.MaxKeys, defaultRunLimitKeys), memory.WithLogger(log))
		out.shared, out.janitor = m, m
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
	out.shared = limiter
	if r.Auth.Enabled {
		out.auth = limiter
	}
	if r.DataPlane.Enabled {
		out.dataPlane = limiter
	}
	if r.Ingest.Enabled {
		out.ingest = limiter
	}
	return out, nil
}
