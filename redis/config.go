package redis

import "time"

// Config is the connection: a redis:// or rediss:// URL, the pool and the timeouts.
type Config struct {
	URL                string
	MaxConnections     int
	MinIdleConnections int
	MaxIdleTime        time.Duration
	ConnectTimeout     time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
}
