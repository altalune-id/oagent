package config

import (
	"regexp"
	"time"
)

// StorageDriver selects where uploaded files live.
type StorageDriver string

// The storage drivers: s3 is any S3-compatible bucket, memory is for tests and development only.
const (
	StorageDriverS3     StorageDriver = "s3"
	StorageDriverMemory StorageDriver = "memory"
)

const maxPresignTTL = 7 * 24 * time.Hour

// StorageConfig configures file uploads; an empty driver turns them off.
type StorageConfig struct {
	Driver StorageDriver `yaml:"driver" mapstructure:"driver" validate:"omitempty,oneof=s3 memory"`
	// App is the second key segment, so several apps can share one bucket.
	App    string              `yaml:"app"    mapstructure:"app"`
	Upload StorageUploadConfig `yaml:"upload" mapstructure:"upload"`
}

// StorageUploadConfig bounds an upload: how long its URL is valid and how long an unfinished one is kept.
type StorageUploadConfig struct {
	URLTTL     time.Duration `yaml:"urlTTL"     mapstructure:"urlTTL"`
	PendingTTL time.Duration `yaml:"pendingTTL" mapstructure:"pendingTTL"`
}

//nolint:gochecknoglobals // compiled once, read-only.
var storageAppRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

func (c *Config) validateStorage() error {
	s := c.Storage
	if s.Driver == "" {
		return nil
	}
	if s.Driver == StorageDriverMemory && c.Env.IsProduction() {
		return &InvalidSettingError{Key: "storage.driver", Reason: "memory is for tests and development only"}
	}
	if !storageAppRE.MatchString(s.App) {
		return &InvalidSettingError{Key: "storage.app", Reason: "must be 1-32 lowercase letters, digits or -"}
	}
	switch {
	case s.Upload.URLTTL < time.Second || s.Upload.URLTTL > maxPresignTTL:
		return &InvalidSettingError{Key: "storage.upload.urlTTL", Reason: "must be between 1s and 168h"}
	case s.Upload.PendingTTL < s.Upload.URLTTL:
		return &InvalidSettingError{Key: "storage.upload.pendingTTL", Reason: "must be at least storage.upload.urlTTL"}
	}
	return nil
}
