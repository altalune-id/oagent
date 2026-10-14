// Package natstest starts an in-process NATS JetStream server for tests.
package natstest

import (
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/require"
)

// Users of the two accounts StartAccounts creates; each account has the same JetStream limits.
const (
	UserA     = "app_a"
	PasswordA = "app-a-password-0001"
	UserB     = "app_b"
	PasswordB = "app-b-password-0002"
)

// Start runs a server without accounts or auth until the test ends.
func Start(t testing.TB) *server.Server {
	t.Helper()
	return run(t, &server.Options{Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: t.TempDir()})
}

// StartAccounts runs a server with accounts APP_A and APP_B, like the shared Altalune NATS, until the test ends.
func StartAccounts(t testing.TB) *server.Server {
	t.Helper()
	opts := &server.Options{}
	require.NoError(t, opts.ProcessConfigString(fmt.Sprintf(`
listen: "127.0.0.1:-1"
jetstream { store_dir: %q }
accounts {
  APP_A { jetstream { max_file: 1G, max_mem: 16M }, users: [{ user: %s, password: %s }] }
  APP_B { jetstream { max_file: 1G, max_mem: 16M }, users: [{ user: %s, password: %s }] }
}
`, t.TempDir(), UserA, PasswordA, UserB, PasswordB)))
	return run(t, opts)
}

func run(t testing.TB, opts *server.Options) *server.Server {
	t.Helper()
	opts.NoLog, opts.NoSigs = true, true
	ns, err := server.NewServer(opts)
	require.NoError(t, err)
	go ns.Start()
	require.True(t, ns.ReadyForConnections(5*time.Second))
	t.Cleanup(ns.Shutdown)
	return ns
}
