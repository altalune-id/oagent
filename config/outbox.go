package config

import "time"

// OutboxConfig tunes the webhook outbox dispatch worker.
type OutboxConfig struct {
	IdlePoll time.Duration `yaml:"idlePoll" mapstructure:"idlePoll" validate:"omitempty,gte=1s"`
}
