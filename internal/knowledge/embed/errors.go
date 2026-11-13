package embed

import "errors"

// Reason is the stable cause of an Error.
type Reason string

// The causes of an Error.
const (
	Unreachable Reason = "unreachable"
	Rejected    Reason = "rejected"
	WrongDims   Reason = "wrong_dims"
)

// Error reports a failed embeddings call; Cause is for server logs only.
type Error struct {
	Reason Reason
	Cause  error
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return "embed: " + string(e.Reason)
	}
	return "embed: " + string(e.Reason) + ": " + e.Cause.Error()
}

func (e *Error) Unwrap() error { return e.Cause }

// AsError returns the *Error in err's tree, if any.
func AsError(err error) (*Error, bool) {
	return errors.AsType[*Error](err)
}
