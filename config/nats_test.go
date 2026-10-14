package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const natsTestPassword = "secret-pw-at-least-16"

func TestLoad_NATSDefaults(t *testing.T) {
	c := validConfig(t)
	assert.Empty(t, c.NATS.URL)
	assert.Equal(t, "oagent", c.NATS.Name)
	assert.Equal(t, 10*time.Second, c.NATS.ConnectTimeout)
	assert.Empty(t, c.NATSFeatures(), "nothing uses NATS by default")
}

func TestLoad_NATSFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_NATS_URL", "nats://nats.internal:4222")
	t.Setenv("OAGENT_NATS_USER", "oagent")
	t.Setenv("OAGENT_NATS_PASSWORD", natsTestPassword)
	t.Setenv("OAGENT_NATS_NAME", "oagent-eu")
	t.Setenv("OAGENT_NATS_CONNECT_TIMEOUT", "3s")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, "nats://nats.internal:4222", c.NATS.URL)
	assert.Equal(t, "oagent", c.NATS.User)
	assert.Equal(t, natsTestPassword, c.NATS.Password)
	assert.Equal(t, "oagent-eu", c.NATS.Name)
	assert.Equal(t, 3*time.Second, c.NATS.ConnectTimeout)
}

func TestLoad_NATSTokenFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_NATS_TOKEN", "a-token")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, "a-token", c.NATS.Token)
}

func TestNATSFeatures(t *testing.T) {
	c := validConfig(t)
	assert.Empty(t, c.NATSFeatures(), "a nats driver with the queue off needs no connection")
	c.Queue.Enabled = true
	assert.Equal(t, []string{"queue.driver"}, c.NATSFeatures())
	c.Queue.Driver = "other"
	assert.Empty(t, c.NATSFeatures())
}

func TestValidate_NATS(t *testing.T) {
	useNATS := func(c *Config) {
		c.Queue.Enabled = true
		c.NATS.URL = "nats://nats.internal:4222"
	}
	ok := func(t *testing.T, err error) { assert.NoError(t, err) }
	for name, tc := range map[string]struct {
		mutate func(c *Config)
		check  func(t *testing.T, err error)
	}{
		"unused nats is ignored, even when broken": {
			mutate: func(c *Config) {
				c.NATS.URL = "http://nope"
				c.NATS.Token = "t"
				c.NATS.User = "oagent"
				c.NATS.Password = "short"
			},
			check: ok,
		},
		"used with a url ok": {mutate: useNATS, check: ok},
		"tls ok":             {mutate: func(c *Config) { useNATS(c); c.NATS.URL = "tls://nats.internal:4222" }, check: ok},
		"websocket ok":       {mutate: func(c *Config) { useNATS(c); c.NATS.URL = "wss://nats.internal:443" }, check: ok},
		"several servers ok": {
			mutate: func(c *Config) { useNATS(c); c.NATS.URL = "nats://a.internal:4222, nats://b.internal:4222" },
			check:  ok,
		},
		"credentials in the url ok": {
			mutate: func(c *Config) { useNATS(c); c.NATS.URL = "nats://oagent:" + natsTestPassword + "@nats.internal:4222" },
			check:  ok,
		},
		"used without a url names the feature": {
			mutate: func(c *Config) { useNATS(c); c.NATS.URL = "" },
			check: func(t *testing.T, err error) {
				missingSetting("nats.url")(t, err)
				assert.ErrorContains(t, err, "queue.driver")
			},
		},
		"wrong scheme": {
			mutate: func(c *Config) { useNATS(c); c.NATS.URL = "http://oagent:" + natsTestPassword + "@nats.internal:4222" },
			check:  natsURLError,
		},
		"no host": {
			mutate: func(c *Config) { useNATS(c); c.NATS.URL = "nats://oagent:" + natsTestPassword + "@" },
			check:  natsURLError,
		},
		"unparsable": {
			mutate: func(c *Config) { useNATS(c); c.NATS.URL = "nats://oagent:" + natsTestPassword + "@nats internal:4222" },
			check:  natsURLError,
		},
		"one bad server among several": {
			mutate: func(c *Config) {
				useNATS(c)
				c.NATS.URL = "nats://a.internal:4222,nats://oagent:" + natsTestPassword + "@"
			},
			check: natsURLError,
		},
		"token ok":   {mutate: func(c *Config) { useNATS(c); c.NATS.Token = "t" }, check: ok},
		"account ok": {mutate: func(c *Config) { useNATS(c); c.NATS.User, c.NATS.Password = "oagent", natsTestPassword }, check: ok},
		"password of 16 ok": {
			mutate: func(c *Config) { useNATS(c); c.NATS.User, c.NATS.Password = "oagent", strings.Repeat("p", 16) },
			check:  ok,
		},
		"password of 15": {
			mutate: func(c *Config) { useNATS(c); c.NATS.User, c.NATS.Password = "oagent", strings.Repeat("p", 15) },
			check: func(t *testing.T, err error) {
				invalidSetting("nats.password")(t, err)
				assert.NotContains(t, err.Error(), "ppp", "the password is not echoed")
			},
		},
		"token and account": {
			mutate: func(c *Config) {
				useNATS(c)
				c.NATS.Token = "t"
				c.NATS.User, c.NATS.Password = "oagent", natsTestPassword
			},
			check: invalidSetting("nats.token"),
		},
		"token and password": {
			mutate: func(c *Config) { useNATS(c); c.NATS.Token = "t"; c.NATS.Password = natsTestPassword },
			check:  invalidSetting("nats.token"),
		},
		"user without password": {
			mutate: func(c *Config) { useNATS(c); c.NATS.User = "oagent" },
			check:  missingSetting("nats.user, nats.password"),
		},
		"password without user": {
			mutate: func(c *Config) { useNATS(c); c.NATS.Password = natsTestPassword },
			check:  missingSetting("nats.user, nats.password"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := validConfig(t)
			tc.mutate(c)
			tc.check(t, c.Validate())
		})
	}
}

func natsURLError(t *testing.T, err error) {
	t.Helper()
	invalidSetting("nats.url")(t, err)
	assert.NotContains(t, err.Error(), natsTestPassword, "the url is a secret")
	assert.NotContains(t, err.Error(), "internal", "the url is not echoed")
}
