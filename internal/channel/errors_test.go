package channel_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/channel"
)

func TestErrors_MapToTheirCodesAndKeepCausesServerSide(t *testing.T) {
	cause := errors.New("dial tcp 10.0.0.1: refused")
	for _, c := range []struct {
		err  error
		is   func(error) bool
		code apperror.Code
	}{
		{&channel.NotFoundError{}, channel.IsNotFoundError, apperror.CodeConnectionNotFound},
		{&channel.InvalidError{Field: "base_url"}, channel.IsInvalidError, apperror.CodeConnectionInvalid},
		{&channel.SecretRequiredError{}, channel.IsSecretRequiredError, apperror.CodeConnectionSecretRequired},
		{&channel.SecretsUnavailableError{Cause: cause}, channel.IsSecretsUnavailableError, apperror.CodeConnectionSecretsUnavailable},
		{&channel.ProviderUnavailableError{Cause: cause}, channel.IsProviderUnavailableError, apperror.CodeConnectionProviderUnavailable},
		{&channel.WebhookURLUnavailableError{}, channel.IsWebhookURLUnavailableError, apperror.CodeConnectionWebhookURLUnavailable},
	} {
		assert.True(t, c.is(c.err), "%T", c.err)
		assert.False(t, c.is(errors.New("other")), "%T", c.err)
		assert.NotEmpty(t, c.err.Error())
		ae := c.err.(interface{ ToAppError() *apperror.AppError }).ToAppError()
		assert.Equal(t, c.code, ae.Code())
		assert.NotContains(t, ae.Message(), "10.0.0.1", "the cause stays server-side")
	}
	assert.ErrorIs(t, &channel.SecretsUnavailableError{Cause: cause}, cause)
	assert.ErrorIs(t, &channel.ProviderUnavailableError{Cause: cause}, cause)
	assert.Contains(t, (&channel.SecretsUnavailableError{}).Error(), "unknown")
	assert.True(t, channel.IsIgnoredError(&channel.IgnoredError{Reason: "reaction"}))
	assert.Contains(t, (&channel.IgnoredError{Reason: "reaction"}).Error(), "reaction")
	assert.True(t, channel.IsTooLargeError(&channel.TooLargeError{Limit: 5}))
	assert.NotEmpty(t, (&channel.TooLargeError{}).Error())
	assert.True(t, channel.IsMediaTypeError(fmt.Errorf("wrapped: %w", &channel.MediaTypeError{})))
	assert.NotEmpty(t, (&channel.MediaTypeError{}).Error())
}
