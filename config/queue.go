package config

// QueueDriver selects what carries queued jobs and broadcasts.
type QueueDriver string

// QueueDriverNATS runs the queue on NATS JetStream.
const QueueDriverNATS QueueDriver = "nats"

// QueueConfig turns the job queue on and picks its driver.
type QueueConfig struct {
	Enabled bool        `yaml:"enabled" mapstructure:"enabled"`
	Driver  QueueDriver `yaml:"driver"  mapstructure:"driver"  validate:"oneof=nats"`
}
