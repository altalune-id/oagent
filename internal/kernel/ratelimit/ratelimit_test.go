package ratelimit

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRule_Validate(t *testing.T) {
	ok := Rule{Name: "login", Rate: 10, Per: 15 * time.Minute, Burst: 5}
	require.NoError(t, ok.Validate())
	assert.Equal(t, 90*time.Second, ok.Interval())
	assert.Equal(t, 6*time.Minute, ok.Tolerance())

	for name, r := range map[string]Rule{
		"no name":          {Rate: 1, Per: time.Second, Burst: 1},
		"zero rate":        {Name: "x", Per: time.Second, Burst: 1},
		"zero period":      {Name: "x", Rate: 1, Burst: 1},
		"zero burst":       {Name: "x", Rate: 1, Per: time.Second},
		"too fast to time": {Name: "x", Rate: 2, Per: time.Nanosecond, Burst: 1},
	} {
		t.Run(name, func(t *testing.T) {
			err := r.Validate()
			assert.True(t, IsInvalidRuleError(err), "got %v", err)
			assert.Contains(t, err.Error(), "ratelimit: rule")
		})
	}
}

func TestKey(t *testing.T) {
	r := Rule{Name: "login"}
	assert.Equal(t, "login:ip:192.0.2.1", Key(r, IPSubject("192.0.2.1")))

	sum := sha256.Sum256([]byte("ana@example.com"))
	want := "email:" + hex.EncodeToString(sum[:])
	assert.Equal(t, want, EmailSubject(" Ana@Example.com "))
	assert.NotContains(t, Key(r, EmailSubject("ana@example.com")), "ana")
}

func TestDisabled(t *testing.T) {
	l := Disabled()
	r := Rule{Name: "login", Rate: 1, Per: time.Hour, Burst: 1}
	for range 100 {
		res, err := l.Allow(t.Context(), "k", r)
		require.NoError(t, err)
		require.True(t, res.Allowed)
	}
	assert.NoError(t, l.Reset(t.Context(), "k"))
	res, err := l.Allow(t.Context(), "k", Rule{})
	require.NoError(t, err)
	assert.True(t, res.Allowed, "a disabled limiter never refuses, even a bad rule")
}

func TestResult_RetryAfterSeconds(t *testing.T) {
	assert.Equal(t, 0, Result{Allowed: true}.RetryAfterSeconds())
	assert.Equal(t, 1, Result{RetryAfter: time.Millisecond}.RetryAfterSeconds())
	assert.Equal(t, 90, Result{RetryAfter: 90 * time.Second}.RetryAfterSeconds())
	assert.Equal(t, 91, Result{RetryAfter: 90*time.Second + time.Nanosecond}.RetryAfterSeconds())
}
