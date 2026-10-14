package redis

import (
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

const testPassword = "hunter2-redis-secret"

func testConfig(url string) Config {
	return Config{
		URL: url, MaxConnections: 10, MaxIdleTime: 5 * time.Minute,
		ConnectTimeout: 300 * time.Millisecond, ReadTimeout: time.Second, WriteTimeout: time.Second,
	}
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func silentServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	done := make(chan struct{})
	t.Cleanup(func() {
		close(done)
		_ = ln.Close()
	})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				<-done
				_ = conn.Close()
			}()
		}
	}()
	return ln.Addr().String()
}

func TestOpen_SilentServerFailsWithinConnectTimeout(t *testing.T) {
	addr := silentServer(t)
	start := time.Now()
	c, err := Open(t.Context(), testConfig("redis://:"+testPassword+"@"+addr+"/0"), nooptrace.NewTracerProvider())
	require.Error(t, err)
	assert.Nil(t, c)
	assert.Less(t, time.Since(start), 2*time.Second, "bounded by redis.connectTimeout")
	assert.ErrorContains(t, err, "redis: cannot connect to "+addr+" (redis.url)")
	assert.NotContains(t, err.Error(), testPassword)
}

func TestOpen_RefusedHasNoPassword(t *testing.T) {
	_, err := Open(t.Context(), testConfig("redis://user:"+testPassword+"@127.0.0.1:1/0"), nooptrace.NewTracerProvider())
	require.Error(t, err)
	assert.ErrorContains(t, err, "redis: cannot connect to 127.0.0.1:1 (redis.url)")
	assert.NotContains(t, err.Error(), testPassword)
}

func TestOpen_BadURLIsNotEchoed(t *testing.T) {
	_, err := Open(t.Context(), testConfig("http://:"+testPassword+"@host:6379"), nooptrace.NewTracerProvider())
	require.Error(t, err)
	assert.ErrorContains(t, err, "redis.url")
	assert.NotContains(t, err.Error(), testPassword)
	assert.NotContains(t, err.Error(), "host:6379")
}
