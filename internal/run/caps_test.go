package run_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/ratelimit/memory"
	"altalune.id/oagent/internal/run"
)

func TestRun_ProjectCapsRefuse(t *testing.T) {
	limit := run.RateCap{Limiter: memory.New(100), PerMinute: 2}
	project, other := uuid.New(), uuid.New()
	ctx := context.Background()
	require.NoError(t, limit.Allow(ctx, project))
	require.NoError(t, limit.Allow(ctx, project))
	err := limit.Allow(ctx, project)
	require.True(t, run.IsRateLimitedError(err), "%v", err)
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok)
	assert.Equal(t, apperror.CodeRunRateLimited, ae.Code())
	require.NoError(t, limit.Allow(ctx, other), "the cap is per project")
	require.NoError(t, run.RateCap{}.Allow(ctx, project), "no limiter caps nothing")
}

func TestErrors_MapToRunCodes(t *testing.T) {
	for _, c := range []struct {
		err  error
		code apperror.Code
	}{
		{&run.NotFoundError{ID: "r"}, apperror.CodeRunNotFound},
		{&run.RateLimitedError{}, apperror.CodeRunRateLimited},
		{&run.RuntimeUnavailableError{}, apperror.CodeRunRuntimeUnavailable},
		{&run.TimedOutError{}, apperror.CodeRunTimedOut},
		{&run.AttemptsExhaustedError{Attempts: 3}, apperror.CodeRunAttemptsExhausted},
		{&run.RuntimeFailedError{Reason: "model_error"}, apperror.CodeRunRuntimeFailed},
	} {
		ae, ok := apperror.AsAppError(c.err)
		require.True(t, ok, "%T", c.err)
		assert.Equal(t, c.code, ae.Code())
		assert.NotEmpty(t, c.err.Error())
		assert.True(t, run.IsExpectedError(c.err))
	}
}
