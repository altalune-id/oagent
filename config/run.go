package config

import "time"

// RunConfig bounds how agent turns run and share the deployment.
type RunConfig struct {
	TurnTimeout              time.Duration `yaml:"turnTimeout"              mapstructure:"turnTimeout"              validate:"gte=1m,lte=14m"`
	LeaseTTL                 time.Duration `yaml:"leaseTTL"                 mapstructure:"leaseTTL"                 validate:"gte=2s"`
	Heartbeat                time.Duration `yaml:"heartbeat"                mapstructure:"heartbeat"                validate:"gte=1s"`
	DrainTimeout             time.Duration `yaml:"drainTimeout"             mapstructure:"drainTimeout"             validate:"gte=0"`
	SweepEvery               time.Duration `yaml:"sweepEvery"               mapstructure:"sweepEvery"               validate:"gte=1s"`
	MaxAttempts              int           `yaml:"maxAttempts"              mapstructure:"maxAttempts"              validate:"gte=1"`
	MaxConcurrentPerInstance int           `yaml:"maxConcurrentPerInstance" mapstructure:"maxConcurrentPerInstance" validate:"gte=1"`
	MaxConcurrentPerProject  int           `yaml:"maxConcurrentPerProject"  mapstructure:"maxConcurrentPerProject"  validate:"gte=1"`
	MaxPerMinutePerProject   int           `yaml:"maxPerMinutePerProject"   mapstructure:"maxPerMinutePerProject"   validate:"gte=1"`
}

func (c *Config) validateRun() error {
	if c.Run.Heartbeat > c.Run.LeaseTTL/2 {
		return &InvalidSettingError{Key: "run.heartbeat", Reason: "must be at most half of run.leaseTTL"}
	}
	if c.Run.DrainTimeout >= c.Scheduler.ShutdownGrace {
		return &InvalidSettingError{Key: "run.drainTimeout", Reason: "must be below scheduler.shutdownGrace"}
	}
	return nil
}
