package storage

import "time"

// The drivers storage.driver may name.
const (
	DriverS3     = "s3"
	DriverMemory = "memory"
)

// Config is the feature's settings; boot maps storage.* into it.
type Config struct {
	Driver       string
	Production   bool
	App          string
	UploadURLTTL time.Duration
	PendingTTL   time.Duration
}

// Enabled reports whether uploads are on.
func (c Config) Enabled() bool { return c.Driver != "" }

// Check refuses a missing driver (uploads off), an unknown one, and memory in production.
func (c Config) Check() error {
	switch c.Driver {
	case "":
		return &NotConfiguredError{}
	case DriverS3:
		return nil
	case DriverMemory:
		if c.Production {
			return &MemoryInProductionError{}
		}
		return nil
	default:
		return &UnknownDriverError{Driver: c.Driver}
	}
}
