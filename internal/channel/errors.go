// Package channel connects agents to messaging channels such as WhatsApp through OpenWA.
package channel

import (
	"errors"

	"altalune.id/oagent/apperror"
)

// NotFoundError reports a connection that does not exist or that the caller does not reach.
type NotFoundError struct{}

func (e *NotFoundError) Error() string { return "channel: connection not found" }

// IsNotFoundError reports whether err's tree contains a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// ToAppError maps NotFoundError to its own code.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeConnectionNotFound, "Channel connection not found").WithCause(e)
}

// InvalidError reports a connection setting that is not valid; Field names it.
type InvalidError struct{ Field string }

func (e *InvalidError) Error() string { return "channel: invalid " + e.Field }

// IsInvalidError reports whether err's tree contains an *InvalidError.
func IsInvalidError(err error) bool {
	_, ok := errors.AsType[*InvalidError](err)
	return ok
}

// ToAppError maps InvalidError to its own code, naming the field.
func (e *InvalidError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeConnectionInvalid, "The connection setting "+e.Field+" is not valid").
		WithMeta(map[string]string{"field": e.Field}).WithCause(e)
}

// SecretRequiredError reports a connection created without a signing secret.
type SecretRequiredError struct{}

func (e *SecretRequiredError) Error() string { return "channel: signing secret required" }

// IsSecretRequiredError reports whether err's tree contains a *SecretRequiredError.
func IsSecretRequiredError(err error) bool {
	_, ok := errors.AsType[*SecretRequiredError](err)
	return ok
}

// ToAppError maps SecretRequiredError to its own code.
func (e *SecretRequiredError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeConnectionSecretRequired, "A signing secret is required").WithCause(e)
}

// SecretsUnavailableError reports sealed secrets that cannot be opened; the cause stays server-side.
type SecretsUnavailableError struct{ Cause error }

func (e *SecretsUnavailableError) Error() string {
	return "channel: secrets unavailable: " + errText(e.Cause)
}

func (e *SecretsUnavailableError) Unwrap() error { return e.Cause }

// IsSecretsUnavailableError reports whether err's tree contains a *SecretsUnavailableError.
func IsSecretsUnavailableError(err error) bool {
	_, ok := errors.AsType[*SecretsUnavailableError](err)
	return ok
}

// ToAppError maps SecretsUnavailableError to its own code; the cause stays server-side.
func (e *SecretsUnavailableError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeConnectionSecretsUnavailable, "The connection's secrets can't be opened right now").WithCause(e)
}

// ProviderUnavailableError reports a failed call to the channel provider; the cause stays server-side.
type ProviderUnavailableError struct{ Cause error }

func (e *ProviderUnavailableError) Error() string {
	return "channel: provider unavailable: " + errText(e.Cause)
}

func (e *ProviderUnavailableError) Unwrap() error { return e.Cause }

// IsProviderUnavailableError reports whether err's tree contains a *ProviderUnavailableError.
func IsProviderUnavailableError(err error) bool {
	_, ok := errors.AsType[*ProviderUnavailableError](err)
	return ok
}

// ToAppError maps ProviderUnavailableError to its own code; the cause stays server-side.
func (e *ProviderUnavailableError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeConnectionProviderUnavailable, "OpenWA could not be reached").WithCause(e)
}

func errText(err error) string {
	if err == nil {
		return "unknown"
	}
	return err.Error()
}

// KeyRefusedError reports the provider refusing the connection's key.
type KeyRefusedError struct{}

func (e *KeyRefusedError) Error() string { return "channel: the provider refused the connection's key" }

// IsKeyRefusedError reports whether err's tree contains a *KeyRefusedError.
func IsKeyRefusedError(err error) bool {
	_, ok := errors.AsType[*KeyRefusedError](err)
	return ok
}

// WebhookURLUnavailableError reports a server with no public URL, so OpenWA has nowhere to send deliveries.
type WebhookURLUnavailableError struct{}

func (e *WebhookURLUnavailableError) Error() string { return "channel: no public URL for the webhook" }

// IsWebhookURLUnavailableError reports whether err's tree contains a *WebhookURLUnavailableError.
func IsWebhookURLUnavailableError(err error) bool {
	_, ok := errors.AsType[*WebhookURLUnavailableError](err)
	return ok
}

// ToAppError maps WebhookURLUnavailableError to its own code.
func (e *WebhookURLUnavailableError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeConnectionWebhookURLUnavailable, "This server has no public URL for OpenWA to send to")
}
