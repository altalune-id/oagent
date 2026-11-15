package webtool

import (
	"errors"
	"strconv"

	"altalune.id/oagent/apperror"
)

// ProviderUnavailableError reports a search provider kind this deployment does not offer.
type ProviderUnavailableError struct{ Kind Kind }

func (e *ProviderUnavailableError) Error() string {
	return "webtool: search provider unavailable: " + apperror.Echo(string(e.Kind))
}

// IsProviderUnavailableError reports whether err's tree contains a *ProviderUnavailableError.
func IsProviderUnavailableError(err error) bool {
	_, ok := errors.AsType[*ProviderUnavailableError](err)
	return ok
}

// SecretRequiredError reports that an API key is needed, for a new provider or a changed kind.
type SecretRequiredError struct{}

func (*SecretRequiredError) Error() string { return "webtool: api key required" }

// IsSecretRequiredError reports whether err's tree contains a *SecretRequiredError.
func IsSecretRequiredError(err error) bool {
	_, ok := errors.AsType[*SecretRequiredError](err)
	return ok
}

// SecretsUnavailableError reports that keys and secrets cannot be sealed or opened because no encryption key is configured.
type SecretsUnavailableError struct{ Cause error }

func (e *SecretsUnavailableError) Error() string {
	msg := "webtool: secrets unavailable: configure an encryption key"
	if e.Cause == nil {
		return msg
	}
	return msg + ": " + e.Cause.Error()
}

func (e *SecretsUnavailableError) Unwrap() error { return e.Cause }

// IsSecretsUnavailableError reports whether err's tree contains a *SecretsUnavailableError.
func IsSecretsUnavailableError(err error) bool {
	_, ok := errors.AsType[*SecretsUnavailableError](err)
	return ok
}

// NotFoundError reports an unknown or unreachable secret, provider or project.
type NotFoundError struct{}

func (*NotFoundError) Error() string { return "webtool: not found" }

// IsNotFoundError reports whether err's tree contains a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// SecretLimitError reports that the project already holds MaxHTTPSecrets secrets.
type SecretLimitError struct{}

func (*SecretLimitError) Error() string { return "webtool: http secret limit reached" }

// IsSecretLimitError reports whether err's tree contains a *SecretLimitError.
func IsSecretLimitError(err error) bool {
	_, ok := errors.AsType[*SecretLimitError](err)
	return ok
}

// SearchKeyUnsealableError reports a stored search key that will not open, such as one sealed under another encryption key.
type SearchKeyUnsealableError struct{ Cause error }

func (e *SearchKeyUnsealableError) Error() string {
	return "webtool: search key will not open: " + e.Cause.Error()
}

func (e *SearchKeyUnsealableError) Unwrap() error { return e.Cause }

// IsSearchKeyUnsealableError reports whether err's tree contains a *SearchKeyUnsealableError.
func IsSearchKeyUnsealableError(err error) bool {
	_, ok := errors.AsType[*SearchKeyUnsealableError](err)
	return ok
}

// IsExpectedError reports whether err is a typed domain error a service returns as-is.
func IsExpectedError(err error) bool {
	return IsProviderUnavailableError(err) || IsSecretRequiredError(err) || IsSecretsUnavailableError(err) ||
		IsNotFoundError(err) || IsSecretLimitError(err) || IsSearchKeyUnsealableError(err)
}

// ToAppError maps ProviderUnavailableError to its own code with the kind.
func (e *ProviderUnavailableError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeWebSearchProviderUnavailable, "Search provider is not available").
		WithMeta(map[string]string{"kind": apperror.Echo(string(e.Kind))}).WithCause(e)
}

// ToAppError maps SecretRequiredError to its own code.
func (e *SecretRequiredError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeWebSearchSecretRequired, "Search provider API key is required").WithCause(e)
}

// ToAppError maps SecretsUnavailableError to its own code.
func (e *SecretsUnavailableError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeWebSecretsUnavailable, "Web tool secrets are unavailable").WithCause(e)
}

// ToAppError maps NotFoundError to its own code.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeHTTPSecretNotFound, "Not found").WithCause(e)
}

// ToAppError maps SecretLimitError to its own code with the limit.
func (e *SecretLimitError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeHTTPSecretLimitReached, "HTTP secret limit reached").
		WithMeta(map[string]string{"max": strconv.Itoa(MaxHTTPSecrets)}).WithCause(e)
}
