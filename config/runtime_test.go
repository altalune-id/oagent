package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const runtimeToken = "0123456789abcdef0123456789abcdef"

func TestLoad_RuntimeDefaults(t *testing.T) {
	c := validConfig(t)
	assert.Equal(t, RuntimeDriverHTTP, c.Runtime.Driver)
	assert.Empty(t, c.Runtime.URL, "a deployment may start without a runtime; runs then refuse")
}

func TestLoad_RuntimeFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_RUNTIME_URL", "http://runtime.internal:8080")
	t.Setenv("OAGENT_RUNTIME_TOKEN", runtimeToken)
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, "http://runtime.internal:8080", c.Runtime.URL)
	assert.Equal(t, runtimeToken, c.Runtime.Token)
}

func TestValidate_Runtime(t *testing.T) {
	c := validConfig(t)
	c.Runtime.Driver = "grpc"
	invalidSetting("runtime.driver")(t, c.Validate())

	c = validConfig(t)
	c.Runtime.URL = "not a url"
	invalidSetting("runtime.url")(t, c.Validate())

	c = validConfig(t)
	c.Runtime.URL = "http://runtime.internal:8080"
	missingSetting("runtime.token")(t, c.Validate())

	c.Runtime.Token = "short"
	invalidSetting("runtime.token")(t, c.Validate())

	c.Runtime.Token = runtimeToken
	require.NoError(t, c.Validate())
}
