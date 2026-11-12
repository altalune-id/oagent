package http

import (
	nethttp "net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/httpclient"
)

func TestClient_IdleConnectionsCloseBeforeTheRuntimeDoes(t *testing.T) {
	tr, ok := httpclient.New(append(clientOptions(), httpclient.WithOtel(false))...).Transport.(*nethttp.Transport)
	require.True(t, ok)
	assert.Equal(t, 4*time.Second, tr.IdleConnTimeout, "below Node's 5 s keep-alive, so a turn never goes out on a connection the runtime closed")
}
