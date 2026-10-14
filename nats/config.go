package nats

import "time"

// Config is the connection: URL (one or several, comma-separated) and either User+Password or Token.
type Config struct {
	URL            string
	Token          string
	User           string
	Password       string
	Name           string
	ConnectTimeout time.Duration
}
