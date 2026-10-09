package apikey

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/apperror"
)

func TestErrors(t *testing.T) {
	assert.Contains(t, (*NotFoundError)(nil).Error(), "not found")
	assert.Contains(t, (&NotFoundError{ID: "x"}).Error(), "x")
	assert.True(t, IsNotFoundError(&NotFoundError{}))

	assert.Contains(t, (&InvalidNameError{}).Error(), "name")
	assert.Contains(t, (&InvalidNameError{Reason: "r"}).Error(), "r")
	assert.Contains(t, (&UnknownScopeError{Scope: "s"}).Error(), "s")
	assert.Contains(t, (&ScopeLevelError{Scope: "s"}).Error(), "s")
	assert.Contains(t, (&ExpiryRequiredError{}).Error(), "expires_at")
	assert.Contains(t, (&ExpiryInPastError{}).Error(), "future")
	assert.Contains(t, (&ExpiryTooLongError{}).Error(), "year")
	assert.Contains(t, (&GrantConflictError{}).Error(), "grant")
	assert.Contains(t, (&EmptyGrantError{}).Error(), "grant")

	assert.True(t, IsExpectedError(&NotFoundError{}))
	assert.True(t, IsExpectedError(&InvalidNameError{}))
	assert.True(t, IsExpectedError(&UnknownScopeError{}))
	assert.True(t, IsExpectedError(&ScopeLevelError{}))
	assert.True(t, IsExpectedError(&ExpiryRequiredError{}))
	assert.True(t, IsExpectedError(&ExpiryInPastError{}))
	assert.True(t, IsExpectedError(&ExpiryTooLongError{}))
	assert.True(t, IsExpectedError(&GrantConflictError{}))
	assert.True(t, IsExpectedError(&EmptyGrantError{}))
	assert.False(t, IsExpectedError(assert.AnError))
}

func TestToAppError(t *testing.T) {
	assert.Equal(t, apperror.CodeAPIKeyNotFound, (&NotFoundError{}).ToAppError().Code())
	assert.Equal(t, apperror.CodeAPIKeyInvalidName, (&InvalidNameError{}).ToAppError().Code())
	assert.Equal(t, apperror.CodeAPIKeyInvalidScope, (&UnknownScopeError{}).ToAppError().Code())
	assert.Equal(t, apperror.CodeAPIKeyInvalidScope, (&ScopeLevelError{}).ToAppError().Code())
	assert.Equal(t, apperror.CodeAPIKeyInvalidExpiry, (&ExpiryRequiredError{}).ToAppError().Code())
	assert.Equal(t, apperror.CodeAPIKeyInvalidExpiry, (&ExpiryInPastError{}).ToAppError().Code())
	assert.Equal(t, apperror.CodeAPIKeyInvalidExpiry, (&ExpiryTooLongError{}).ToAppError().Code())
	assert.Equal(t, apperror.CodeAPIKeyInvalidGrant, (&GrantConflictError{}).ToAppError().Code())
	assert.Equal(t, apperror.CodeAPIKeyInvalidGrant, (&EmptyGrantError{}).ToAppError().Code())
}
