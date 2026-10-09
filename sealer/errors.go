package sealer

import (
	"errors"
	"fmt"
)

// UnavailableError reports that no encryption key is configured.
type UnavailableError struct{}

func (*UnavailableError) Error() string { return "sealer: encryption unavailable: no key configured" }

// IsUnavailableError reports whether err's tree contains an *UnavailableError.
func IsUnavailableError(err error) bool {
	_, ok := errors.AsType[*UnavailableError](err)
	return ok
}

// InvalidKeyError reports that the configured key is not KeyLen bytes.
type InvalidKeyError struct{ Len int }

func (e *InvalidKeyError) Error() string {
	return fmt.Sprintf("sealer: key: got %d bytes, want %d", e.Len, KeyLen)
}

// IsInvalidKeyError reports whether err's tree contains an *InvalidKeyError.
func IsInvalidKeyError(err error) bool {
	_, ok := errors.AsType[*InvalidKeyError](err)
	return ok
}

// OpenFailedError reports that a ciphertext could not be authenticated or decrypted.
type OpenFailedError struct{ Cause error }

// SECURITY: the message names no key material and no plaintext.
func (e *OpenFailedError) Error() string {
	if e.Cause == nil {
		return "sealer: open: ciphertext failed authentication"
	}
	return "sealer: open: ciphertext failed authentication: " + e.Cause.Error()
}

func (e *OpenFailedError) Unwrap() error { return e.Cause }

// IsOpenFailedError reports whether err's tree contains an *OpenFailedError.
func IsOpenFailedError(err error) bool {
	_, ok := errors.AsType[*OpenFailedError](err)
	return ok
}
