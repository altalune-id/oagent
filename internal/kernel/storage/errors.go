package storage

import (
	"errors"
	"io"
	"strconv"
)

// NotConfiguredError signals that this server has no file storage.
type NotConfiguredError struct{}

func (*NotConfiguredError) Error() string { return "storage: file storage is not configured" }

// IsNotConfiguredError reports whether err's tree contains a *NotConfiguredError.
func IsNotConfiguredError(err error) bool {
	_, ok := errors.AsType[*NotConfiguredError](err)
	return ok
}

// MemoryInProductionError signals the memory driver configured for production.
type MemoryInProductionError struct{}

func (*MemoryInProductionError) Error() string {
	return "storage: the memory driver is for development and tests, not production"
}

// IsMemoryInProductionError reports whether err's tree contains a *MemoryInProductionError.
func IsMemoryInProductionError(err error) bool {
	_, ok := errors.AsType[*MemoryInProductionError](err)
	return ok
}

// UnknownDriverError signals a storage.driver value that names no driver.
type UnknownDriverError struct{ Driver string }

func (e *UnknownDriverError) Error() string {
	return "storage: unknown driver " + strconv.Quote(e.Driver)
}

// ObjectNotFoundError signals that storage holds no object under the key.
type ObjectNotFoundError struct{ Key string }

func (e *ObjectNotFoundError) Error() string { return "storage: no object at " + e.Key }

// IsObjectNotFoundError reports whether err's tree contains an *ObjectNotFoundError.
func IsObjectNotFoundError(err error) bool {
	_, ok := errors.AsType[*ObjectNotFoundError](err)
	return ok
}

// ObjectTooLargeError signals an object read past the caller's byte cap.
type ObjectTooLargeError struct{ MaxBytes int64 }

func (e *ObjectTooLargeError) Error() string {
	return "storage: object is larger than " + strconv.FormatInt(e.MaxBytes, 10) + " bytes"
}

// IsObjectTooLargeError reports whether err's tree contains an *ObjectTooLargeError.
func IsObjectTooLargeError(err error) bool {
	_, ok := errors.AsType[*ObjectTooLargeError](err)
	return ok
}

type cappedBody struct {
	rc   io.ReadCloser
	left int64
	max  int64
}

// CapReader wraps rc so reading past maxBytes fails with *ObjectTooLargeError; drivers use it for Open.
func CapReader(rc io.ReadCloser, maxBytes int64) io.ReadCloser {
	return &cappedBody{rc: rc, left: maxBytes, max: maxBytes}
}

func (c *cappedBody) Read(p []byte) (int, error) {
	if c.left <= 0 {
		var probe [1]byte
		if n, _ := c.rc.Read(probe[:]); n > 0 {
			return 0, &ObjectTooLargeError{MaxBytes: c.max}
		}
		return 0, io.EOF
	}
	if int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.rc.Read(p)
	c.left -= int64(n)
	return n, err
}

func (c *cappedBody) Close() error { return c.rc.Close() }

// TypeNotAllowedError signals a content type the purpose does not accept.
type TypeNotAllowedError struct {
	Purpose     string
	ContentType string
}

func (e *TypeNotAllowedError) Error() string {
	return "storage: " + e.Purpose + " does not accept " + e.ContentType
}

// IsTypeNotAllowedError reports whether err's tree contains a *TypeNotAllowedError.
func IsTypeNotAllowedError(err error) bool {
	_, ok := errors.AsType[*TypeNotAllowedError](err)
	return ok
}

// TooLargeError signals a declared size above the purpose's cap.
type TooLargeError struct {
	Purpose  string
	MaxBytes int64
}

func (e *TooLargeError) Error() string {
	return "storage: " + e.Purpose + " accepts at most " + strconv.FormatInt(e.MaxBytes, 10) + " bytes"
}

// IsTooLargeError reports whether err's tree contains a *TooLargeError.
func IsTooLargeError(err error) bool {
	_, ok := errors.AsType[*TooLargeError](err)
	return ok
}

// UploadNotFoundError signals an upload that does not exist for this project, is no longer pending, or has expired.
type UploadNotFoundError struct{ PublicID string }

func (e *UploadNotFoundError) Error() string {
	return "storage: upload " + e.PublicID + " not found or expired"
}

// IsUploadNotFoundError reports whether err's tree contains an *UploadNotFoundError.
func IsUploadNotFoundError(err error) bool {
	_, ok := errors.AsType[*UploadNotFoundError](err)
	return ok
}

// MismatchReason is the stable cause of an UploadMismatchError.
type MismatchReason string

// The causes of an UploadMismatchError.
const (
	MismatchMissing MismatchReason = "missing"
	MismatchSize    MismatchReason = "size"
	MismatchType    MismatchReason = "type"
	MismatchContent MismatchReason = "content"
)

// UploadMismatchError signals a stored object that does not match its declaration.
type UploadMismatchError struct {
	PublicID string
	Reason   MismatchReason
}

func (e *UploadMismatchError) Error() string {
	return "storage: upload " + e.PublicID + " does not match what was declared: " + string(e.Reason)
}

// IsUploadMismatchError reports whether err's tree contains an *UploadMismatchError.
func IsUploadMismatchError(err error) bool {
	_, ok := errors.AsType[*UploadMismatchError](err)
	return ok
}

// IsExpectedError reports whether err is a typed storage error a service returns as-is.
func IsExpectedError(err error) bool {
	return IsTypeNotAllowedError(err) || IsTooLargeError(err) || IsUploadNotFoundError(err) ||
		IsUploadMismatchError(err) || IsNotConfiguredError(err) || IsObjectTooLargeError(err)
}
