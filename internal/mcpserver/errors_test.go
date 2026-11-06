package mcpserver

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/sealer"
)

func TestErrors_ToAppError(t *testing.T) {
	cases := []struct {
		name string
		err  interface {
			error
			ToAppError() *apperror.AppError
		}
		code apperror.Code
		meta map[string]string
	}{
		{"not found", &NotFoundError{ID: "x"}, apperror.CodeMcpServerNotFound, nil},
		{"already exists", &AlreadyExistsError{Name: "yasaku"}, apperror.CodeMcpServerAlreadyExists, map[string]string{"name": "yasaku"}},
		{"url", &InvalidURLError{Reason: URLHTTPSRequired}, apperror.CodeMcpServerInvalidURL, map[string]string{"reason": "https_required", "max": "2048"}},
		{"in use", &InUseError{}, apperror.CodeMcpServerInUse, nil},
		{"unreachable", &UnreachableError{Cause: assertErr{}}, apperror.CodeMcpServerUnreachable, nil},
		{"secret required", &SecretRequiredError{Reason: SecretValueRequired}, apperror.CodeMcpServerSecretRequired, map[string]string{"reason": "secret_required"}},
		{"header name", &SecretRequiredError{Reason: SecretHeaderName}, apperror.CodeMcpServerSecretRequired, map[string]string{"reason": "header_name"}},
		{"secrets unavailable", &SecretsUnavailableError{Cause: &sealer.UnavailableError{}}, apperror.CodeMcpServerSecretsUnavailable, nil},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ae := tt.err.ToAppError()
			assert.Equal(t, tt.code, ae.Code())
			if tt.meta == nil {
				assert.Empty(t, ae.Meta())
			} else {
				assert.Equal(t, tt.meta, ae.Meta())
			}
			assert.ErrorIs(t, ae, tt.err)
			assert.NotEmpty(t, tt.err.Error())
		})
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "dial tcp: refused" }

func TestErrors_Is(t *testing.T) {
	unavailable := &SecretsUnavailableError{Cause: &sealer.UnavailableError{}}
	for name, c := range map[string]struct {
		is  func(error) bool
		err error
	}{
		"not found":           {IsNotFoundError, &NotFoundError{}},
		"already exists":      {IsAlreadyExistsError, &AlreadyExistsError{}},
		"url":                 {IsInvalidURLError, &InvalidURLError{}},
		"in use":              {IsInUseError, &InUseError{}},
		"unreachable":         {IsUnreachableError, &UnreachableError{}},
		"secret required":     {IsSecretRequiredError, &SecretRequiredError{}},
		"secrets unavailable": {IsSecretsUnavailableError, unavailable},
	} {
		t.Run(name, func(t *testing.T) {
			assert.True(t, c.is(c.err))
			assert.False(t, c.is(assertErr{}))
			assert.False(t, c.is(nil))
			assert.True(t, IsExpectedError(c.err))
		})
	}
	assert.False(t, IsExpectedError(assertErr{}))
	assert.True(t, sealer.IsUnavailableError(unavailable))
}

func TestErrors_TextEchoesCappedInput(t *testing.T) {
	long := strings.Repeat("x", 10*apperror.EchoLimit)
	for _, err := range []error{&AlreadyExistsError{Name: long}, &NotFoundError{ID: long}} {
		assert.Less(t, len(err.Error()), 2*apperror.EchoLimit, "%T", err)
	}
	assert.Contains(t, (&UnreachableError{Cause: assertErr{}}).Error(), "refused")
	assert.Contains(t, (&UnreachableError{}).Error(), "unreachable")
	assert.Contains(t, (&SecretsUnavailableError{}).Error(), "unavailable")
}

func TestAuthRefusedError(t *testing.T) {
	err := &AuthRefusedError{Status: 401}
	assert.Equal(t, apperror.CodeMcpServerAuthRefused, err.ToAppError().Code())
	assert.Empty(t, err.ToAppError().Meta())
	assert.True(t, IsAuthRefusedError(err))
	assert.True(t, IsExpectedError(err))
	assert.False(t, IsAuthRefusedError(assertErr{}))
	assert.Contains(t, err.Error(), "401")
}
