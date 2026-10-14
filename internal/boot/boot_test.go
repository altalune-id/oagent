package boot

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/queue"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/migrator"
	"altalune.id/oagent/ratelimit"
	"altalune.id/oagent/reqid"
	"altalune.id/oagent/telemetry"
	"altalune.id/oagent/worker"
)

func restoreSeams(t *testing.T) {
	t.Helper()
	s, p, m, u, l := setupTelemetry, openPool, openMigrator, migrateUp, withLock
	bq, nc, nl, ss, or := buildQueueFn, newConsumer, newListener, startSeq, openRedisFn
	t.Cleanup(func() {
		setupTelemetry, openPool, openMigrator, migrateUp, withLock = s, p, m, u, l
		buildQueueFn, newConsumer, newListener, startSeq, openRedisFn = bq, nc, nl, ss, or
	})
}

func stubTelemetry(called *bool, err error) telemetrySetupFunc {
	return func(context.Context, telemetry.Config, *slog.Logger) (trace.TracerProvider, metric.MeterProvider, func(context.Context) error, error) {
		if err != nil {
			return nil, nil, nil, err
		}
		return nooptrace.NewTracerProvider(), noopmetric.NewMeterProvider(), func(context.Context) error {
			if called != nil {
				*called = true
			}
			return nil
		}, nil
	}
}

func passthroughLock(_ context.Context, _ *sql.DB, _ string, fn func(context.Context) error) error {
	return fn(context.Background())
}

func newDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectClose()
	return sqlDB, mock
}

func TestBoot(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	migDB, _ := newDB(t)

	called := false
	setupTelemetry = stubTelemetry(&called, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	openMigrator = func(context.Context, string, migrator.Config) (*sql.DB, error) { return migDB, nil }
	migrateUp = func(context.Context, *sql.DB, migrator.Config) error { return nil }
	withLock = passthroughLock

	app, err := Boot(context.Background(), config.Config{DB: config.DBConfig{DSN: "x", Schema: "public", AutoMigrate: true}}, nil)
	require.NoError(t, err)
	assert.NotNil(t, app.Services.Orgs)
	assert.Same(t, poolDB, app.Kernel.Pool.W)
	assert.NotNil(t, app.Kernel.Tracer)
	assert.NotNil(t, app.Kernel.Meter)
	assert.NotNil(t, app.Kernel.Queue) // always non-nil; Disabled when queue.enabled=false
	require.NotNil(t, app.Supervisor)
	assert.Len(t, app.Supervisor.Workers(), 5) // http + db-health + outbox + mailqueue + apikey-usage
	assert.Contains(t, workerNames(app), "apikey-usage")
	assert.Contains(t, workerNames(app), "mailqueue.send")
	assert.NotNil(t, app.Services.Mailer)
	assert.NotNil(t, app.Services.MailQueue)
	assert.NotNil(t, app.Services.KeyAuthn)
	assert.Equal(t, ratelimit.Disabled(), app.Services.AuthLimiter, "rate limits are off by default")
	assert.NotContains(t, workerNames(app), "ratelimit.janitor")

	require.NoError(t, app.Close(context.Background()))
	assert.True(t, called)
}

func TestBoot_SchedulerEnabled(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)

	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	app, err := Boot(context.Background(), config.Config{
		DB:        config.DBConfig{DSN: "x"},
		Scheduler: config.SchedulerConfig{Enabled: true},
	}, testLogger())
	require.NoError(t, err)
	assert.Len(t, app.Supervisor.Workers(), 6) // http + db-health + outbox + mailqueue + apikey-usage + scheduler
	require.NoError(t, app.Close(context.Background()))
}

func TestBoot_WithSchedulerFalse(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	app, err := Boot(context.Background(),
		config.Config{DB: config.DBConfig{DSN: "x"}, Scheduler: config.SchedulerConfig{Enabled: true}},
		testLogger(), WithScheduler(false))
	require.NoError(t, err)
	assert.Len(t, app.Supervisor.Workers(), 5) // http + db-health + outbox + mailqueue + apikey-usage
	require.NoError(t, app.Close(context.Background()))
}

