package outbox

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func validEntry() Entry {
	return Entry{
		ID:            uuid.New(),
		PublicID:      "delivery",
		EventID:       uuid.New(),
		EventPublicID: "event",
		OrgID:         uuid.New(),
		ProjectID:     uuid.New(),
		Target:        "endpoint",
		Payload:       []byte("{}"),
	}
}

func TestValidate(t *testing.T) {
	assert.NoError(t, validEntry().validate())

	e := validEntry()
	e.Target = ""
	assert.True(t, IsInvalidEntryError(e.validate()))

	e = validEntry()
	e.OrgID = uuid.Nil
	assert.True(t, IsInvalidEntryError(e.validate()))

	e = validEntry()
	e.PublicID = ""
	assert.True(t, IsInvalidEntryError(e.validate()))

	e = validEntry()
	e.EventPublicID = ""
	assert.True(t, IsInvalidEntryError(e.validate()))

	e = validEntry()
	e.Status = StatusDelivered
	assert.True(t, IsInvalidEntryError(e.validate()))
}

func TestStatus(t *testing.T) {
	assert.True(t, StatusPending.Valid())
	assert.False(t, StatusPending.Terminal())
	assert.True(t, StatusDelivered.Terminal())
	assert.True(t, StatusFailed.Terminal())
	assert.False(t, Status("nope").Valid())
}

func TestBackoffWithinBounds(t *testing.T) {
	for attempt := 1; attempt <= 10; attempt++ {
		base := BackoffBase(attempt)
		got := Backoff(attempt)
		assert.LessOrEqual(t, got, base+base/10, "attempt %d", attempt)
		assert.GreaterOrEqual(t, got, base-base/10, "attempt %d", attempt)
	}
	assert.Equal(t, 30*time.Second, BackoffBase(1))
}
