package boot

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/capabilities"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/queue"
	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/org"
	"altalune.id/oagent/internal/testutil/fakes"
	"altalune.id/oagent/migrator"
	"altalune.id/oagent/pg"
	"altalune.id/oagent/reqid"
	"altalune.id/oagent/telemetry"
	"altalune.id/oagent/worker"
)

func restoreSeams(t *testing.T) {
	t.Helper()
	s, p, o, m, u, l := setupTelemetry, openPool, openPostgres, openMigrator, migrateUp, withLock
	bq, or, on := buildQueueFn, openRedisFn, openNATSFn
	t.Cleanup(func() {
		setupTelemetry, openPool, openPostgres, openMigrator, migrateUp, withLock = s, p, o, m, u, l
		buildQueueFn, openRedisFn, openNATSFn = bq, or, on
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
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	openMigrator = func(context.Context, string, migrator.Config) (*sql.DB, error) { return migDB, nil }
	migrateUp = func(context.Context, *sql.DB, migrator.Config) error { return nil }
	withLock = passthroughLock

	app, err := Boot(context.Background(), config.Config{DB: config.DBConfig{AutoMigrate: true}, Postgres: config.PostgresConfig{DSN: "x", Schema: "public"}}, nil)
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
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	app, err := Boot(context.Background(), config.Config{
		Postgres:  config.PostgresConfig{DSN: "x"},
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
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	app, err := Boot(context.Background(),
		config.Config{Postgres: config.PostgresConfig{DSN: "x"}, Scheduler: config.SchedulerConfig{Enabled: true}},
		testLogger(), WithScheduler(false))
	require.NoError(t, err)
	assert.Len(t, app.Supervisor.Workers(), 5) // http + db-health + outbox + mailqueue + apikey-usage
	require.NoError(t, app.Close(context.Background()))
}

func TestBoot_SchedulerOnlyDisabled(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	_, err := Boot(context.Background(),
		config.Config{Postgres: config.PostgresConfig{DSN: "x"}}, testLogger(), WithSchedulerOnly(true))
	assert.Error(t, err)
}

func TestBoot_ConsumerOnlyDisabled(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	_, err := Boot(context.Background(),
		config.Config{Postgres: config.PostgresConfig{DSN: "x"}}, testLogger(), WithConsumerOnly(true))
	assert.Error(t, err)
}

func TestBoot_ConsumerEnabled(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	stubNATS(t)
	stubQueue(fakeQueue{})

	app, err := Boot(context.Background(), natsFeatureConfig(), testLogger())
	require.NoError(t, err)
	assert.Len(t, app.Supervisor.Workers(), 8) // http + db-health + nats-health + outbox + mailqueue + apikey-usage + consumer + listener
	assert.Contains(t, workerNames(app), "nats-health")
	assert.Contains(t, workerNames(app), "fake-consumer")
	assert.Contains(t, workerNames(app), "fake-listener")
	require.NoError(t, app.Close(context.Background()))
}

func TestBoot_ConsumerError(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	stubNATS(t)
	stubQueue(fakeQueue{consumerErr: errors.New("boom")})

	_, err := Boot(context.Background(), natsFeatureConfig(), testLogger())
	assert.ErrorContains(t, err, "boom")
}

func TestBoot_ListenerError(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	stubNATS(t)
	stubQueue(fakeQueue{listenerErr: errors.New("boom")})

	_, err := Boot(context.Background(), natsFeatureConfig(), testLogger())
	assert.ErrorContains(t, err, "boom")
}

func TestBoot_NoAutoMigrate(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	require.NoError(t, poolDB.Close()) // closed since App isn't closed here

	migratorCalled := false
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	openMigrator = func(context.Context, string, migrator.Config) (*sql.DB, error) {
		migratorCalled = true
		return nil, errors.New("should not be called")
	}

	app, err := Boot(context.Background(), config.Config{DB: config.DBConfig{AutoMigrate: false}, Postgres: config.PostgresConfig{DSN: "x"}}, nil)
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
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{}, errors.New("boom")
	}

	_, err := Boot(context.Background(), config.Config{Postgres: config.PostgresConfig{DSN: "x"}}, nil)
	assert.Error(t, err)
	assert.True(t, called)
}

func TestBoot_MigratorError(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)

	called := false
	setupTelemetry = stubTelemetry(&called, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	openMigrator = func(context.Context, string, migrator.Config) (*sql.DB, error) { return nil, errors.New("boom") }

	_, err := Boot(context.Background(), config.Config{DB: config.DBConfig{AutoMigrate: true}, Postgres: config.PostgresConfig{DSN: "x"}}, nil)
	assert.Error(t, err)
	assert.True(t, called)
}

func TestBoot_MigrateError(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	migDB, _ := newDB(t)

	called := false
	setupTelemetry = stubTelemetry(&called, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	openMigrator = func(context.Context, string, migrator.Config) (*sql.DB, error) { return migDB, nil }
	migrateUp = func(context.Context, *sql.DB, migrator.Config) error { return errors.New("boom") }
	withLock = passthroughLock

	_, err := Boot(context.Background(), config.Config{DB: config.DBConfig{AutoMigrate: true}, Postgres: config.PostgresConfig{DSN: "x"}}, nil)
	assert.Error(t, err)
	assert.True(t, called)
}

func TestBoot_LockError(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	migDB, _ := newDB(t)

	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	openMigrator = func(context.Context, string, migrator.Config) (*sql.DB, error) { return migDB, nil }
	withLock = func(context.Context, *sql.DB, string, func(context.Context) error) error { return errors.New("boom") }

	_, err := Boot(context.Background(), config.Config{DB: config.DBConfig{AutoMigrate: true}, Postgres: config.PostgresConfig{DSN: "x"}}, nil)
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

type fakeQueue struct {
	queue.Queue
	consumerErr, listenerErr error
}

func (f fakeQueue) Consumer(context.Context, []queue.Handler) (worker.Worker, error) {
	if f.consumerErr != nil {
		return nil, f.consumerErr
	}
	return fakeWorker{name: "fake-consumer", run: func(context.Context) error { return nil }}, nil
}

func (f fakeQueue) Listener(context.Context, []queue.Listener) (worker.Worker, error) {
	if f.listenerErr != nil {
		return nil, f.listenerErr
	}
	return fakeWorker{name: "fake-listener", run: func(context.Context) error { return nil }}, nil
}

func stubQueue(q queue.Queue) {
	buildQueueFn = func(context.Context, kernel.Kernel, config.Config, *nats.Conn, *slog.Logger) (queue.Queue, error) {
		return q, nil
	}
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
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	_, nc := stubNATS(t)
	buildQueueFn = func(context.Context, kernel.Kernel, config.Config, *nats.Conn, *slog.Logger) (queue.Queue, error) {
		return nil, errors.New("boom")
	}

	_, err := Boot(context.Background(), natsFeatureConfig(), testLogger())
	assert.ErrorContains(t, err, "boom")
	assert.True(t, nc.IsClosed(), "a queue failure closes the connection boot opened")
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
	assert.Equal(t, "md", migratorDSN(config.PostgresConfig{DSN: "d", Migrator: config.PostgresMigratorConfig{DSN: "md"}}))
	assert.Equal(t, "d", migratorDSN(config.PostgresConfig{DSN: "d"}))
}

func TestWriterAndReaderConfig(t *testing.T) {
	c := config.PostgresConfig{
		DSN:            "d",
		Role:           "r",
		MaxOpenConns:   5,
		ConnectTimeout: time.Second,
		ConnectBackoff: time.Millisecond,
		Migrator:       config.PostgresMigratorConfig{DSN: "md", Role: "mr"},
		Reader:         config.PostgresReaderConfig{DSN: "rd", Role: "rr", MaxOpenConns: 3},
	}
	w := writerConfig(c)
	assert.Equal(t, pg.Config{DSN: "d", Role: "r", MaxOpenConns: 5, ConnectTimeout: time.Second, ConnectBackoff: time.Millisecond}, w)
	r := readerConfig(c)
	assert.Equal(t, pg.Config{DSN: "rd", Role: "rr", MaxOpenConns: 3, ConnectTimeout: time.Second, ConnectBackoff: time.Millisecond}, r)
}

func TestOpenPostgresPool_WithReader(t *testing.T) {
	restoreSeams(t)
	writer, writerMock, err := sqlmock.New()
	require.NoError(t, err)
	writerMock.ExpectClose()
	reader, readerMock, err := sqlmock.New()
	require.NoError(t, err)
	readerMock.ExpectClose()
	var dsns []string
	openPostgres = func(_ context.Context, c pg.Config, _ *slog.Logger) (*sql.DB, error) {
		dsns = append(dsns, c.DSN)
		if c.DSN == "rd" {
			return reader, nil
		}
		return writer, nil
	}

	p, err := openPostgresPool(context.Background(), config.PostgresConfig{DSN: "d", Reader: config.PostgresReaderConfig{DSN: "rd"}}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"d", "rd"}, dsns)
	assert.Same(t, writer, p.W)
	assert.Same(t, reader, p.R)
	assert.NoError(t, p.Close())
}

func TestOpenPostgresPool_WithoutReaderAliasesTheWriter(t *testing.T) {
	restoreSeams(t)
	writer, _ := newDB(t)
	openPostgres = func(context.Context, pg.Config, *slog.Logger) (*sql.DB, error) { return writer, nil }

	p, err := openPostgresPool(context.Background(), config.PostgresConfig{DSN: "d"}, nil)
	require.NoError(t, err)
	assert.Same(t, p.W, p.R)
	assert.NoError(t, p.Close())
}

func TestOpenPostgresPool_WriterError(t *testing.T) {
	restoreSeams(t)
	openPostgres = func(context.Context, pg.Config, *slog.Logger) (*sql.DB, error) { return nil, errors.New("down") }

	_, err := openPostgresPool(context.Background(), config.PostgresConfig{DSN: "d"}, nil)
	assert.Error(t, err)
}

func TestOpenPostgresPool_ReaderErrorClosesTheWriter(t *testing.T) {
	restoreSeams(t)
	writer, writerMock := newDB(t)
	openPostgres = func(_ context.Context, c pg.Config, _ *slog.Logger) (*sql.DB, error) {
		if c.DSN == "rd" {
			return nil, errors.New("reader down")
		}
		return writer, nil
	}

	_, err := openPostgresPool(context.Background(), config.PostgresConfig{DSN: "d", Reader: config.PostgresReaderConfig{DSN: "rd"}}, nil)
	require.Error(t, err)
	assert.NoError(t, writerMock.ExpectationsWereMet(), "the writer is closed when the reader fails")
}

func TestOpenDB_UnknownDriver(t *testing.T) {
	_, err := openDB(context.Background(), config.Config{DB: config.DBConfig{Driver: "sqlite"}}, nil)
	assert.ErrorContains(t, err, "db.driver")
}

func TestOpenPostgresPool_NeverLeaksAPassword(t *testing.T) {
	const writerPW, readerPW = "writer-pw-do-not-print", "reader-pw-do-not-print"
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	for name, c := range map[string]config.PostgresConfig{
		"writer": {DSN: "postgres://app:" + writerPW + "@127.0.0.1:1/db?sslmode=disable&connect_timeout=1"},
		"reader": {
			DSN:    "postgres://app:" + writerPW + "@127.0.0.1:1/db?sslmode=disable&connect_timeout=1",
			Reader: config.PostgresReaderConfig{DSN: "postgres://ro:" + readerPW + "@127.0.0.1:1/db?sslmode=disable"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			restoreSeams(t)
			c.ConnectTimeout, c.ConnectBackoff = 200*time.Millisecond, 50*time.Millisecond
			if name == "reader" {
				writer, _ := newDB(t)
				openPostgres = func(ctx context.Context, pc pg.Config, l *slog.Logger) (*sql.DB, error) {
					if pc.DSN == c.DSN {
						return writer, nil
					}
					return pg.Open(ctx, pc, l)
				}
			}
			_, err := openPostgresPool(context.Background(), c, log)
			require.Error(t, err)
			for _, pw := range []string{writerPW, readerPW} {
				assert.NotContains(t, err.Error(), pw)
				assert.NotContains(t, logs.String(), pw)
			}
		})
	}
}

func TestMigrate_NeverLeaksTheMigratorPassword(t *testing.T) {
	restoreSeams(t)
	const pw = "migrator-pw-do-not-print"
	err := migrate(context.Background(), config.PostgresConfig{
		DSN:      "postgres://app:app-pw@127.0.0.1:1/db?sslmode=disable",
		Migrator: config.PostgresMigratorConfig{DSN: "postgres://owner:" + pw + "@127.0.0.1:1/db?sslmode=disable&connect_timeout=1"},
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), pw)
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
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	_, err := Boot(context.Background(), config.Config{
		Postgres: config.PostgresConfig{DSN: "x"},
		Mail:     config.MailConfig{Driver: config.MailDriverSMTP},
	}, testLogger())
	assert.ErrorContains(t, err, "mail")
}

func TestCapabilitiesConfig(t *testing.T) {
	assert.Equal(t, capabilities.Config{Selfhosted: true, EnvGenesis: true}, capabilitiesConfig(config.Config{
		Mode: config.ModeSelfhosted,
		Auth: config.AuthConfig{Mode: config.AuthModeLocal, Genesis: config.GenesisConfig{Email: "root@example.com"}},
	}))
	assert.Equal(t, capabilities.Config{Selfhosted: true, BasicAuth: true}, capabilitiesConfig(config.Config{
		Mode: config.ModeSelfhosted, Auth: config.AuthConfig{Mode: config.AuthModeBasic},
	}))
	assert.Equal(t, capabilities.Config{Cloud: true}, capabilitiesConfig(config.Config{Mode: config.ModeCloud}))
}
