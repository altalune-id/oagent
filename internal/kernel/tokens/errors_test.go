package tokens_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/internal/kernel/tokens"
)

var uuid0 = uuid.Nil

func TestInvalidTokenError(t *testing.T) {
	cause := errors.New("signature invalid")
	err := &tokens.InvalidTokenError{Reason: "bad sig", Cause: cause}
	assert.Equal(t, "tokens: invalid token: bad sig", err.Error())
	assert.ErrorIs(t, err, cause)
	assert.Equal(t, "tokens: invalid token", (&tokens.InvalidTokenError{}).Error())
}
