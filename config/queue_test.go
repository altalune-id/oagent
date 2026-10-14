package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_QueueAccountFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_QUEUE_USER", "oagent")
	t.Setenv("OAGENT_QUEUE_PASSWORD", "a-long-account-password")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, "oagent", c.Queue.User)
	assert.Equal(t, "a-long-account-password", c.Queue.Password)
}

func TestValidate_QueueCredentials(t *testing.T) {
	for name, tc := range map[string]struct {
		queue QueueConfig
		check func(t *testing.T, err error)
	}{
		"none":              {QueueConfig{}, func(t *testing.T, err error) { assert.NoError(t, err) }},
		"token":             {QueueConfig{Token: "t"}, func(t *testing.T, err error) { assert.NoError(t, err) }},
		"account":           {QueueConfig{User: "oagent", Password: "p"}, func(t *testing.T, err error) { assert.NoError(t, err) }},
		"token and user":    {QueueConfig{Token: "t", User: "oagent", Password: "p"}, invalidSetting("queue.token")},
		"token and pass":    {QueueConfig{Token: "t", Password: "p"}, invalidSetting("queue.token")},
		"user without pass": {QueueConfig{User: "oagent"}, missingSetting("queue.user, queue.password")},
		"pass without user": {QueueConfig{Password: "p"}, missingSetting("queue.user, queue.password")},
	} {
		t.Run(name, func(t *testing.T) {
			c := validConfig(t)
			c.Queue = tc.queue
			tc.check(t, c.Validate())
		})
	}
}
