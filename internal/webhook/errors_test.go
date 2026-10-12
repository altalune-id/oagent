package webhook

import (
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/oagent/apperror"
)

func TestErrorMessages(t *testing.T) {
	assert.Contains(t, (&NotFoundError{}).Error(), "not found")
	assert.Contains(t, (&NotFoundError{ID: "x"}).Error(), "id=x")
	assert.Contains(t, (&InvalidURLError{Reason: URLHTTPSRequired}).Error(), "must use https")
	assert.Contains(t, (&InvalidEventTypesError{Event: "x.y", Reason: EventTypeDuplicate}).Error(), "x.y is listed twice")
	assert.Contains(t, (&InvalidDescriptionError{Reason: DescriptionNotUTF8}).Error(), "not valid UTF-8")
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

func appErr(t *testing.T, err error) *apperror.AppError {
	t.Helper()
	ae, ok := apperror.AsAppError(err)
	require.True(t, ok, "%v carries no code", err)
	return ae
}

func TestToAppError(t *testing.T) {
	maxURL, maxDesc := strconv.Itoa(MaxURLLen), strconv.Itoa(MaxDescriptionRunes)
	for name, tc := range map[string]struct {
		err  error
		code apperror.Code
		meta map[string]string
	}{
		"not found":      {&NotFoundError{ID: "x"}, apperror.CodeWebhookNotFound, map[string]string{}},
		"url":            {&InvalidURLError{Reason: URLHTTPSRequired}, apperror.CodeWebhookInvalidURL, map[string]string{"reason": "https_required", "max": maxURL}},
		"event types":    {&InvalidEventTypesError{Event: "x.y", Reason: EventTypeUnknown}, apperror.CodeWebhookInvalidEventTypes, map[string]string{"reason": "unknown", "event": "x.y"}},
		"no event types": {&InvalidEventTypesError{Reason: EventTypesRequired}, apperror.CodeWebhookInvalidEventTypes, map[string]string{"reason": "required"}},
		"description":    {&InvalidDescriptionError{Reason: DescriptionTooLong}, apperror.CodeWebhookInvalidDescription, map[string]string{"reason": "too_long", "max": maxDesc}},
		"limit":          {&EndpointLimitError{Limit: 3}, apperror.CodeWebhookEndpointLimit, map[string]string{"limit": "3"}},
		"inactive":       {&EndpointInactiveError{ID: "x"}, apperror.CodeWebhookEndpointInactive, map[string]string{}},
		"secret race":    {&SecretConflictError{ID: "x"}, apperror.CodeWebhookSecretConflict, map[string]string{}},
		"delivery gone":  {&DeliveryNotFoundError{ID: "x"}, apperror.CodeWebhookDeliveryNotFound, map[string]string{}},
		"not retryable":  {&DeliveryNotRetryableError{ID: "x"}, apperror.CodeWebhookDeliveryNotRetryable, map[string]string{}},
	} {
		t.Run(name, func(t *testing.T) {
			ae := appErr(t, tc.err)
			assert.Equal(t, tc.code, ae.Code())
			assert.Equal(t, tc.meta, ae.Meta())
			assert.NotContains(t, ae.Message(), "webhook:", "the wire message is for people, not logs")
			assert.ErrorIs(t, ae, tc.err)
		})
	}
}

func TestInternalErrorsCarryNoCode(t *testing.T) {
	for _, err := range []error{
		&InvalidAttemptError{Field: "f", Reason: "r"},
		&SecretUnavailableError{},
		&DeliveryFailedError{StatusCode: 500},
	} {
		_, ok := apperror.AsAppError(err)
		assert.False(t, ok, "%T must stay internal", err)
	}
}
