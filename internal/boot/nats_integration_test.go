//go:build integration

package boot

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/internal/kernel/db"
	natsqueue "altalune.id/oagent/internal/kernel/queue/nats"
	"altalune.id/oagent/internal/testutil/natstest"
)

func TestBoot_QueueOnNATSAccount(t *testing.T) {
	restoreSeams(t)
	ns := natstest.StartAccounts(t)
	poolDB, _ := newDB(t)
	setupTelemetry = stubTelemetry(nil, nil)
	openPool = func(context.Context, config.PostgresConfig, *slog.Logger) (db.Pool, error) {
		return db.Pool{W: poolDB, R: poolDB}, nil
	}
	cfg := natsFeatureConfig()
	cfg.NATS = natsTestConfig(ns.ClientURL())
	cfg.NATS.User, cfg.NATS.Password = natstest.UserA, natstest.PasswordA

	app, err := Boot(t.Context(), cfg, testLogger())
	require.NoError(t, err)
	assert.IsType(t, &natsqueue.Client{}, app.Kernel.Queue)
	assert.Contains(t, workerNames(app), "nats-health")

	other, err := nats.Connect(ns.ClientURL(), nats.UserInfo(natstest.UserB, natstest.PasswordB))
	require.NoError(t, err)
	t.Cleanup(other.Close)
	js, err := jetstream.New(other)
	require.NoError(t, err)
	_, err = js.Stream(t.Context(), "WORK")
	require.ErrorIs(t, err, jetstream.ErrStreamNotFound, "oagent's streams live in its own account")

	require.Equal(t, 2, ns.NumClients())
	require.NoError(t, app.Close(context.Background()))
	assert.Eventually(t, func() bool { return ns.NumClients() == 1 }, 5*time.Second, 10*time.Millisecond, "closing the app drains oagent's connection")
}
