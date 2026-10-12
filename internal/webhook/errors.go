package webhook

import (
	"errors"
	"fmt"
	"strconv"

	"altalune.id/oagent/apperror"
	"altalune.id/oagent/internal/kernel/events"
)

// NotFoundError reports that no endpoint matched the lookup.
type NotFoundError struct{ ID string }

func (e *NotFoundError) Error() string {
	if e.ID == "" {
		return "webhook: endpoint not found"
	}
	return fmt.Sprintf("webhook: endpoint not found: id=%s", e.ID)
}

// ToAppError maps NotFoundError to its own code.
func (e *NotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeWebhookNotFound, "Webhook not found").WithCause(e)
}

// IsNotFoundError reports whether err's tree contains a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}

// URLReason is the stable cause of an InvalidURLError, sent to clients as meta.reason.
type URLReason string

// The causes of an InvalidURLError.
const (
	URLRequired       URLReason = "required"
	URLTooLong        URLReason = "too_long"
	URLNotUTF8        URLReason = "not_utf8"
	URLUnparsable     URLReason = "unparsable"
	URLHTTPSRequired  URLReason = "https_required"
	URLHasCredentials URLReason = "has_credentials"
	URLNoHost         URLReason = "no_host"
)

func (r URLReason) text() string {
	switch r {
	case URLRequired:
		return "is required"
	case URLTooLong:
		return "is longer than " + strconv.Itoa(MaxURLLen) + " characters"
	case URLNotUTF8:
		return "is not valid UTF-8"
	case URLUnparsable:
		return "cannot be parsed"
	case URLHTTPSRequired:
		return "must use https"
	case URLHasCredentials:
		return "must not carry credentials"
	case URLNoHost:
		return "has no host"
	default:
		return "is invalid"
	}
}

// InvalidURLError reports that the endpoint URL breaks its invariants.
type InvalidURLError struct{ Reason URLReason }

func (e *InvalidURLError) Error() string {
	return "webhook: invalid endpoint url: " + e.Reason.text()
}

// ToAppError maps InvalidURLError to its own code with the reason and the length limit.
func (e *InvalidURLError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeWebhookInvalidURL, "Webhook URL "+e.Reason.text()).
		WithMeta(map[string]string{"reason": string(e.Reason), "max": strconv.Itoa(MaxURLLen)}).
		WithCause(e)
}

// IsInvalidURLError reports whether err's tree contains an *InvalidURLError.
func IsInvalidURLError(err error) bool {
	_, ok := errors.AsType[*InvalidURLError](err)
	return ok
}

// EventTypesReason is the stable cause of an InvalidEventTypesError, sent to clients as meta.reason.
type EventTypesReason string

// The causes of an InvalidEventTypesError.
const (
	EventTypesRequired       EventTypesReason = "required"
	EventTypeUnknown         EventTypesReason = "unknown"
	EventTypeNotSubscribable EventTypesReason = "not_subscribable"
	EventTypeDuplicate       EventTypesReason = "duplicate"
)

// InvalidEventTypesError reports that the subscribed event types break their invariants; Event is the offending type, if any.
type InvalidEventTypesError struct {
	Event  events.Type
	Reason EventTypesReason
}

func (e *InvalidEventTypesError) Error() string {
	return "webhook: invalid event types: " + e.text()
}

// ToAppError maps InvalidEventTypesError to its own code with the reason and the offending type.
func (e *InvalidEventTypesError) ToAppError() *apperror.AppError {
	meta := map[string]string{"reason": string(e.Reason)}
	if e.Event != "" {
		meta["event"] = apperror.Echo(string(e.Event))
	}
	return apperror.New(apperror.CodeWebhookInvalidEventTypes, "Webhook event types: "+e.text()).WithMeta(meta).WithCause(e)
}

func (e *InvalidEventTypesError) text() string {
	switch e.Reason {
	case EventTypesRequired:
		return "at least one event type is required"
	case EventTypeUnknown:
		return "unknown event type " + apperror.Echo(string(e.Event))
	case EventTypeNotSubscribable:
		return "event type " + apperror.Echo(string(e.Event)) + " cannot be subscribed to"
	case EventTypeDuplicate:
		return "event type " + apperror.Echo(string(e.Event)) + " is listed twice"
	default:
		return "invalid event types"
	}
}

// IsInvalidEventTypesError reports whether err's tree contains an *InvalidEventTypesError.
func IsInvalidEventTypesError(err error) bool {
	_, ok := errors.AsType[*InvalidEventTypesError](err)
	return ok
}

// DescriptionReason is the stable cause of an InvalidDescriptionError, sent to clients as meta.reason.
type DescriptionReason string

// The causes of an InvalidDescriptionError.
const (
	DescriptionNotUTF8 DescriptionReason = "not_utf8"
	DescriptionTooLong DescriptionReason = "too_long"
)

func (r DescriptionReason) text() string {
	switch r {
	case DescriptionNotUTF8:
		return "is not valid UTF-8"
	case DescriptionTooLong:
		return "is longer than " + strconv.Itoa(MaxDescriptionRunes) + " characters"
	default:
		return "is invalid"
	}
}

// InvalidDescriptionError reports that the endpoint description breaks its invariants.
type InvalidDescriptionError struct{ Reason DescriptionReason }

func (e *InvalidDescriptionError) Error() string {
	return "webhook: invalid endpoint description: " + e.Reason.text()
}

// ToAppError maps InvalidDescriptionError to its own code with the reason and the length limit.
func (e *InvalidDescriptionError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeWebhookInvalidDescription, "Webhook description "+e.Reason.text()).
		WithMeta(map[string]string{"reason": string(e.Reason), "max": strconv.Itoa(MaxDescriptionRunes)}).
		WithCause(e)
}

