package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecode(t *testing.T) {
	got, err := Decode[testPayload](Message{Name: "test.run", Version: 1, Data: []byte(`{"title":"a"}`)})
	require.NoError(t, err)
	assert.Equal(t, testPayload{Title: "a"}, got)

	_, err = Decode[testPayload](Message{Name: "test.run", Version: 1, Data: []byte(`{`)})
	assert.ErrorContains(t, err, "queue: decode test.run v1")
}

func TestPermanent(t *testing.T) {
	assert.NoError(t, Permanent(nil))
	cause := errors.New("bad payload")
	err := Permanent(cause)
	assert.True(t, IsPermanentError(err))
	assert.ErrorIs(t, err, cause)
	assert.Equal(t, "queue: permanent: bad payload", err.Error())
	assert.False(t, IsPermanentError(cause))
}

func TestConsumerClosedAndDisabledErrors(t *testing.T) {
	closed := &ConsumerClosedError{Subject: "jobs.test.run.v1"}
	assert.True(t, IsConsumerClosedError(closed))
	assert.Contains(t, closed.Error(), "jobs.test.run.v1")
	assert.False(t, IsConsumerClosedError(context.Canceled))
	assert.Equal(t, "queue: disabled", (&DisabledError{}).Error())
}

func TestDisabled_ConsumerChecksWiringFirst(t *testing.T) {
	_, err := Disabled(nil).Consumer(t.Context(), []Handler{{Job: testJob()}})
	assert.True(t, IsNilHandlerError(err))
}
