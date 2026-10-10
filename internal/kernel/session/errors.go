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
