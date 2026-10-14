//go:build integration

package redis

import (
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	nooptrace "go.opentelemetry.io/otel/trace/noop"

	"altalune.id/oagent/internal/testutil/redistest"
)

func TestOpen_RealServer(t *testing.T) {
	h := redistest.New(t)
	c, err := Open(t.Context(), testConfig(h.URL), nooptrace.NewTracerProvider())
	require.NoError(t, err)
	require.NoError(t, c.Set(t.Context(), h.Prefix+"k", "v", time.Minute).Err())
	assert.Equal(t, "v", c.Get(t.Context(), h.Prefix+"k").Val())
	require.NoError(t, c.Del(t.Context(), h.Prefix+"k").Err())

	rh := NewHealth(c, time.Second, discard())
	rh.probe(t.Context())
	assert.True(t, rh.Ready())

	require.NoError(t, c.Close())
	assert.NotPanics(t, func() { assert.ErrorIs(t, c.Close(), goredis.ErrClosed) }, "a second Close is an error, not a panic")
	rh.probe(t.Context())
	assert.False(t, rh.Ready())
}
