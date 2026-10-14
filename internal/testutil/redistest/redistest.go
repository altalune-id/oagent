//go:build integration

// Package redistest gives integration tests a real Redis: TEST_REDIS_URL when set, otherwise one testcontainers instance per test binary.
package redistest

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/client"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"altalune.id/oagent/nanoid"
)

const (
	envURL     = "TEST_REDIS_URL"
	envCI      = "CI"
	image      = "redis:7-alpine"
	labelOwner = "id.altalune.oagent.redistest"
	staleAfter = 30 * time.Minute
	prefixLen  = 12
)

//nolint:gochecknoglobals // one server per test binary; every test gets its own key prefix on it.
var server struct {
	once sync.Once
	url  string
	err  error
}

// Handle is one test's key prefix on the shared Redis server.
type Handle struct {
	URL    string
	Prefix string
}

// New returns a handle with a fresh key prefix; without TEST_REDIS_URL it skips if no container starts, and fails under CI.
func New(t *testing.T) *Handle {
	t.Helper()
	url := os.Getenv(envURL)
	if url == "" {
		server.once.Do(func() { server.url, server.err = startContainer(t) })
		if server.err != nil {
			if os.Getenv(envCI) != "" {
				t.Fatalf("redistest: no Redis under CI: set %s or provide a container runtime: %v", envURL, server.err)
			}
			t.Skipf("redistest: cannot start Redis (set %s or start docker/podman): %v", envURL, server.err)
		}
		url = server.url
	}
	return &Handle{URL: url, Prefix: uniquePrefix(t)}
}

// Client opens a client on the handle's server, and deletes the handle's keys and closes the client when the test ends.
func (h *Handle) Client(t *testing.T) *redis.Client {
	t.Helper()
	opts, err := redis.ParseURL(h.URL)
	if err != nil {
		t.Fatalf("redistest: parse URL: %v", err)
	}
	c := redis.NewClient(opts)
	if err := c.Ping(t.Context()).Err(); err != nil {
		_ = c.Close()
		t.Fatalf("redistest: ping: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		var keys []string
		iter := c.Scan(ctx, 0, h.Prefix+"*", 0).Iterator()
		for iter.Next(ctx) {
			keys = append(keys, iter.Val())
		}
		if len(keys) > 0 {
			if err := c.Del(ctx, keys...).Err(); err != nil {
				t.Errorf("redistest: leaked keys under %s: %v", h.Prefix, err)
			}
		}
		_ = c.Close()
	})
	return c
}

// NOTE: Ryuk reaps the container when the binary exits; with Ryuk off (podman) the label sweep removes it on a later run.
func startContainer(t *testing.T) (string, error) {
	t.Helper()
	sweepStale(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := testcontainers.Run(ctx, image,
		testcontainers.WithExposedPorts("6379/tcp"),
		testcontainers.WithLabels(map[string]string{labelOwner: "true"}),
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").WithStartupTimeout(time.Minute),
			wait.ForListeningPort("6379/tcp").WithStartupTimeout(time.Minute),
		),
	)
	if err != nil {
		return "", fmt.Errorf("start %s: %w", image, err)
	}
	endpoint, err := c.PortEndpoint(ctx, "6379/tcp", "redis")
	if err != nil {
		_ = c.Terminate(context.Background())
		return "", fmt.Errorf("endpoint: %w", err)
	}
	return endpoint + "/0", nil
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
		t.Logf("redistest: sweep: list: %v", err)
		return
	}
	cutoff := time.Now().Add(-staleAfter).Unix()
	for _, ctr := range found.Items {
		if ctr.Created > cutoff {
			continue
		}
		if _, err := cli.ContainerRemove(ctx, ctr.ID, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}); err != nil {
			t.Logf("redistest: sweep: remove %s: %v", ctr.ID, err)
		}
	}
}

func uniquePrefix(t *testing.T) string {
	t.Helper()
	id, err := nanoid.New(prefixLen)
	if err != nil {
		t.Fatalf("redistest: nanoid: %v", err)
	}
	return "oagenttest:" + strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(id)) + ":"
}
