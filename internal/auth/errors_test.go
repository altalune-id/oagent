package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/apperror"
)

func TestInvalidCredentialsError(t *testing.T) {
	err := &InvalidCredentialsError{}
	assert.True(t, IsInvalidCredentialsError(err))
	assert.False(t, IsInvalidCredentialsError(assert.AnError))
	ae := err.ToAppError()
	assert.Equal(t, apperror.CodeAuthInvalidCredentials, ae.Code())
	assert.Equal(t, "Invalid email or password", ae.Message())
}

func TestRateLimitedError(t *testing.T) {
	err := &RateLimitedError{RetryAfter: 90 * time.Second}
	assert.True(t, IsRateLimitedError(err))
	assert.False(t, IsRateLimitedError(assert.AnError))
	assert.Equal(t, 90, err.RetryAfterSeconds())
	ae := err.ToAppError()
	assert.Equal(t, apperror.CodeAuthRateLimited, ae.Code())
	assert.Equal(t, "Too many attempts. Try again in 2 minutes.", ae.Message())
	assert.Equal(t, map[string]string{"retry_after": "90"}, ae.Meta())

	for d, want := range map[time.Duration]string{
		0:                "Too many attempts. Try again in a minute.",
		30 * time.Second: "Too many attempts. Try again in a minute.",
		time.Minute:      "Too many attempts. Try again in a minute.",
		61 * time.Second: "Too many attempts. Try again in 2 minutes.",
		15 * time.Minute: "Too many attempts. Try again in 15 minutes.",
	} {
		assert.Equal(t, want, (&RateLimitedError{RetryAfter: d}).Message(), d)
	}
}
