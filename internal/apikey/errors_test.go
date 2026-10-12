package apikey

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	assert.Contains(t, (&PersonOnlyScopeError{Scope: "s"}).Error(), "s")
	assert.Contains(t, (&ExpiryRequiredError{}).Error(), "expires_at")
	assert.Contains(t, (&ExpiryInPastError{}).Error(), "future")
	assert.Contains(t, (&ExpiryTooLongError{}).Error(), "366 days")
	assert.Contains(t, (&GrantConflictError{}).Error(), "grant")
	assert.Contains(t, (&EmptyGrantError{}).Error(), "grant")

	assert.True(t, IsExpectedError(&NotFoundError{}))
	assert.True(t, IsExpectedError(&InvalidNameError{}))
	assert.True(t, IsExpectedError(&UnknownScopeError{}))
	assert.True(t, IsExpectedError(&ScopeLevelError{}))
	assert.True(t, IsExpectedError(&PersonOnlyScopeError{}))
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
	assert.Equal(t, apperror.CodeAPIKeyPersonOnlyScope, (&PersonOnlyScopeError{}).ToAppError().Code())
	assert.Contains(t, (&PersonOnlyScopeError{Scope: "s"}).ToAppError().Message(), "can only be held by a person")
	assert.Equal(t, apperror.CodeAPIKeyInvalidExpiry, (&ExpiryRequiredError{}).ToAppError().Code())
	assert.Equal(t, apperror.CodeAPIKeyInvalidExpiry, (&ExpiryInPastError{}).ToAppError().Code())
	assert.Equal(t, apperror.CodeAPIKeyInvalidExpiry, (&ExpiryTooLongError{}).ToAppError().Code())
	assert.Equal(t, apperror.CodeAPIKeyInvalidGrant, (&GrantConflictError{}).ToAppError().Code())
	assert.Equal(t, apperror.CodeAPIKeyInvalidGrant, (&EmptyGrantError{}).ToAppError().Code())
}

func TestToAppErrorMeta(t *testing.T) {
	for name, tc := range map[string]struct {
		err  interface{ ToAppError() *apperror.AppError }
		code apperror.Code
		meta map[string]string
	}{
		"name required": {&InvalidNameError{Reason: NameRequired}, apperror.CodeAPIKeyInvalidName, map[string]string{"reason": "required", "max": "100"}},
		"name length":   {&InvalidNameError{Reason: NameLength}, apperror.CodeAPIKeyInvalidName, map[string]string{"reason": "length", "max": "100"}},
		"unknown scope": {&UnknownScopeError{Scope: "x:y"}, apperror.CodeAPIKeyInvalidScope, map[string]string{"reason": "unknown", "scope": "x:y"}},
		"scope level":   {&ScopeLevelError{Scope: "org:write"}, apperror.CodeAPIKeyInvalidScope, map[string]string{"reason": "level", "scope": "org:write"}},
		"person only":   {&PersonOnlyScopeError{Scope: "member:write"}, apperror.CodeAPIKeyPersonOnlyScope, map[string]string{"scope": "member:write"}},
		"no expiry":     {&ExpiryRequiredError{}, apperror.CodeAPIKeyInvalidExpiry, map[string]string{"reason": "required", "max_days": "366"}},
		"past":          {&ExpiryInPastError{}, apperror.CodeAPIKeyInvalidExpiry, map[string]string{"reason": "past", "max_days": "366"}},
		"too far":       {&ExpiryTooLongError{}, apperror.CodeAPIKeyInvalidExpiry, map[string]string{"reason": "too_far", "max_days": "366"}},
		"conflict":      {&GrantConflictError{}, apperror.CodeAPIKeyInvalidGrant, map[string]string{"reason": "conflict"}},
		"empty":         {&EmptyGrantError{}, apperror.CodeAPIKeyInvalidGrant, map[string]string{"reason": "empty"}},
		"outside org":   {&GrantOutsideOrgError{}, apperror.CodeAPIKeyInvalidGrant, map[string]string{"reason": "outside_org"}},
		"above role":    {&ScopeExceedsRoleError{Scope: "org:write"}, apperror.CodeAPIKeyScopeExceedsRole, map[string]string{"scope": "org:write"}},
		"not a member":  {&ScopeExceedsRoleError{}, apperror.CodeAPIKeyScopeExceedsRole, map[string]string{"reason": "not_member"}},
	} {
		t.Run(name, func(t *testing.T) {
			ae := tc.err.ToAppError()
			assert.Equal(t, tc.code, ae.Code())
			assert.Equal(t, tc.meta, ae.Meta())
		})
	}
	assert.True(t, IsGrantError(&GrantOutsideOrgError{}))
	assert.Equal(t, NameRequired, mustNameErr(t, ValidateName(" ")).Reason)
	assert.Equal(t, NameLength, mustNameErr(t, ValidateName(strings.Repeat("n", 101))).Reason)
}

func mustNameErr(t *testing.T, err error) *InvalidNameError {
	t.Helper()
	e, ok := errors.AsType[*InvalidNameError](err)
	require.True(t, ok)
	return e
}
