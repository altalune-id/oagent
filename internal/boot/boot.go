// Package boot is the composition root: it wires config into the runtime graph.
package boot

import (
	"context"
	"fmt"
	"log/slog"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/config"
	"altalune.id/oagent/httpserver"
	"altalune.id/oagent/internal/controlplane"
	"altalune.id/oagent/internal/health"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/scheduler"
	"altalune.id/oagent/worker"
)

// Boot builds the runtime graph from cfg: telemetry, the DB pool, migrations, and the services.
func Boot(ctx context.Context, cfg config.Config, log *slog.Logger, opts ...Option) (_ *App, err error) {
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
	defer func() {
		if err != nil {
			_ = shutdown(ctx)
		}
	}()

	pool, err := openDB(ctx, cfg, log)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = pool.Close()
		}
	}()

	if cfg.DB.AutoMigrate {
		if err := migrate(ctx, cfg.Postgres); err != nil {
			return nil, err
		}
	}

	if err := guardRLS(ctx, pool, cfg, log); err != nil {
		return nil, err
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

	nc, nh, err := buildNATS(ctx, &k, cfg, log)
	if err != nil {
		return nil, err
	}
	if nc != nil {
		defer func() {
			if err != nil {
				nc.Close()
			}
		}()
	}

	qclient, err := buildQueueFn(ctx, k, cfg, nc, log)
	if err != nil {
		return nil, err
	}
	k.Queue = qclient

	rdb, rh, err := buildRedis(ctx, &k, cfg, tp, log)
	if err != nil {
		return nil, err
	}
	if rdb != nil {
		defer func() {
			if err != nil {
				_ = rdb.Close()
			}
		}()
	}

	svcs := buildServices(k, cfg)
	know, err := buildKnowledge(ctx, &k, cfg, svcs, log)
	if err != nil {
		return nil, err
	}
	if svcs.MCPTokens, err = buildMCPTokens(cfg, providerClient()); err != nil {
		return nil, err
	}
	if svcs.Mailer, err = buildMailer(cfg); err != nil {
		return nil, err
	}
	lims, err := buildLimiters(cfg, rdb, log)
	if err != nil {
		return nil, err
	}
	svcs.AuthLimiter = lims.auth
	runs, err := buildRun(k, cfg, svcs, lims.shared, log)
	if err != nil {
		return nil, err
	}
	svcs.RunStore, svcs.Runs, svcs.RunTurns, svcs.RunKicker = runs.store, runs.svc, runs.runner, runs.kicker
	svcs.RuntimeReady = runs.ready
	if err := k.Queue.Declare(queueJobs(), nil); err != nil {
		return nil, fmt.Errorf("boot: declare queue jobs: %w", err)
	}
	if svcs.Validator, err = controlplane.NewValidator(); err != nil {
		return nil, err
	}
	if err := buildAuthSecrets(cfg, svcs, log); err != nil {
		return nil, err
	}
	if err := firstBoot(ctx, cfg, svcs, log, logsInstallURL(o)); err != nil {
		return nil, err
	}
	if svcs.InstallRequired, err = installRequired(ctx, svcs); err != nil {
		return nil, err
	}
	warnSampleLegal(cfg, log, logsInstallURL(o))
	dbHealth := buildDBHealth(ctx, k, cfg, log)
	apiHandler := buildAPIHandler(svcs, k.Reporter.Unexpected, cfg.API, cfg.HTTP.BasePath, log)
	crossOrigin, err := buildCrossOrigin(cfg.HTTP.BaseURL)
	if err != nil {
		return nil, err
	}
	consoleHandler := buildConsoleHandler(cfg.HTTP.BasePath, cfg.OIDC.AccountURL)
	mcpServer := buildMCPServer(cfg.MCP, svcs, log, mcpPublicHost(cfg.HTTP)...)
	mcpResource, mcpDocs, err := buildMCPResource(cfg, mcpServer, log)
	if err != nil {
		return nil, err
	}

	sup := worker.New(log)
	sup.Register(httpserver.New(
		httpConfig(cfg.HTTP),
		buildHTTPHandler(httpSurfaces{
			log:        log,
			unexpected: k.Reporter.Unexpected,
			probes:     health.NewWeb(health.NewService(readinessChecks(dbHealth, rh, nh)...)),
			basePath:   cfg.HTTP.BasePath,
			api:        apiHandler,
			dataPlane:  buildDataPlane(cfg, svcs, lims, k.Reporter.Unexpected, log),
			consoleTry: buildConsoleTry(cfg, svcs, lims, k.Reporter.Unexpected, log),
			ingest:     buildIngest(cfg, svcs, db.NewTransactor(k.Pool), lims, log),
			console:    consoleHandler,
			mcp:        buildMCPHandler(mcpServer, svcs, mcpResource, log),
			mcpDocs:    mcpDocs,
			routes:     buildRoutes(cfg, svcs, log, k.Reporter.Unexpected),

			installRequired: svcs.InstallRequired,
			session:         buildSessionMiddleware(svcs, k.Reporter.Unexpected),
			consoleGate:     buildConsoleGate(cfg, svcs),

			requireAcceptance: cfg.Compliance.RequireAcceptance && svcs.Caps.LocalLogin,
			termsUpdated:      svcs.Legal.TermsUpdatedAt(),
			crossOrigin:       crossOrigin,
		}),
		log,
	))
	if rh != nil {
		sup.Register(rh)
	}
	if nh != nil {
		sup.Register(nh)
	}
	sup.Register(buildOutboxWorker(svcs, cfg.Outbox, log))
	sup.Register(buildMailWorker(svcs, cfg.Mail, log))
	sup.Register(svcs.KeyUsage)
	sup.Register(svcs.RunTurns)
	if know.s3Health != nil {
		sup.Register(know.s3Health)
	}
	if len(cfg.Knowledge.Backends()) > 0 {
		sup.Register(know.ingest)
		sup.Register(know.crawl)
	}
	if lims.janitor != nil {
		sup.Register(lims.janitor)
	}

	var runner *scheduler.Runner
	if off := schedulerOff(o, cfg); off == "" {
		r, schedErr := buildScheduler(k, cfg, svcs, log)
		if schedErr != nil {
			return nil, schedErr
		}
		runner = r
		sup.Register(runner)
		if svcs.ChannelReplier != nil {
			sup.Register(svcs.ChannelReplier.WithSweep(outboxTenants{orgs: svcs.Orgs}, cfg.Channel.ReplyIdlePoll))
		}
	} else if log != nil {
		log.Info("boot: scheduler disabled by " + off)
	}
	if o.schedulerOnly && runner == nil {
		return nil, fmt.Errorf("boot: scheduler-only requires the scheduler, but it is disabled by %s", schedulerOff(o, cfg))
	}

	var consumer worker.Worker
	if off := consumerOff(o, cfg); off == "" {
		c, cErr := k.Queue.Consumer(ctx, queueHandlers(svcs))
		if cErr != nil {
			return nil, fmt.Errorf("boot: queue consumer: %w", cErr)
		}
		consumer = c
		sup.Register(consumer)
	} else if log != nil {
		log.Info("boot: queue consumer disabled by " + off)
	}
	if o.consumerOnly && consumer == nil {
		return nil, fmt.Errorf("boot: consumer-only requires the queue consumer, but it is disabled by %s", consumerOff(o, cfg))
	}
	if cfg.Queue.Enabled {
		listener, lErr := k.Queue.Listener(ctx, queueListeners(svcs))
		if lErr != nil {
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
