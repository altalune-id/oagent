package boot

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/auth-server/config"
)

func TestHTTPConfig(t *testing.T) {
	c := httpConfig(config.HTTPConfig{Addr: ":8080"})
	assert.Equal(t, "http", c.Name)
	assert.Equal(t, ":8080", c.Addr)
}
