package boot

import (
	"fmt"
	"log/slog"

	"altalune.id/oagent/config"
	"altalune.id/oagent/ratelimit"
	"altalune.id/oagent/ratelimit/memory"
	"altalune.id/oagent/worker"
)

type limiters struct {
	auth    ratelimit.Limiter
	janitor worker.Worker
}

func buildLimiters(cfg config.Config, log *slog.Logger) (limiters, error) {
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
	default:
		return limiters{}, fmt.Errorf("boot: rateLimit.store %q is not supported", r.Store)
	}
	if r.Auth.Enabled {
		out.auth = store
	}
	return out, nil
}
