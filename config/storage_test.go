package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_StorageDefaults(t *testing.T) {
	c := validConfig(t)
	assert.Equal(t, StorageConfig{
		App:    "oagent",
		Upload: StorageUploadConfig{URLTTL: 15 * time.Minute, PendingTTL: time.Hour},
	}, c.Storage, "uploads are off until a driver is set")
}

func TestLoad_StorageFromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_STORAGE_DRIVER", "memory")
	t.Setenv("OAGENT_ENV", "development")
	t.Setenv("OAGENT_STORAGE_APP", "oagent-dev")
	t.Setenv("OAGENT_STORAGE_UPLOAD_URL_TTL", "2m")
	t.Setenv("OAGENT_STORAGE_UPLOAD_PENDING_TTL", "30m")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, StorageDriverMemory, c.Storage.Driver)
	assert.Equal(t, "oagent-dev", c.Storage.App)
	assert.Equal(t, 2*time.Minute, c.Storage.Upload.URLTTL)
	assert.Equal(t, 30*time.Minute, c.Storage.Upload.PendingTTL)
	assert.Empty(t, c.S3Features(), "memory needs no connection")
}

func TestLoad_StorageRejectsAnUnknownDriver(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_STORAGE_DRIVER", "gcs")
	_, err := Load(missingPath(t))
	assert.ErrorContains(t, err, "storage.driver")
}

func TestValidate_Storage(t *testing.T) {
	useStorage := func(c *Config) {
		c.Storage.Driver = StorageDriverMemory
		c.Env = EnvDevelopment
	}
	for name, tc := range map[string]struct {
		mutate func(c *Config)
		check  func(t *testing.T, err error)
	}{
		"off is ignored, even when broken": {
			mutate: func(c *Config) { c.Storage.App = "Bad App"; c.Storage.Upload.URLTTL = 0 },
			check:  func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"memory in development ok": {mutate: useStorage, check: func(t *testing.T, err error) { assert.NoError(t, err) }},
		"s3 in production ok":      {mutate: useS3, check: func(t *testing.T, err error) { assert.NoError(t, err) }},
		"memory in production": {
			mutate: func(c *Config) { useStorage(c); c.Env = EnvProduction },
			check:  invalidSetting("storage.driver"),
		},
		"bad app":   {mutate: func(c *Config) { useStorage(c); c.Storage.App = "OAgent" }, check: invalidSetting("storage.app")},
		"empty app": {mutate: func(c *Config) { useStorage(c); c.Storage.App = "" }, check: invalidSetting("storage.app")},
		"upload url ttl too short": {
			mutate: func(c *Config) { useStorage(c); c.Storage.Upload.URLTTL = time.Millisecond },
			check:  invalidSetting("storage.upload.urlTTL"),
		},
		"upload url ttl beyond 7 days": {
			mutate: func(c *Config) { useStorage(c); c.Storage.Upload.URLTTL = 8 * 24 * time.Hour },
			check:  invalidSetting("storage.upload.urlTTL"),
		},
		"pending shorter than the url": {
			mutate: func(c *Config) { useStorage(c); c.Storage.Upload.PendingTTL = time.Minute },
			check:  invalidSetting("storage.upload.pendingTTL"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := validConfig(t)
			tc.mutate(c)
			tc.check(t, c.validateStorage())
		})
	}
}
