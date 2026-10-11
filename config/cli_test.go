package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad_CLISessionFileDefault(t *testing.T) {
	assert.Equal(t, "~/.oagent/session.json", validConfig(t).CLI.SessionFile)
}

func TestLoad_CLISessionFileFromEnv(t *testing.T) {
	t.Setenv("OAGENT_CLI_SESSION_FILE", "/tmp/x/session.json")
	assert.Equal(t, "/tmp/x/session.json", validConfig(t).CLI.SessionFile)
}
