package outbox

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorMessages(t *testing.T) {
	assert.Contains(t, (&NotFoundError{ID: "x"}).Error(), "not found")
	assert.Contains(t, (&TerminalStateError{ID: "x", Status: StatusFailed}).Error(), "already failed")
	assert.Contains(t, (&StaleClaimError{ID: "x"}).Error(), "re-claimed")
	assert.Contains(t, (&NotFailedError{ID: "x", Status: StatusPending}).Error(), "not failed")
	assert.Contains(t, (&InvalidEntryError{Field: "F", Reason: "R"}).Error(), "F R")
}

func TestErrorPredicatesFalse(t *testing.T) {
	err := errors.New("x")
	assert.False(t, IsNotFoundError(err))
	assert.False(t, IsTerminalStateError(err))
	assert.False(t, IsStaleClaimError(err))
	assert.False(t, IsNotFailedError(err))
	assert.False(t, IsInvalidEntryError(err))
}
