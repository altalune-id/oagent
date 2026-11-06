//go:build integration

package runtimetest

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	envImage     = "OAGENT_RUNTIME_IMAGE"
	envCI        = "CI"
	runtimePort  = "5160/tcp"
	hostAlias    = "host.testcontainers.internal"
	tokenLength  = 48
	startTimeout = 3 * time.Minute
)

// Runtime is the runtime container behind a local proxy whose URL survives a kill and a restart.
type Runtime struct {
	t         *testing.T
	token     string
	hostPorts []int
	proxy     net.Listener

	mu        sync.Mutex
	target    string
	container testcontainers.Container
	conns     map[net.Conn]struct{}
}

// StartRuntime starts the runtime container wired to the fake model; it skips without a container runtime, fails under CI.
func StartRuntime(t *testing.T, model *FakeModel, mcps ...*FakeMCP) *Runtime {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("runtimetest: proxy listen: %v", err)
	}
	ports := make([]int, 0, 1+len(mcps))
	ports = append(ports, model.Port())
	for _, m := range mcps {
		ports = append(ports, m.Port())
	}
	r := &Runtime{t: t, token: strings.Repeat("t", tokenLength), hostPorts: ports, proxy: ln, conns: map[net.Conn]struct{}{}}
	t.Cleanup(func() {
		_ = ln.Close()
		r.Kill()
	})
	go r.accept()
	if err := r.start(); err != nil {
		if os.Getenv(envCI) != "" {
			t.Fatalf("runtimetest: runtime container: %v", err)
		}
		t.Skipf("runtimetest: cannot start the runtime container (set %s or start a container runtime): %v", envImage, err)
	}
	return r
}

// URL is the stable runtime URL.
func (r *Runtime) URL() string { return "http://" + r.proxy.Addr().String() }

// Token is the runtime bearer.
func (r *Runtime) Token() string { return r.token }

// ModelBaseURL is the fake model's base URL as the container sees it.
func (r *Runtime) ModelBaseURL() string { return r.HostURL(r.hostPorts[0], "/v1") }

// HostURL is the URL of a host server on port, as the container sees it.
func (r *Runtime) HostURL(port int, path string) string {
	return fmt.Sprintf("http://%s:%d%s", hostAlias, port, path)
}

// Kill terminates the container and drops every proxied connection, as a crash would.
func (r *Runtime) Kill() {
	r.mu.Lock()
	c := r.container
	r.container, r.target = nil, ""
	for conn := range r.conns {
		_ = conn.Close()
	}
	r.conns = map[net.Conn]struct{}{}
	r.mu.Unlock()
	if c != nil {
		_ = c.Terminate(context.Background())
	}
}

// Restart starts a fresh container behind the same URL.
func (r *Runtime) Restart() {
	r.t.Helper()
	if err := r.start(); err != nil {
		r.t.Fatalf("runtimetest: restart runtime: %v", err)
	}
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

func (r *Runtime) start() error {
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()
	allow := make([]string, 0, len(r.hostPorts))
	for _, p := range r.hostPorts {
		allow = append(allow, fmt.Sprintf("%s:%d", hostAlias, p))
	}
	req := testcontainers.ContainerRequest{
		ExposedPorts: []string{runtimePort},
		Env: map[string]string{
			"RUNTIME_TOKEN":         r.token,
			"RUNTIME_ENV":           "development",
			"RUNTIME_ALLOW_PRIVATE": strings.Join(allow, ","),
		},
		HostAccessPorts: r.hostPorts,
		WaitingFor:      wait.ForHTTP("/ping").WithPort(runtimePort).WithStartupTimeout(startTimeout),
	}
	if image := os.Getenv(envImage); image != "" {
		req.Image = image
	} else {
		req.FromDockerfile = testcontainers.FromDockerfile{Context: repoRoot(), Dockerfile: "apps/runtime/Dockerfile", KeepImage: true}
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		return err
	}
	endpoint, err := c.PortEndpoint(ctx, runtimePort, "")
	if err != nil {
		_ = c.Terminate(context.Background())
		return err
	}
	r.mu.Lock()
	r.container, r.target = c, endpoint
	r.mu.Unlock()
	return nil
}

func (r *Runtime) accept() {
	for {
		conn, err := r.proxy.Accept()
		if err != nil {
			return
		}
		go r.forward(conn)
	}
}

func (r *Runtime) forward(conn net.Conn) {
	r.mu.Lock()
	target := r.target
	r.mu.Unlock()
	if target == "" {
		_ = conn.Close()
		return
	}
	up, err := net.Dial("tcp", target)
	if err != nil {
		_ = conn.Close()
		return
	}
	r.mu.Lock()
	r.conns[conn], r.conns[up] = struct{}{}, struct{}{}
	r.mu.Unlock()
	done := make(chan struct{}, 2)
	pipe := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		done <- struct{}{}
	}
	go pipe(up, conn)
	go pipe(conn, up)
	<-done
	_ = conn.Close()
	_ = up.Close()
	r.mu.Lock()
	delete(r.conns, conn)
	delete(r.conns, up)
	r.mu.Unlock()
}