func TestBoot_SchedulerOnlyDisabled(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	_, err := Boot(context.Background(),
		config.Config{DB: config.DBConfig{DSN: "x"}}, testLogger(), WithSchedulerOnly(true))
	assert.Error(t, err)
}

func TestBoot_ConsumerOnlyDisabled(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	_, err := Boot(context.Background(),
		config.Config{DB: config.DBConfig{DSN: "x"}}, testLogger(), WithConsumerOnly(true))
	assert.Error(t, err)
}

func TestBoot_ConsumerEnabled(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	buildQueueFn = func(context.Context, kernel.Kernel, config.Config, *slog.Logger) (*queue.Client, error) {
		return queue.Disabled(testLogger()), nil
	}
	newConsumer = func(context.Context, *queue.Client, []queue.Handler) (*queue.Consumer, error) {
		return &queue.Consumer{}, nil
	}
	newListener = func(*queue.Client, []queue.Listener, uint64) (*queue.Listen, error) {
		return &queue.Listen{}, nil
	}
	startSeq = func(context.Context, *queue.Client) (uint64, error) { return 1, nil }

	app, err := Boot(context.Background(), config.Config{
		DB:    config.DBConfig{DSN: "x"},
		Queue: config.QueueConfig{Enabled: true},
	}, testLogger())
	require.NoError(t, err)
	assert.Len(t, app.Supervisor.Workers(), 7) // http + db-health + outbox + mailqueue + apikey-usage + consumer + listener
	assert.False(t, app.Kernel.Queue.Enabled())
	require.NoError(t, app.Close(context.Background()))
}

func TestBoot_ConsumerError(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	buildQueueFn = func(context.Context, kernel.Kernel, config.Config, *slog.Logger) (*queue.Client, error) {
		return queue.Disabled(testLogger()), nil
	}
	newConsumer = func(context.Context, *queue.Client, []queue.Handler) (*queue.Consumer, error) {
		return nil, errors.New("boom")
	}

	_, err := Boot(context.Background(), config.Config{DB: config.DBConfig{DSN: "x"}, Queue: config.QueueConfig{Enabled: true}}, testLogger())
	assert.Error(t, err)
}

func TestBoot_ListenerError(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	buildQueueFn = func(context.Context, kernel.Kernel, config.Config, *slog.Logger) (*queue.Client, error) {
		return queue.Disabled(testLogger()), nil
	}
	newConsumer = func(context.Context, *queue.Client, []queue.Handler) (*queue.Consumer, error) {
		return &queue.Consumer{}, nil
	}
	startSeq = func(context.Context, *queue.Client) (uint64, error) { return 0, errors.New("boom") }

	_, err := Boot(context.Background(), config.Config{DB: config.DBConfig{DSN: "x"}, Queue: config.QueueConfig{Enabled: true}}, testLogger())
	assert.Error(t, err)
}

func TestBoot_NoAutoMigrate(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	require.NoError(t, poolDB.Close()) // closed since App isn't closed here

	migratorCalled := false
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	openMigrator = func(context.Context, string, migrator.Config) (*sql.DB, error) {
		migratorCalled = true
		return nil, errors.New("should not be called")
	}

	app, err := Boot(context.Background(), config.Config{DB: config.DBConfig{DSN: "x", AutoMigrate: false}}, nil)
	require.NoError(t, err)
	assert.NotNil(t, app.Services.Orgs)
	assert.False(t, migratorCalled)
}

func TestBoot_TelemetryError(t *testing.T) {
	restoreSeams(t)
	setupTelemetry = stubTelemetry(nil, errors.New("boom"))

	_, err := Boot(context.Background(), config.Config{}, nil)
	assert.Error(t, err)
}

