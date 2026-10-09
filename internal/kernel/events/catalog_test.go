package events_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/kernel/events"
)

func TestCatalog(t *testing.T) {
	spec, ok := events.Lookup(events.AgentCreated)
	assert.True(t, ok)
	assert.True(t, spec.Subscribable)
	assert.Equal(t, "v1", spec.APIVersion())

	_, ok = events.Lookup("nope")
	assert.False(t, ok)

	assert.Len(t, events.Subscribable(), 2)
	assert.Len(t, events.All(), 3)
}

func TestCheckPayload(t *testing.T) {
	assert.NoError(t, events.CheckPayload(events.AgentCreated, events.AgentCreatedV1{}))
	assert.NoError(t, events.CheckPayload(events.AgentUpdated, events.AgentUpdatedV1{}))
	assert.NoError(t, events.CheckPayload(events.WebhookPing, events.WebhookPingV1{}))

	assert.True(t, events.IsPayloadMismatchError(events.CheckPayload(events.AgentCreated, 1)))
	assert.True(t, events.IsUnknownTypeError(events.CheckPayload("nope", 1)))
}

func TestErrorPredicates(t *testing.T) {
	assert.False(t, events.IsUnknownTypeError(errors.New("x")))
	assert.False(t, events.IsPayloadMismatchError(errors.New("x")))
	assert.True(t, events.IsPayloadMismatchError(events.CheckPayload(events.AgentUpdated, 1)))
	assert.True(t, events.IsPayloadMismatchError(events.CheckPayload(events.WebhookPing, 1)))
	assert.Contains(t, (&events.UnknownTypeError{Type: "x"}).Error(), "unknown")
	assert.Contains(t, (&events.PayloadMismatchError{Type: "x"}).Error(), "payload mismatch")
}
