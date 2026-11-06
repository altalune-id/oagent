package httpserver

import "time"

// Config configures the HTTP server worker.
type Config struct {
	Name string
	Addr string
	// ShutdownGrace is how long in-flight requests may finish after a stop; zero means 10 s.
	ShutdownGrace time.Duration
}
