package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/apperror"
)

func TestInvalidCredentialsError(t *testing.T) {
	err := &InvalidCredentialsError{}
	assert.True(t, IsInvalidCredentialsError(err))
	assert.False(t, IsInvalidCredentialsError(assert.AnError))
	ae := err.ToAppError()
	assert.Equal(t, apperror.CodeAuthInvalidCredentials, ae.Code())
	assert.Equal(t, "Invalid email or password", ae.Message())
}
