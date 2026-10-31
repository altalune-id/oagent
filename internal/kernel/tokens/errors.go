package tokens

// InvalidTokenError signals that verification refused the presented bearer token.
type InvalidTokenError struct {
	Reason string
	Cause  error
}

func (e *InvalidTokenError) Error() string {
	if e.Reason == "" {
		return "tokens: invalid token"
	}
	return "tokens: invalid token: " + e.Reason
}

func (e *InvalidTokenError) Unwrap() error { return e.Cause }
