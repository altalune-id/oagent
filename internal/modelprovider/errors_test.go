package modelprovider

import (
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
		{"not found", &NotFoundError{ID: "x"}, apperror.CodeModelProviderNotFound, nil},
		{"already exists", &AlreadyExistsError{Name: "openrouter"}, apperror.CodeModelProviderAlreadyExists, map[string]string{"name": "openrouter"}},
		{"base url", &InvalidBaseURLError{Reason: URLHTTPSRequired}, apperror.CodeModelProviderInvalidBaseURL, map[string]string{"reason": "https_required", "max": "2048"}},
		{"models too many", &InvalidModelsError{Reason: ModelsTooMany}, apperror.CodeModelProviderInvalidModels, map[string]string{"reason": "too_many", "max": "64"}},
		{"models duplicate keeps the model internal", &InvalidModelsError{Reason: ModelsDuplicate, Model: "a"}, apperror.CodeModelProviderInvalidModels, map[string]string{"reason": "duplicate", "max": "64"}},
		{"in use on delete", &InUseError{Reason: InUseDelete}, apperror.CodeModelProviderInUse, map[string]string{"reason": "delete"}},
		{"in use model removed", &InUseError{Reason: InUseModelRemoved, Model: "gpt-4o"}, apperror.CodeModelProviderInUse, map[string]string{"reason": "model_removed"}},
		{"in use price removed", &InUseError{Reason: InUsePriceRemoved, Model: "gpt-4o"}, apperror.CodeModelProviderInUse, map[string]string{"reason": "price_removed"}},
		{"secret required", &SecretRequiredError{}, apperror.CodeModelProviderSecretRequired, map[string]string{"reason": "secret_required"}},
		{"unknown model", &UnknownModelError{Model: "gpt-9"}, apperror.CodeModelProviderUnknownModel, map[string]string{"model": "gpt-9"}},
		{"secrets unavailable", &SecretsUnavailableError{Cause: &sealer.UnavailableError{}}, apperror.CodeModelProviderSecretsUnavailable, nil},
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

func TestErrors_Is(t *testing.T) {
	unavailable := &SecretsUnavailableError{Cause: &sealer.UnavailableError{}}
	cases := []struct {
		name string
		is   func(error) bool
		err  error
	}{
		{"not found", IsNotFoundError, &NotFoundError{}},
		{"already exists", IsAlreadyExistsError, &AlreadyExistsError{}},
		{"base url", IsInvalidBaseURLError, &InvalidBaseURLError{}},
		{"models", IsInvalidModelsError, &InvalidModelsError{}},
		{"in use", IsInUseError, &InUseError{}},
		{"secret required", IsSecretRequiredError, &SecretRequiredError{}},
		{"unknown model", IsUnknownModelError, &UnknownModelError{}},
		{"secrets unavailable", IsSecretsUnavailableError, unavailable},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, tt.is(tt.err))
			assert.False(t, tt.is(assert.AnError))
			assert.False(t, tt.is(nil))
			assert.True(t, IsExpectedError(tt.err))
		})
	}
	assert.False(t, IsExpectedError(assert.AnError))
	assert.False(t, IsExpectedError(nil))
	assert.True(t, sealer.IsUnavailableError(unavailable), "wraps the sealer error")
}

func TestErrors_Text(t *testing.T) {
	assert.Contains(t, (&NotFoundError{}).Error(), "not found")
	assert.Contains(t, (&NotFoundError{ID: "x"}).Error(), "x")
	assert.Contains(t, (&InvalidModelsError{Reason: ModelsDuplicate, Model: "a"}).Error(), "a")
	assert.Contains(t, (&InUseError{Reason: InUseModelRemoved, Model: "gpt-4o"}).Error(), "gpt-4o")
	assert.Contains(t, (&SecretsUnavailableError{}).Error(), "unavailable")
	assert.Contains(t, (&SecretsUnavailableError{Cause: assert.AnError}).Error(), assert.AnError.Error())
}
