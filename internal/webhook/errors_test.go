package webhook

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorMessages(t *testing.T) {
	assert.Contains(t, (&NotFoundError{}).Error(), "not found")
	assert.Contains(t, (&NotFoundError{ID: "x"}).Error(), "id=x")
	assert.Contains(t, (&InvalidURLError{Reason: "r"}).Error(), "r")
	assert.Contains(t, (&InvalidEventTypesError{Reason: "r"}).Error(), "r")
	assert.Contains(t, (&InvalidDescriptionError{Reason: "r"}).Error(), "r")
	assert.Contains(t, (&EndpointLimitError{Limit: 3}).Error(), "limit=3")
	assert.Contains(t, (&EndpointInactiveError{ID: "x"}).Error(), "id=x")
	assert.Contains(t, (&SecretConflictError{ID: "x"}).Error(), "id=x")
	assert.Contains(t, (&InvalidAttemptError{Field: "F", Reason: "R"}).Error(), "F R")
	assert.Contains(t, (&SecretUnavailableError{}).Error(), "rotate")
	assert.Contains(t, (&DeliveryFailedError{StatusCode: 500}).Error(), "status=500")
	assert.Contains(t, (&SecretUnavailableError{Cause: errors.New("boom")}).Error(), "boom")
	assert.Error(t, (&SecretUnavailableError{Cause: errors.New("boom")}).Unwrap())
}

func TestErrorPredicatesFalse(t *testing.T) {
	err := errors.New("x")
	assert.False(t, IsNotFoundError(err))
	assert.False(t, IsInvalidURLError(err))
	assert.False(t, IsInvalidEventTypesError(err))
	assert.False(t, IsInvalidDescriptionError(err))
	assert.False(t, IsEndpointLimitError(err))
	assert.False(t, IsEndpointInactiveError(err))
	assert.False(t, IsSecretConflictError(err))
	assert.False(t, IsInvalidAttemptError(err))
	assert.False(t, IsSecretUnavailableError(err))
	assert.False(t, IsDeliveryFailedError(err))
}
