// Package scheduler runs registered Jobs on wall-clock or interval schedules inside one process.
package scheduler

import "time"

// Schedule decides when a Job fires next.
type Schedule interface {
	// NOTE: Next is called concurrently, so any randomness must be guarded.
	Next(now time.Time) time.Time
	String() string
}
