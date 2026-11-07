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

// MediaTypeError reports media that is not a JPEG, PNG or WebP image.
type MediaTypeError struct{}

func (e *MediaTypeError) Error() string { return "openwa: unsupported media type" }

// IsMediaTypeError reports whether err's tree contains a *MediaTypeError.
func IsMediaTypeError(err error) bool {
	_, ok := errors.AsType[*MediaTypeError](err)
	return ok
}
