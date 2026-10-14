// Package redis opens and watches the one Redis connection the drivers share.
package redis

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/extra/redisotel/v9"
	goredis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace"
)

// Open connects and pings once within ConnectTimeout. SECURITY: errors name the host only; never the password.
func Open(ctx context.Context, c Config, tp trace.TracerProvider) (goredis.UniversalClient, error) {
	opts, err := goredis.ParseURL(c.URL)
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

	client := goredis.NewClient(opts)
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
