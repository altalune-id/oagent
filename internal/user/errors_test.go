package user

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/apperror"
)

func TestErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		err interface {
			error
			ToAppError() *apperror.AppError
		}
		is   func(error) bool
		code apperror.Code
	}{
		"not found":     {&NotFoundError{}, IsNotFoundError, apperror.CodeUserNotFound},
		"already exist": {&AlreadyExistsError{Email: "a@example.com"}, IsAlreadyExistsError, apperror.CodeUserAlreadyExists},
		"invalid email": {&InvalidEmailError{}, IsInvalidEmailError, apperror.CodeUserInvalidEmail},
		"invalid name":  {&InvalidNameError{}, IsInvalidNameError, apperror.CodeUserInvalidName},
		"unknown email": {&UnknownEmailError{Email: "a@example.com"}, IsUnknownEmailError, apperror.CodeUserNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			assert.True(t, tc.is(tc.err))
			assert.True(t, IsExpectedError(tc.err))
			assert.NotEmpty(t, tc.err.Error())
			assert.Equal(t, tc.code, tc.err.ToAppError().Code())
		})
	}
	assert.False(t, IsExpectedError(errors.New("boom")))
}

func TestUnknownEmailError(t *testing.T) {
	err := &UnknownEmailError{Email: "ada@example.com"}

	assert.True(t, IsNotFoundError(err), "it is a not-found")
	assert.NotContains(t, err.Error(), "ada@example.com", "the address stays out of logs")
	assert.Equal(t, "No user with email ada@example.com", err.ToAppError().Message())
	assert.False(t, IsUnknownEmailError(&NotFoundError{}))
}
