package resend

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// APIError reports a non-2xx answer from the Resend API.
type APIError struct {
	StatusCode int
	RetryAfter time.Duration
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("mailer: resend: api status %d: %s", e.StatusCode, e.Body)
}

// HTTPStatus returns the API's HTTP status code.
func (e *APIError) HTTPStatus() int { return e.StatusCode }

// RetryDelay returns the wait the API asked for in Retry-After, or zero.
func (e *APIError) RetryDelay() time.Duration { return e.RetryAfter }

// Retryable reports whether the status is a rate limit or a server fault, so a later send may succeed.
func (e *APIError) Retryable() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= http.StatusInternalServerError
}

// IsAPIError reports whether err's tree contains an *APIError.
func IsAPIError(err error) bool {
	_, ok := errors.AsType[*APIError](err)
	return ok
}
