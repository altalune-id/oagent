package mailqueue

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBackoffBase_Schedule(t *testing.T) {
	for attempt, want := range map[int]time.Duration{
		0: 30 * time.Second,
		1: 30 * time.Second,
		2: 30 * time.Second,
		3: 5 * time.Minute,
		4: 30 * time.Minute,
		5: 2 * time.Hour,
		6: 5 * time.Hour,
		7: 10 * time.Hour,
		9: 10 * time.Hour,
	} {
		assert.Equal(t, want, backoffBase(attempt), "attempt %d", attempt)
	}
}

func TestBackoff_JittersWithinTenPercent(t *testing.T) {
	for attempt := 1; attempt <= 10; attempt++ {
		base := backoffBase(attempt)
		for range 20 {
			got := backoff(attempt)
			assert.LessOrEqual(t, got, base+base/10, "attempt %d", attempt)
			assert.GreaterOrEqual(t, got, base-base/10, "attempt %d", attempt)
		}
	}
}
