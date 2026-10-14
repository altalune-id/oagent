package pg

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetry_SucceedsAfterFailure(t *testing.T) {
	attempts := 0
	err := retry(context.Background(), time.Second, time.Millisecond, func(context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("boom")
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestRetry_StopsOnPermanent(t *testing.T) {
	attempts := 0
	err := retry(context.Background(), time.Second, time.Millisecond, func(context.Context) error {
		attempts++
		return permanent(errors.New("fatal"))
	})
	assert.True(t, isPermanentError(err))
	assert.Equal(t, 1, attempts)
}

func TestRetry_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := retry(ctx, time.Second, time.Millisecond, func(context.Context) error {
		return errors.New("boom")
	})
	assert.Error(t, err)
}

func TestRetry_ExhaustsBudget(t *testing.T) {
	err := retry(context.Background(), time.Millisecond, time.Millisecond, func(context.Context) error {
		return errors.New("boom")
	})
	assert.Error(t, err)
}

func TestRetry_ZeroBudget(t *testing.T) {
	err := retry(context.Background(), 0, 0, func(context.Context) error {
		return errors.New("boom")
	})
	assert.Error(t, err)
}

func TestRetry_CanceledDuringSleep(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := retry(ctx, time.Second, 100*time.Millisecond, func(context.Context) error {
		return errors.New("boom")
	})
	assert.Error(t, err)
}

func TestPermanent_Nil(t *testing.T) {
	assert.NoError(t, permanent(nil))
}

func TestPermanentError_ErrorAndUnwrap(t *testing.T) {
	base := errors.New("boom")
	err := permanent(base)
	assert.Equal(t, "boom", err.Error())
	assert.ErrorIs(t, err, base)
}

func TestWithRetryLogger_Nil(t *testing.T) {
	ctx := withRetryLogger(context.Background(), nil)
	assert.Nil(t, loggerFrom(ctx))
}
