package ingest

import "errors"

// UnverifiedError reports a delivery that is not authenticated: an unknown provider or connection, or a signature that does not verify.
type UnverifiedError struct{}

func (*UnverifiedError) Error() string { return "ingest: unverified" }

// IsUnverifiedError reports whether err's tree contains an *UnverifiedError.
func IsUnverifiedError(err error) bool {
	_, ok := errors.AsType[*UnverifiedError](err)
	return ok
}

// PayloadTooLargeError reports a delivery body over the surface's byte limit.
type PayloadTooLargeError struct{}

func (*PayloadTooLargeError) Error() string { return "ingest: payload too large" }

// IsPayloadTooLargeError reports whether err's tree contains a *PayloadTooLargeError.
func IsPayloadTooLargeError(err error) bool {
	_, ok := errors.AsType[*PayloadTooLargeError](err)
	return ok
}

// BadRequestError reports a delivery body that could not be read.
type BadRequestError struct{}

func (*BadRequestError) Error() string { return "ingest: bad request" }

// IsBadRequestError reports whether err's tree contains a *BadRequestError.
func IsBadRequestError(err error) bool {
	_, ok := errors.AsType[*BadRequestError](err)
	return ok
}

// UnavailableError reports a connection lookup that failed, so the provider should retry.
type UnavailableError struct{}

func (*UnavailableError) Error() string { return "ingest: unavailable" }

// IsUnavailableError reports whether err's tree contains an *UnavailableError.
func IsUnavailableError(err error) bool {
	_, ok := errors.AsType[*UnavailableError](err)
	return ok
}

// NoHandlerError reports a registered provider whose verified deliveries have nowhere to go.
type NoHandlerError struct{}

func (*NoHandlerError) Error() string { return "ingest: provider has no handler" }

// IsNoHandlerError reports whether err's tree contains a *NoHandlerError.
func IsNoHandlerError(err error) bool {
	_, ok := errors.AsType[*NoHandlerError](err)
	return ok
}

// RateLimitedError reports a connection over its delivery rate; the answer carries Retry-After.
type RateLimitedError struct{ RetryAfterSeconds int }

func (*RateLimitedError) Error() string { return "ingest: rate limited" }

// IsRateLimitedError reports whether err's tree contains a *RateLimitedError.
func IsRateLimitedError(err error) bool {
	_, ok := errors.AsType[*RateLimitedError](err)
	return ok
}
