// Package boot is the composition root: it wires config into the runtime graph.
package boot

import (
	"context"
	"fmt"
	"log/slog"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/httpserver"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/queue"
	"altalune.id/oagent/scheduler"
	"altalune.id/oagent/worker"
)

// Boot builds the runtime graph from cfg: telemetry, the DB pool, migrations, and the services.
func Boot(ctx context.Context, cfg config.Config, log *slog.Logger, opts ...Option) (*App, error) {
	o := newOptions()
	for _, opt := range opts {
		opt(o)
	}
	if o.logger != nil {
		log = o.logger
	}

	tp, mp, shutdown, err := setupTelemetry(ctx, telemetryConfig(cfg.Telemetry), log)
	if err != nil {
		return nil, err
	}

	pool, err := openPool(ctx, dbConfig(cfg.DB), log)
	if err != nil {
		_ = shutdown(ctx)
		return nil, err
	}

	if cfg.DB.AutoMigrate {
		if err := migrate(ctx, cfg.DB); err != nil {
			_ = pool.Close()
			_ = shutdown(ctx)
			return nil, err
		}
	}

	k := kernel.Kernel{
		Log:    log,
		Pool:   pool,
		Tracer: tp.Tracer(kernel.Scope),
		Meter:  mp.Meter(kernel.Scope),
		Reporter: apperror.NewReporter(log, cfg.Env.IsProduction(),
			apperror.WithIncidentCounter(newIncidentCounter(mp)),
			apperror.WithContextMeta(requestMeta),
		),
	}
	k.AddCloser(pool)

	qclient, err := buildQueueFn(ctx, k, cfg, log)
	if err != nil {
		_ = pool.Close()
		_ = shutdown(ctx)
		return nil, err
	}
	k.Queue = qclient
	k.AddCloser(qclient)

	svcs := buildServices(k, cfg)
	health := buildDBHealth(k, cfg, log)
	apiHandler := buildAPIHandler(svcs, k.Reporter.Unexpected, cfg.API, cfg.HTTP.BasePath)
	mcpServer := buildMCPServer(cfg.MCP, svcs, log)

	sup := worker.New(log)
	sup.Register(httpserver.New(httpConfig(cfg.HTTP), buildHTTPHandler(log, k.Reporter.Unexpected, health.Ready, cfg.HTTP.BasePath, apiHandler, mcpHTTPHandler(mcpServer)), log))
	sup.Register(health)

	var runner *scheduler.Runner
	if off := schedulerOff(o, cfg); off == "" {
		r, schedErr := buildScheduler(k, cfg, log)
		if schedErr != nil {
			_ = pool.Close()
			_ = shutdown(ctx)
			return nil, schedErr
		}
		runner = r
		sup.Register(runner)
	} else if log != nil {
		log.Info("boot: scheduler disabled by " + off)
	}
	if o.schedulerOnly && runner == nil {
		_ = pool.Close()
		_ = shutdown(ctx)
		return nil, fmt.Errorf("boot: scheduler-only requires the scheduler, but it is disabled by %s", schedulerOff(o, cfg))
	}

	var consumer *queue.Consumer
	if off := consumerOff(o, cfg); off == "" {
		c, cErr := newConsumer(ctx, k.Queue, queueHandlers(svcs))
		if cErr != nil {
			_ = pool.Close()
			_ = shutdown(ctx)
			return nil, fmt.Errorf("boot: queue consumer: %w", cErr)
		}
		consumer = c
		sup.Register(consumer)
	} else if log != nil {
		log.Info("boot: queue consumer disabled by " + off)
	}
	if o.consumerOnly && consumer == nil {
		_ = pool.Close()
		_ = shutdown(ctx)
		return nil, fmt.Errorf("boot: consumer-only requires the queue consumer, but it is disabled by %s", consumerOff(o, cfg))
	}
	if cfg.Queue.Enabled {
		seq, sErr := startSeq(ctx, k.Queue)
		if sErr != nil {
			_ = pool.Close()
			_ = shutdown(ctx)
			return nil, fmt.Errorf("boot: queue listener: %w", sErr)
		}
		listener, lErr := newListener(k.Queue, queueListeners(svcs), seq)
		if lErr != nil {
			_ = pool.Close()
			_ = shutdown(ctx)
			return nil, fmt.Errorf("boot: queue listener: %w", lErr)
		}
		sup.Register(listener)
	}

	return &App{
		Kernel:            k,
		Services:          svcs,
		Supervisor:        sup,
		MCP:               mcpServer,
		shutdownTelemetry: shutdown,
	}, nil
}
