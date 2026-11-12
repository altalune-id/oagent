package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testS3Secret = "s3-secret-access-key-value"

func useS3(c *Config) {
	c.Storage.Driver = StorageDriverS3
	c.S3 = S3Config{
		Endpoint: "https://acct.r2.cloudflarestorage.com", Region: "auto", Bucket: "altalune-files",
		AccessKeyID: "key-id", SecretAccessKey: testS3Secret, UsePathStyle: true, Timeout: 10 * time.Second,
	}
}

func TestLoad_S3Defaults(t *testing.T) {
	c := validConfig(t)
	assert.Empty(t, c.S3.Endpoint)
	assert.True(t, c.S3.UsePathStyle)
	assert.Equal(t, 10*time.Second, c.S3.Timeout)
	assert.Empty(t, c.S3Features(), "nothing uses S3 by default")
}

func TestLoad_S3FromEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("OAGENT_STORAGE_DRIVER", "s3")
	t.Setenv("OAGENT_S3_ENDPOINT", "https://acct.r2.cloudflarestorage.com")
	t.Setenv("OAGENT_S3_BROWSER_ENDPOINT", "https://uploads.example.com")
	t.Setenv("OAGENT_S3_REGION", "auto")
	t.Setenv("OAGENT_S3_BUCKET", "altalune-files")
	t.Setenv("OAGENT_S3_ACCESS_KEY_ID", "key-id")
	t.Setenv("OAGENT_S3_SECRET_ACCESS_KEY", testS3Secret)
	t.Setenv("OAGENT_S3_USE_PATH_STYLE", "false")
	t.Setenv("OAGENT_S3_TIMEOUT", "3s")
	c, err := Load(missingPath(t))
	require.NoError(t, err)
	assert.Equal(t, S3Config{
		Endpoint: "https://acct.r2.cloudflarestorage.com", BrowserEndpoint: "https://uploads.example.com",
		Region: "auto", Bucket: "altalune-files", AccessKeyID: "key-id", SecretAccessKey: testS3Secret,
		UsePathStyle: false, Timeout: 3 * time.Second,
	}, c.S3)
	assert.Equal(t, []string{"storage.driver"}, c.S3Features())
}

func TestS3Features(t *testing.T) {
	c := validConfig(t)
	c.Storage.Driver = StorageDriverMemory
	assert.Empty(t, c.S3Features())
	c.Storage.Driver = StorageDriverS3
	assert.Equal(t, []string{"storage.driver"}, c.S3Features())
}

func TestValidate_S3(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(c *Config)
		check  func(t *testing.T, err error)
	}{
		"unused s3 is ignored, even when broken": {
			mutate: func(c *Config) { c.S3.Endpoint = "ftp://nope"; c.S3.Timeout = 0 },
			check:  func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"used with every key ok": {
			mutate: useS3,
			check:  func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"any region is accepted": {
			mutate: func(c *Config) { useS3(c); c.S3.Region = "garage" },
			check:  func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"http endpoint and browser endpoint ok": {
			mutate: func(c *Config) {
				useS3(c)
				c.S3.Endpoint, c.S3.BrowserEndpoint = "http://garage:3900", "http://localhost:3900"
			},
			check: func(t *testing.T, err error) { assert.NoError(t, err) },
		},
		"no endpoint names the feature": {
			mutate: func(c *Config) { useS3(c); c.S3.Endpoint = "" },
			check: func(t *testing.T, err error) {
				missingSetting("s3.endpoint")(t, err)
				assert.ErrorContains(t, err, "storage.driver is s3")
			},
		},
		"no region":     {mutate: func(c *Config) { useS3(c); c.S3.Region = "" }, check: missingSetting("s3.region")},
		"no bucket":     {mutate: func(c *Config) { useS3(c); c.S3.Bucket = "" }, check: missingSetting("s3.bucket")},
		"no key id":     {mutate: func(c *Config) { useS3(c); c.S3.AccessKeyID = "" }, check: missingSetting("s3.accessKeyID")},
		"no secret key": {mutate: func(c *Config) { useS3(c); c.S3.SecretAccessKey = "" }, check: missingSetting("s3.secretAccessKey")},
		"endpoint without a scheme": {
			mutate: func(c *Config) { useS3(c); c.S3.Endpoint = "acct.r2.cloudflarestorage.com" },
			check:  s3EndpointError("s3.endpoint"),
		},
		"unparsable endpoint": {
			mutate: func(c *Config) { useS3(c); c.S3.Endpoint = "https://acct r2" },
			check:  s3EndpointError("s3.endpoint"),
		},
		"credentials in the endpoint": {
			mutate: func(c *Config) { useS3(c); c.S3.Endpoint = "https://key:" + testS3Secret + "@acct.r2" },
			check:  s3EndpointError("s3.endpoint"),
		},
		"query in the browser endpoint": {
			mutate: func(c *Config) { useS3(c); c.S3.BrowserEndpoint = "https://acct.r2/?t=" + testS3Secret },
			check:  s3EndpointError("s3.browserEndpoint"),
		},
		"zero timeout": {
			mutate: func(c *Config) { useS3(c); c.S3.Timeout = 0 },
			check:  invalidSetting("s3.timeout"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := validConfig(t)
			tc.mutate(c)
			tc.check(t, c.validateS3())
		})
	}
}

func s3EndpointError(key string) func(t *testing.T, err error) {
	return func(t *testing.T, err error) {
		t.Helper()
		invalidSetting(key)(t, err)
		assert.NotContains(t, err.Error(), testS3Secret)
		assert.NotContains(t, err.Error(), "acct", "the endpoint is not echoed")
	}
}
