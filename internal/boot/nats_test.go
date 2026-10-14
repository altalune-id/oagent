package boot

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/testutil/natstest"
	natsconn "altalune.id/oagent/nats"
)

const (
	natsTestPassword = "hunter2-nats-secret"
	natsTestToken    = "s3cr3t-nats-token"
)

func natsTestConfig(url string) config.NATSConfig {
	return config.NATSConfig{URL: url, Name: "oagent", ConnectTimeout: 300 * time.Millisecond}
}

func natsFeatureConfig() config.Config {
	return config.Config{
		Postgres: config.PostgresConfig{DSN: "x"},
		Queue:    config.QueueConfig{Enabled: true, Driver: config.QueueDriverNATS},
		NATS:     natsTestConfig("nats://oagent:" + natsTestPassword + "@127.0.0.1:1"),
	}
}

func TestNATSConfig(t *testing.T) {
	got := natsConfig(config.NATSConfig{
		URL: "nats://h:4222", Token: "t", User: "u", Password: "p", Name: "n", ConnectTimeout: time.Second,
	})
	assert.Equal(t, natsconn.Config{
		URL: "nats://h:4222", Token: "t", User: "u", Password: "p", Name: "n", ConnectTimeout: time.Second,
	}, got)
}

func stubNATS(t *testing.T) (calls *int, nc *nats.Conn) {
	t.Helper()
	ns := natstest.Start(t)
	nc, err := nats.Connect(ns.ClientURL())
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	calls = new(int)
	openNATSFn = func(context.Context, natsconn.Config, *slog.Logger) (*nats.Conn, error) {
		*calls++
		return nc, nil
	}
	return calls, nc
}

func assertNoSecrets(t *testing.T, s string) {
	t.Helper()
	assert.NotContains(t, s, natsTestPassword)
	assert.NotContains(t, s, natsTestToken)
}

// Review Focus 1: with nothing selecting NATS, boot never dials it, even when nats.url is set.
func TestBuildNATS_UnusedNeverConnects(t *testing.T) {
	restoreSeams(t)
	calls, _ := stubNATS(t)
	log, logs := bufferLogger()

	k := &kernel.Kernel{}
	nc, nh, err := buildNATS(t.Context(), k, config.Config{}, log)
	require.NoError(t, err)
	assert.Nil(t, nc)
	assert.Nil(t, nh)
	assert.Zero(t, *calls)
	assert.Empty(t, logs.String(), "no url, nothing to say")

	cfg := config.Config{Queue: config.QueueConfig{Driver: config.QueueDriverNATS}, NATS: natsTestConfig("nats://oagent:" + natsTestPassword + "@nats.internal:4222")}
	nc, nh, err = buildNATS(t.Context(), k, cfg, log)
	require.NoError(t, err)
	assert.Nil(t, nc, "the nats driver with the queue off needs no connection")
	assert.Nil(t, nh)
	assert.Zero(t, *calls)
	assert.Contains(t, logs.String(), "no feature uses NATS")
	assertNoSecrets(t, logs.String())
	assert.NotContains(t, logs.String(), "nats.internal")
}

func TestBuildNATS_FeatureConnectsOnceAndClosesWithTheKernel(t *testing.T) {
	restoreSeams(t)
	calls, stub := stubNATS(t)

	k := &kernel.Kernel{}
	nc, nh, err := buildNATS(t.Context(), k, natsFeatureConfig(), testLogger())
	require.NoError(t, err)
	require.NotNil(t, nh)
	assert.Same(t, stub, nc)
	assert.Equal(t, 1, *calls)
	assert.True(t, nh.Ready(), "the boot connect succeeded")
	assert.Equal(t, "nats-health", nh.Name())

	require.NoError(t, k.Close())
	assert.True(t, stub.IsClosed(), "the kernel drains and closes the connection")
}

func TestBuildNATS_ConnectErrorFailsBoot(t *testing.T) {
	restoreSeams(t)
	openNATSFn = func(context.Context, natsconn.Config, *slog.Logger) (*nats.Conn, error) {
		return nil, errors.New("nats: cannot connect to 127.0.0.1:1 (nats.url): refused")
	}
	_, _, err := buildNATS(t.Context(), &kernel.Kernel{}, natsFeatureConfig(), testLogger())
	assert.ErrorContains(t, err, "boot: nats: cannot connect")
}

// Review Focus 1: the default config never dials NATS and adds no NATS readiness worker.
func TestBoot_DefaultConfigNeverOpensNATS(t *testing.T) {
	restoreSeams(t)
	calls, _ := stubNATS(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	app, err := Boot(context.Background(), config.Config{Postgres: config.PostgresConfig{DSN: "x"}}, testLogger())
	require.NoError(t, err)
	assert.Zero(t, *calls)
	assert.NotContains(t, workerNames(app), "nats-health")
	require.NoError(t, app.Close(context.Background()))
}

func TestBoot_LaterFailureClosesNATS(t *testing.T) {
	restoreSeams(t)
	calls, nc := stubNATS(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}

	cfg := natsFeatureConfig()
	cfg.Mail = config.MailConfig{Driver: config.MailDriverSMTP}
	_, err := Boot(context.Background(), cfg, testLogger())
	require.Error(t, err)
	assert.Equal(t, 1, *calls)
	assert.True(t, nc.IsClosed(), "a failed boot does not leak the connection")
}

func TestBoot_NATSConnectErrorFailsBoot(t *testing.T) {
	restoreSeams(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	_, err := Boot(context.Background(), natsFeatureConfig(), testLogger())
	require.Error(t, err)
	assert.ErrorContains(t, err, "nats: cannot connect to 127.0.0.1:1 (nats.url)")
	assertNoSecrets(t, err.Error())
}
