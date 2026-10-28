package session

import "errors"

// BadCookieError reports a session cookie that fails its HMAC check.
type BadCookieError struct{}

func (*BadCookieError) Error() string { return "session: cookie signature invalid" }

// IsBadCookieError reports whether err's tree contains a *BadCookieError.
func IsBadCookieError(err error) bool {
	_, ok := errors.AsType[*BadCookieError](err)
	return ok
}

// NotFoundError reports a session that is missing or expired.
type NotFoundError struct{}

func (*NotFoundError) Error() string { return "session: not found" }

// IsNotFoundError reports whether err's tree contains a *NotFoundError.
func IsNotFoundError(err error) bool {
	_, ok := errors.AsType[*NotFoundError](err)
	return ok
}
