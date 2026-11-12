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

	assert.Len(t, events.Subscribable(), 11)
	assert.Len(t, events.All(), 12)
	for _, t2 := range []events.Type{events.ApprovalRequested, events.ApprovalDecided} {
		spec, ok := events.Lookup(t2)
		assert.True(t, ok, t2)
		assert.True(t, spec.Subscribable, t2)
		assert.False(t, spec.OrgWide, t2)
		assert.Equal(t, "v1", spec.APIVersion())
	}
	for _, t2 := range []events.Type{events.ChannelConnectionCreated, events.ChannelConnectionUpdated, events.ChannelConnectionDeleted} {
		spec, ok := events.Lookup(t2)
		assert.True(t, ok, t2)
		assert.True(t, spec.Subscribable, t2)
		assert.False(t, spec.OrgWide, t2)
		assert.Equal(t, "v1", spec.APIVersion())
	}
	for _, t2 := range []events.Type{events.InviteCreated, events.MemberAdded} {
		spec, ok := events.Lookup(t2)
		assert.True(t, ok, t2)
		assert.True(t, spec.Subscribable, t2)
		assert.True(t, spec.OrgWide, t2)
	}
	for _, t2 := range []events.Type{events.RunCompleted, events.RunFailed} {
		spec, ok := events.Lookup(t2)
		assert.True(t, ok, t2)
		assert.True(t, spec.Subscribable, t2)
	}
	for _, t2 := range []events.Type{events.AgentCreated, events.AgentUpdated, events.RunCompleted, events.RunFailed, events.WebhookPing} {
		spec, _ := events.Lookup(t2)
		assert.False(t, spec.OrgWide, t2)
	}
}

func TestCheckPayload(t *testing.T) {
	assert.NoError(t, events.CheckPayload(events.AgentCreated, events.AgentCreatedV1{}))
	assert.NoError(t, events.CheckPayload(events.AgentUpdated, events.AgentUpdatedV1{}))
	assert.NoError(t, events.CheckPayload(events.WebhookPing, events.WebhookPingV1{}))
	assert.NoError(t, events.CheckPayload(events.InviteCreated, events.InviteCreatedV1{}))
	assert.NoError(t, events.CheckPayload(events.MemberAdded, events.MemberAddedV1{}))
	assert.True(t, events.IsPayloadMismatchError(events.CheckPayload(events.InviteCreated, events.MemberAddedV1{})))
	assert.True(t, events.IsPayloadMismatchError(events.CheckPayload(events.MemberAdded, events.InviteCreatedV1{})))
	assert.NoError(t, events.CheckPayload(events.RunCompleted, events.RunCompletedV1{}))
	assert.NoError(t, events.CheckPayload(events.RunFailed, events.RunFailedV1{}))
	assert.True(t, events.IsPayloadMismatchError(events.CheckPayload(events.RunCompleted, events.RunFailedV1{})))
	assert.True(t, events.IsPayloadMismatchError(events.CheckPayload(events.RunFailed, events.RunCompletedV1{})))

	assert.NoError(t, events.CheckPayload(events.ApprovalRequested, events.ApprovalRequestedV1{}))
	assert.NoError(t, events.CheckPayload(events.ApprovalDecided, events.ApprovalDecidedV1{}))
	assert.True(t, events.IsPayloadMismatchError(events.CheckPayload(events.ApprovalRequested, events.ApprovalDecidedV1{})))
	assert.True(t, events.IsPayloadMismatchError(events.CheckPayload(events.ApprovalDecided, events.ApprovalRequestedV1{})))

	assert.NoError(t, events.CheckPayload(events.ChannelConnectionCreated, events.ChannelConnectionCreatedV1{}))
	assert.NoError(t, events.CheckPayload(events.ChannelConnectionUpdated, events.ChannelConnectionUpdatedV1{}))
	assert.NoError(t, events.CheckPayload(events.ChannelConnectionDeleted, events.ChannelConnectionDeletedV1{}))
	assert.True(t, events.IsPayloadMismatchError(events.CheckPayload(events.ChannelConnectionCreated, events.ChannelConnectionUpdatedV1{})))
	assert.True(t, events.IsPayloadMismatchError(events.CheckPayload(events.ChannelConnectionUpdated, events.ChannelConnectionDeletedV1{})))
	assert.True(t, events.IsPayloadMismatchError(events.CheckPayload(events.ChannelConnectionDeleted, events.ChannelConnectionCreatedV1{})))

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
