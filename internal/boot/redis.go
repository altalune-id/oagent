package boot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/health"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/worker"
)

const redisProbeEvery = 5 * time.Second

type redisOpener func(context.Context, config.RedisConfig, trace.TracerProvider) (redis.UniversalClient, error)

//nolint:gochecknoglobals // test seam.
var openRedisFn redisOpener = openRedis

// SECURITY: errors name the host only; the URL and the server's reply never carry the password.
func openRedis(ctx context.Context, c config.RedisConfig, tp trace.TracerProvider) (redis.UniversalClient, error) {
	opts, err := redis.ParseURL(c.URL)
	if err != nil {
		return nil, errors.New("redis: redis.url is not a valid redis:// or rediss:// URL")
	}
	opts.PoolSize = c.MaxConnections
	opts.MinIdleConns = c.MinIdleConnections
	opts.ConnMaxIdleTime = c.MaxIdleTime
	opts.DialTimeout = c.ConnectTimeout
	opts.ReadTimeout = c.ReadTimeout
	opts.WriteTimeout = c.WriteTimeout
	opts.ContextTimeoutEnabled = true

	client := redis.NewClient(opts)
	// SECURITY: no db.statement on spans; keys hold client IPs and email hashes.
	if err := redisotel.InstrumentTracing(client, redisotel.WithTracerProvider(tp), redisotel.WithDBStatement(false)); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis: tracing: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, c.ConnectTimeout)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis: cannot connect to %s (redis.url): %w", opts.Addr, err)
	}
	return client, nil
}

func buildRedis(ctx context.Context, k *kernel.Kernel, cfg config.Config, tp trace.TracerProvider, log *slog.Logger) (*redisHealth, error) {
	features := cfg.RedisFeatures()
	if len(features) == 0 {
		if cfg.Redis.URL != "" && log != nil {
			log.InfoContext(ctx, "boot: redis.url is set but no feature uses Redis; not connecting")
		}
		return nil, nil
	}
	client, err := openRedisFn(ctx, cfg.Redis, tp)
	if err != nil {
		return nil, fmt.Errorf("boot: %w", err)
	}
	k.AddCloser(client)
	if log != nil {
		log.InfoContext(ctx, "boot: redis connected", slog.String("used_by", strings.Join(features, ", ")))
	}
	return newRedisHealth(client, cfg.Redis.ReadTimeout, log), nil
}

func readinessChecks(dbHealth *db.HealthMonitor, rh *redisHealth) []health.Check {
	checks := []health.Check{dbHealth}
	if rh != nil {
		checks = append(checks, rh)
	}
	return checks
}

var (
	_ worker.Worker = (*redisHealth)(nil)
	_ health.Check  = (*redisHealth)(nil)
)

type redisHealth struct {
	client  redis.UniversalClient
	timeout time.Duration
	log     *slog.Logger
	ready   atomic.Bool
}

func newRedisHealth(client redis.UniversalClient, timeout time.Duration, log *slog.Logger) *redisHealth {
	h := &redisHealth{client: client, timeout: timeout, log: log}
	h.ready.Store(true)
	return h
}

func (h *redisHealth) conn() redis.UniversalClient {
	if h == nil {
		return nil
	}
	return h.client
}

// Name implements worker.Worker.
func (h *redisHealth) Name() string { return "redis-health" }

// Ready implements health.Check.
func (h *redisHealth) Ready() bool { return h.ready.Load() }

// Run implements worker.Worker: it probes now, then every redisProbeEvery, until ctx is done.
func (h *redisHealth) Run(ctx context.Context) error {
	h.probe(ctx)
	ticker := time.NewTicker(redisProbeEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			h.probe(ctx)
		}
	}
}

func (h *redisHealth) probe(ctx context.Context) {
	pingCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	err := h.client.Ping(pingCtx).Err()
	if ctx.Err() != nil {
		return
	}
	if was := h.ready.Swap(err == nil); was == (err == nil) || h.log == nil {
		return
	}
	if err != nil {
		h.log.WarnContext(ctx, "redis: not ready", slog.String("err", err.Error()))
		return
	}
	h.log.InfoContext(ctx, "redis: ready again")
}
