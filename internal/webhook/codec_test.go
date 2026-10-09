package webhook

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/internal/kernel/events"
	"altalune.id/oagent/internal/kernel/tenant"
)

func TestEventTypesCodec(t *testing.T) {
	raw, err := marshalEventTypes([]events.Type{events.AgentCreated, events.AgentUpdated})
	require.NoError(t, err)
	got, err := unmarshalEventTypes(raw)
	require.NoError(t, err)
	assert.Equal(t, []events.Type{events.AgentCreated, events.AgentUpdated}, got)

	raw, err = marshalEventTypes(nil)
	require.NoError(t, err)
	assert.Equal(t, "[]", raw)
	_, err = unmarshalEventTypes("nope")
	assert.Error(t, err)
}

func TestHeadersCodec(t *testing.T) {
	raw, err := marshalHeaders([]Header{{Name: "X", Value: "y"}})
	require.NoError(t, err)
	got, err := unmarshalHeaders(raw)
	require.NoError(t, err)
	assert.Equal(t, []Header{{Name: "X", Value: "y"}}, got)
	_, err = unmarshalHeaders("nope")
	assert.Error(t, err)
}

func TestPrepareAttempt(t *testing.T) {
	org, project := uuid.New(), uuid.New()
	tc := tenant.Context{OrgID: org, ProjectID: project}

	a, err := prepareAttempt(tc, Attempt{OrgID: org, ProjectID: project, ResponseBody: "b"})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, a.ID)
	assert.False(t, a.CreatedAt.IsZero())

	_, err = prepareAttempt(tc, Attempt{OrgID: uuid.New(), ProjectID: project})
	assert.True(t, IsInvalidAttemptError(err))

	_, err = prepareAttempt(tc, Attempt{OrgID: org, ProjectID: uuid.New()})
	assert.True(t, IsInvalidAttemptError(err))

	at := time.Unix(1, 0)
	a, err = prepareAttempt(tc, Attempt{OrgID: org, ProjectID: project, CreatedAt: at})
	require.NoError(t, err)
	assert.Equal(t, at.UTC(), a.CreatedAt)
}