func TestBoot_PoolError(t *testing.T) {
	restoreSeams(t)
	called := false
	setupTelemetry = stubTelemetry(&called, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{}, errors.New("boom")
	}

	_, err := Boot(context.Background(), config.Config{DB: config.DBConfig{DSN: "x"}}, nil)
	assert.Error(t, err)
	assert.True(t, called)
}

func TestBoot_MigratorError(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)

	called := false
	setupTelemetry = stubTelemetry(&called, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	openMigrator = func(context.Context, string, migrator.Config) (*sql.DB, error) { return nil, errors.New("boom") }

	_, err := Boot(context.Background(), config.Config{DB: config.DBConfig{DSN: "x", AutoMigrate: true}}, nil)
	assert.Error(t, err)
	assert.True(t, called)
}

func TestBoot_MigrateError(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	migDB, _ := newDB(t)

	called := false
	setupTelemetry = stubTelemetry(&called, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	openMigrator = func(context.Context, string, migrator.Config) (*sql.DB, error) { return migDB, nil }
	migrateUp = func(context.Context, *sql.DB, migrator.Config) error { return errors.New("boom") }
	withLock = passthroughLock

	_, err := Boot(context.Background(), config.Config{DB: config.DBConfig{DSN: "x", AutoMigrate: true}}, nil)
	assert.Error(t, err)
	assert.True(t, called)
}

func TestBoot_LockError(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	migDB, _ := newDB(t)

	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	openMigrator = func(context.Context, string, migrator.Config) (*sql.DB, error) { return migDB, nil }
	withLock = func(context.Context, *sql.DB, string, func(context.Context) error) error { return errors.New("boom") }

	_, err := Boot(context.Background(), config.Config{DB: config.DBConfig{DSN: "x", AutoMigrate: true}}, nil)
	assert.Error(t, err)
}

func TestApp_Close_ShutdownError(t *testing.T) {
	poolDB, _ := newDB(t)
	k := kernel.Kernel{Pool: db.Pool{W: poolDB, R: poolDB}}
	k.AddCloser(poolDB)

	app := &App{Kernel: k, shutdownTelemetry: func(context.Context) error { return errors.New("shutdown") }}
	assert.Error(t, app.Close(context.Background()))
}

func TestApp_Close_PoolError(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectClose().WillReturnError(errors.New("close"))

	k := kernel.Kernel{Pool: db.Pool{W: sqlDB, R: sqlDB}}
	k.AddCloser(sqlDB)

	app := &App{Kernel: k, shutdownTelemetry: func(context.Context) error { return nil }}
	assert.Error(t, app.Close(context.Background()))
}

func TestIncidentCounter(t *testing.T) {
	c := newIncidentCounter(noopmetric.NewMeterProvider())
	c.Inc(context.Background(), "unexpected") // no-op provider; must not panic
}

type fakeWorker struct {
	name string
	run  func(context.Context) error
}

func (f fakeWorker) Name() string                  { return f.name }
func (f fakeWorker) Run(ctx context.Context) error { return f.run(ctx) }

func TestWithLogger(t *testing.T) {
	o := newOptions()
	WithLogger(testLogger())(o)
	assert.NotNil(t, o.logger)
}

func TestBoot_QueueError(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	_, err := Boot(context.Background(), config.Config{
		DB:    config.DBConfig{DSN: "x"},
		Queue: config.QueueConfig{Enabled: true, URL: "nats://127.0.0.1:1", ConnectTimeout: 50 * time.Millisecond},
	}, testLogger())
	assert.Error(t, err)
}

func TestApp_Run(t *testing.T) {
	sup := worker.New(nil)
	sup.Register(fakeWorker{name: "noop", run: func(context.Context) error { return nil }})

	app := &App{Supervisor: sup}
	assert.NoError(t, app.Run(context.Background()))
}

func TestRequestMeta(t *testing.T) {
	assert.Nil(t, requestMeta(context.Background()))

	ctx := reqid.WithContext(context.Background(), "r1")
	assert.Equal(t, map[string]string{"request_id": "r1"}, requestMeta(ctx))
}

func TestMigrationLockName(t *testing.T) {
	assert.Equal(t, "migrate:t_", MigrationLockName("t_"))
}

func TestMigratorDSN(t *testing.T) {
	assert.Equal(t, "md", migratorDSN(config.DBConfig{DSN: "d", Migrator: config.MigratorConfig{DSN: "md"}}))
	assert.Equal(t, "d", migratorDSN(config.DBConfig{DSN: "d"}))
}

func TestDBConfig(t *testing.T) {
	out := dbConfig(config.DBConfig{
		DSN:          "d",
		Schema:       "s",
		TablePrefix:  "t",
		Role:         "r",
		MaxOpenConns: 5,
		Migrator:     config.MigratorConfig{DSN: "md", Role: "mr"},
		Reader:       config.ReaderConfig{DSN: "rd", Role: "rr", MaxOpenConns: 3},
	})
	assert.Equal(t, "d", out.DSN)
	assert.Equal(t, "t", out.TablePrefix)
	assert.Equal(t, 5, out.MaxOpenConns)
	assert.Equal(t, "md", out.Migrator.DSN)
	assert.Equal(t, "rd", out.Reader.DSN)
	assert.Equal(t, 3, out.Reader.MaxOpenConns)
}

func TestTelemetryConfig(t *testing.T) {
	out := telemetryConfig(config.TelemetryConfig{
		OTLP:    config.OTLPConfig{Endpoint: "e", Protocol: "grpc", Insecure: true},
		Tracing: config.TracingConfig{Enabled: true, SampleRatio: 0.5},
		Metrics: config.MetricsConfig{Enabled: true, Prometheus: config.PrometheusConfig{Enabled: true, Addr: ":1", Path: "/m"}},
	})
	assert.Equal(t, "e", out.OTLP.Endpoint)
	assert.Equal(t, "grpc", out.OTLP.Protocol)
	assert.True(t, out.OTLP.Insecure)
	assert.True(t, out.Tracing.Enabled)
	assert.InDelta(t, 0.5, out.Tracing.SampleRatio, 0.0001)
	assert.Equal(t, ":1", out.Metrics.Prometheus.Addr)
	assert.Equal(t, "/m", out.Metrics.Prometheus.Path)
}

func TestBuildMCPServer(t *testing.T) {
	svcs := &Services{Orgs: org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, nil)}

	assert.Nil(t, buildMCPServer(config.MCPConfig{}, svcs, nil))
	srv := buildMCPServer(config.MCPConfig{Enabled: true}, svcs, nil)
	require.NotNil(t, srv)
	assert.NotNil(t, mcpHTTPHandler(srv))
	assert.Nil(t, mcpHTTPHandler(nil))
}

func TestBuildMCPServer_AppsUI(t *testing.T) {
	svcs := &Services{Orgs: org.NewService(&fakes.OrgStore{}, fakes.Transactor{}, nil)}
	srv := buildMCPServer(config.MCPConfig{Enabled: true, AppsUI: true}, svcs, nil)
	require.NotNil(t, srv)

	assert.NotNil(t, srv.SDK()) // seals the registry and resolves the tool -> UI resource link
}

func TestApp_RunMCP_Disabled(t *testing.T) {
	assert.Error(t, (&App{}).RunMCP(context.Background()))
}

func workerNames(app *App) []string {
	var names []string
	for _, w := range app.Supervisor.Workers() {
		names = append(names, w.Name())
	}
	return names
}

func TestBoot_MailerError(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, db.DBConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	_, err := Boot(context.Background(), config.Config{
		DB:   config.DBConfig{DSN: "x"},
		Mail: config.MailConfig{Driver: config.MailDriverSMTP},
	}, testLogger())
	assert.ErrorContains(t, err, "mail")
}
