package nats

import (
	"testing"

	"github.com/nats-io/nats-server/v2/server"

	"altalune.id/oagent/internal/testutil/natstest"
)

func startNATS(t *testing.T) string {
	t.Helper()
	return natstest.Start(t).ClientURL()
}

func startNATSServer(t *testing.T) *server.Server {
	t.Helper()
	return natstest.Start(t)
}
