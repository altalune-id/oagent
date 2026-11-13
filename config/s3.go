package config

import (
	"net/url"
	"strings"
	"time"
)

// S3Config is the one S3-compatible connection (R2, Garage, AWS); it is opened only when a feature selects driver s3.
type S3Config struct {
	// Endpoint is the S3 API URL oagent calls, e.g. https://<account>.r2.cloudflarestorage.com.
	Endpoint string `yaml:"endpoint" mapstructure:"endpoint"`
	// BrowserEndpoint is the S3 API URL browsers upload to, when it is not Endpoint; signatures cover its exact host.
	BrowserEndpoint string `yaml:"browserEndpoint" mapstructure:"browserEndpoint"`
	// Region is the signing region: auto for R2, the s3_region of a Garage server.
	Region          string        `yaml:"region"          mapstructure:"region"`
	Bucket          string        `yaml:"bucket"          mapstructure:"bucket"`
	AccessKeyID     string        `yaml:"accessKeyID"     mapstructure:"accessKeyID"     awareness:"secret"`
	SecretAccessKey string        `yaml:"secretAccessKey" mapstructure:"secretAccessKey" awareness:"secret"`
	UsePathStyle    bool          `yaml:"usePathStyle"    mapstructure:"usePathStyle"`
	Timeout         time.Duration `yaml:"timeout"         mapstructure:"timeout"`
}

// S3Features lists the settings that select S3; empty means oagent never connects to it.
func (c *Config) S3Features() []string {
	var out []string
	if c.Storage.Driver == StorageDriverS3 {
		out = append(out, "storage.driver")
	}
	return out
}

func (c *Config) validateS3() error {
	features := c.S3Features()
	if len(features) == 0 {
		return nil
	}
	s := c.S3
	reason := strings.Join(features, ", ") + " is s3"
	for _, req := range []struct{ key, val string }{
		{"s3.endpoint", s.Endpoint},
		{"s3.region", s.Region},
		{"s3.bucket", s.Bucket},
		{"s3.accessKeyID", s.AccessKeyID},
		{"s3.secretAccessKey", s.SecretAccessKey},
	} {
		if req.val == "" {
			return &MissingSettingError{Keys: req.key, Reason: reason}
		}
	}
	if err := validateS3Endpoint("s3.endpoint", s.Endpoint); err != nil {
		return err
	}
	if s.BrowserEndpoint != "" {
		if err := validateS3Endpoint("s3.browserEndpoint", s.BrowserEndpoint); err != nil {
			return err
		}
	}
	if s.Timeout <= 0 {
		return &InvalidSettingError{Key: "s3.timeout", Reason: "must be positive"}
	}
	return nil
}

// SECURITY: an endpoint may carry credentials, so no error repeats it or the parser's message.
func validateS3Endpoint(key, raw string) error {
	u, err := url.Parse(raw)
	switch {
	case err != nil, u.Scheme != "http" && u.Scheme != "https", u.Host == "":
		return &InvalidSettingError{Key: key, Reason: "must be an http:// or https:// URL with a host"}
	case u.User != nil, u.RawQuery != "", u.Fragment != "":
		return &InvalidSettingError{Key: key, Reason: "must not carry credentials, a query or a fragment"}
	}
	return nil
}
