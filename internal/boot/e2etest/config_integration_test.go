//go:build integration

package e2etest

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"altalune.id/oagent/config"
	"altalune.id/oagent/sealer"
)

const testKeyHex = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

func devLocalConfig() config.Config {
	return config.Config{
		Mode:     config.ModeSelfhosted,
		Env:      config.EnvDevelopment,
		HTTP:     config.HTTPConfig{Addr: ":5150"},
		Postgres: config.PostgresConfig{Schema: "public"},
		Auth:     config.AuthConfig{Mode: config.AuthModeLocal},
	}
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func testSealer(t *testing.T) sealer.Sealer {
	t.Helper()
	key, err := sealer.GenerateKey()
	require.NoError(t, err)
	sl, err := sealer.New(key)
	require.NoError(t, err)
	return sl
}