// IsInvalidDescriptionError reports whether err's tree contains an *InvalidDescriptionError.
func IsInvalidDescriptionError(err error) bool {
	_, ok := errors.AsType[*InvalidDescriptionError](err)
	return ok
}

// EndpointLimitError reports that the project already has the maximum number of endpoints.
type EndpointLimitError struct{ Limit int }

func (e *EndpointLimitError) Error() string {
	return fmt.Sprintf("webhook: endpoint limit reached: limit=%d", e.Limit)
}

// ToAppError maps EndpointLimitError to its own code with the limit.
func (e *EndpointLimitError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeWebhookEndpointLimit, fmt.Sprintf("This project already has the maximum of %d webhooks", e.Limit)).
		WithMeta(map[string]string{"limit": strconv.Itoa(e.Limit)}).
		WithCause(e)
}

// IsEndpointLimitError reports whether err's tree contains an *EndpointLimitError.
func IsEndpointLimitError(err error) bool {
	_, ok := errors.AsType[*EndpointLimitError](err)
	return ok
}

// EndpointInactiveError reports an operation refused because the endpoint is disabled.
type EndpointInactiveError struct{ ID string }

func (e *EndpointInactiveError) Error() string {
	return fmt.Sprintf("webhook: endpoint inactive: id=%s", e.ID)
}

// ToAppError maps EndpointInactiveError to its own code.
func (e *EndpointInactiveError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeWebhookEndpointInactive, "The webhook is inactive; activate it first").WithCause(e)
}

// IsEndpointInactiveError reports whether err's tree contains an *EndpointInactiveError.
func IsEndpointInactiveError(err error) bool {
	_, ok := errors.AsType[*EndpointInactiveError](err)
	return ok
}

// SecretConflictError reports a secret write refused because the stored secrets changed since they were loaded.
type SecretConflictError struct{ ID string }

func (e *SecretConflictError) Error() string {
	return fmt.Sprintf("webhook: secret changed concurrently: id=%s", e.ID)
}

// ToAppError maps SecretConflictError to its own code; a retry reloads the secrets.
func (e *SecretConflictError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeWebhookSecretConflict, "The webhook secret was just changed elsewhere; try again").WithCause(e)
}

// IsSecretConflictError reports whether err's tree contains a *SecretConflictError.
func IsSecretConflictError(err error) bool {
	_, ok := errors.AsType[*SecretConflictError](err)
	return ok
}

// InvalidAttemptError reports an attempt field that cannot be persisted under the tenant scope on ctx.
type InvalidAttemptError struct {
	Field  string
	Reason string
}

func (e *InvalidAttemptError) Error() string {
	return fmt.Sprintf("webhook: invalid attempt: %s %s", e.Field, e.Reason)
}

// IsInvalidAttemptError reports whether err's tree contains an *InvalidAttemptError.
func IsInvalidAttemptError(err error) bool {
	_, ok := errors.AsType[*InvalidAttemptError](err)
	return ok
}

// SecretUnavailableError reports that the endpoint's signing secret could not be opened.
type SecretUnavailableError struct{ Cause error }

func (e *SecretUnavailableError) Error() string {
	msg := "webhook: signing secret unavailable: rotate the endpoint secret to recover"
	if e.Cause == nil {
		return msg
	}
	return msg + ": " + e.Cause.Error()
}

func (e *SecretUnavailableError) Unwrap() error { return e.Cause }

// IsSecretUnavailableError reports whether err's tree contains a *SecretUnavailableError.
func IsSecretUnavailableError(err error) bool {
	_, ok := errors.AsType[*SecretUnavailableError](err)
	return ok
}

// DeliveryFailedError reports that the receiver answered with a non-2xx status.
type DeliveryFailedError struct{ StatusCode int }

func (e *DeliveryFailedError) Error() string {
	return fmt.Sprintf("webhook: delivery failed: status=%d", e.StatusCode)
}

// IsDeliveryFailedError reports whether err's tree contains a *DeliveryFailedError.
func IsDeliveryFailedError(err error) bool {
	_, ok := errors.AsType[*DeliveryFailedError](err)
	return ok
}

// DeliveryNotFoundError reports that no delivery of the endpoint matched the lookup.
type DeliveryNotFoundError struct{ ID string }

func (e *DeliveryNotFoundError) Error() string {
	return fmt.Sprintf("webhook: delivery not found: id=%s", e.ID)
}

// ToAppError maps DeliveryNotFoundError to its own code.
func (e *DeliveryNotFoundError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeWebhookDeliveryNotFound, "Delivery not found").WithCause(e)
}

// IsDeliveryNotFoundError reports whether err's tree contains a *DeliveryNotFoundError.
func IsDeliveryNotFoundError(err error) bool {
	_, ok := errors.AsType[*DeliveryNotFoundError](err)
	return ok
}

// DeliveryNotRetryableError reports a retry refused because the delivery has not failed.
type DeliveryNotRetryableError struct{ ID string }

func (e *DeliveryNotRetryableError) Error() string {
	return fmt.Sprintf("webhook: delivery not retryable: id=%s", e.ID)
}

// ToAppError maps DeliveryNotRetryableError to its own code.
func (e *DeliveryNotRetryableError) ToAppError() *apperror.AppError {
	return apperror.New(apperror.CodeWebhookDeliveryNotRetryable, "Only failed deliveries can be retried").WithCause(e)
}

// IsDeliveryNotRetryableError reports whether err's tree contains a *DeliveryNotRetryableError.
func IsDeliveryNotRetryableError(err error) bool {
	_, ok := errors.AsType[*DeliveryNotRetryableError](err)
	return ok
}
