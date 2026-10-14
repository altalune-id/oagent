package boot

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	natsconn "altalune.id/oagent/nats"
	"altalune.id/oagent/pg"
	redisconn "altalune.id/oagent/redis"
)

func TestReadinessChecks(t *testing.T) {
	dbh := &pg.HealthMonitor{}
	assert.Len(t, readinessChecks(dbh, nil, nil), 1, "no redis or nats, no extra check")

	rh := redisconn.NewHealth(nil, time.Second, testLogger())
	nh := natsconn.NewHealth(nil, testLogger())
	checks := readinessChecks(dbh, rh, nh)
	require.Len(t, checks, 3)
	assert.Same(t, rh, checks[1])
	assert.Same(t, nh, checks[2])

	checks = readinessChecks(dbh, nil, nh)
	require.Len(t, checks, 2)
	assert.Same(t, nh, checks[1])
}
