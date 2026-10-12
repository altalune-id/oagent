//go:build integration

// Package pgtest gives integration tests a real Postgres: TEST_PG_DSN when set, otherwise one testcontainers instance per test binary.
package pgtest

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/moby/moby/client"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"altalune.id/oagent/migrator"
	"altalune.id/oagent/nanoid"
	"altalune.id/oagent/schema"
)

const (
	envDSN       = "TEST_PG_DSN"
	envCI        = "CI"
	image        = "postgres:17-alpine"
	labelOwner   = "id.altalune.oagent.pgtest"
	staleAfter   = 30 * time.Minute
	schemaPrefix = "pgt_"
	schemaLen    = 16
)

//nolint:gochecknoglobals // one server per test binary; every test gets its own schema on it.
var server struct {
	once sync.Once
	dsn  string
	err  error
}

// Handle is one test's private schema on the shared Postgres server.
type Handle struct {
	DSN    string
	Schema string
}

// New returns a handle with a fresh schema; without TEST_PG_DSN it skips if no container starts, and fails under CI.
func New(t *testing.T) *Handle {
	t.Helper()
	dsn := os.Getenv(envDSN)
	if dsn == "" {
		server.once.Do(func() { server.dsn, server.err = startContainer(t) })
		if server.err != nil {
			if os.Getenv(envCI) != "" {
				t.Fatalf("pgtest: no Postgres under CI: set %s or provide a container runtime: %v", envDSN, server.err)
			}
			t.Skipf("pgtest: cannot start Postgres (set %s or start docker/podman): %v", envDSN, server.err)
		}
		dsn = server.dsn
	}
	return &Handle{DSN: dsn, Schema: uniqueSchema(t)}
}

// OpenDB creates the handle's schema, pins search_path to it and drops it when the test ends.
func (h *Handle) OpenDB(t *testing.T) *sql.DB {
	t.Helper()
	connCfg, err := pgx.ParseConfig(h.DSN)
	if err != nil {
		t.Fatalf("pgtest: parse DSN: %v", err)
	}
	connCfg.RuntimeParams["search_path"] = h.Schema

	sqlDB := stdlib.OpenDB(*connCfg)
	if err := sqlDB.PingContext(t.Context()); err != nil {
		_ = sqlDB.Close()
		t.Fatalf("pgtest: ping: %v", err)
	}
	quoted := pgx.Identifier{h.Schema}.Sanitize()
	if _, err := sqlDB.ExecContext(t.Context(), "CREATE SCHEMA IF NOT EXISTS "+quoted); err != nil {
		_ = sqlDB.Close()
		t.Fatalf("pgtest: create schema %s: %v", h.Schema, err)
	}
	t.Cleanup(func() {
		if _, err := sqlDB.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+quoted+" CASCADE"); err != nil {
			t.Errorf("pgtest: leaked schema %s: %v", h.Schema, err)
		}
		_ = sqlDB.Close()
	})
	return sqlDB
}

// Migrated returns a database whose own schema has every oagent migration applied, and that schema's name.
func Migrated(t *testing.T) (sqlDB *sql.DB, schemaName string) {
	t.Helper()
	h := New(t)
	sqlDB = h.OpenDB(t)
	if err := schema.MigrateUp(t.Context(), sqlDB, migrator.Config{Schema: h.Schema}); err != nil {
		t.Fatalf("pgtest: migrate: %v", err)
	}
	return sqlDB, h.Schema
}

// NOTE: Ryuk reaps the container when the binary exits; with Ryuk off (podman) the label sweep removes it on a later run.
func startContainer(t *testing.T) (string, error) {
	t.Helper()
	sweepStale(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := postgres.Run(ctx, image,
		postgres.WithDatabase("oagent_test"),
		postgres.WithUsername("oagent"),
		postgres.WithPassword("oagent"),
		testcontainers.WithLabels(map[string]string{labelOwner: "true"}),
		// NOTE: Docker Desktop proxies the port after the log line; without the port wait the first connect can fail.
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(time.Minute),
			wait.ForListeningPort("5432/tcp").WithStartupTimeout(time.Minute),
		),
	)
	if err != nil {
		return "", fmt.Errorf("start %s: %w", image, err)
	}
	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = c.Terminate(context.Background())
		return "", fmt.Errorf("connection string: %w", err)
	}
	return dsn, nil
}

func sweepStale(t *testing.T) {
	t.Helper()
	provider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return
	}
	defer func() { _ = provider.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cli := provider.Client()
	found, err := cli.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: make(client.Filters).Add("label", labelOwner+"=true"),
	})
	if err != nil {
		t.Logf("pgtest: sweep: list: %v", err)
		return
	}
	cutoff := time.Now().Add(-staleAfter).Unix()
	for _, ctr := range found.Items {
		if ctr.Created > cutoff {
			continue
		}
		if _, err := cli.ContainerRemove(ctx, ctr.ID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}); err != nil {
			t.Logf("pgtest: sweep: remove %s: %v", ctr.ID, err)
		}
	}
}

func uniqueSchema(t *testing.T) string {
	t.Helper()
	id, err := nanoid.New(schemaLen)
	if err != nil {
		t.Fatalf("pgtest: nanoid: %v", err)
	}
	return schemaPrefix + strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(id))
}
