package openwa

import "errors"

// MessageIDError reports a message id that is not an OpenWA message public id, refused before any request.
type MessageIDError struct{}

func (e *MessageIDError) Error() string { return "openwa: invalid message id" }

// IsMessageIDError reports whether err's tree contains a *MessageIDError.
func IsMessageIDError(err error) bool {
	_, ok := errors.AsType[*MessageIDError](err)
	return ok
}
