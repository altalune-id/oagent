package apperror

import (
	"errors"
	"maps"
)

// AppError is the wire error envelope.
type AppError struct {
	code    Code
	message string
	meta    map[string]string
	cause   error
}

// New builds an AppError.
func New(code Code, message string) *AppError {
	return &AppError{code: code, message: message}
}

func (e *AppError) Error() string { return e.message }

// Code returns the stable error code.
func (e *AppError) Code() Code { return e.code }

// Message returns the human-facing message.
func (e *AppError) Message() string { return e.message }

// Unwrap exposes the cause for errors.Is/As.
func (e *AppError) Unwrap() error { return e.cause }

// Meta returns a defensive copy of the envelope metadata.
func (e *AppError) Meta() map[string]string {
	out := make(map[string]string, len(e.meta))
	maps.Copy(out, e.meta)
	return out
}

// WithCause returns a copy carrying cause; nil is a no-op.
func (e *AppError) WithCause(cause error) *AppError {
	if cause == nil {
		return e
	}
	cp := *e
	cp.cause = cause
	return &cp
}

// WithMeta returns a copy merged with meta; empty is a no-op.
func (e *AppError) WithMeta(meta map[string]string) *AppError {
	if len(meta) == 0 {
		return e
	}
	cp := *e
	cp.meta = make(map[string]string, len(e.meta)+len(meta))
	maps.Copy(cp.meta, e.meta)
	maps.Copy(cp.meta, meta)
	return &cp
}

// AsAppError walks the error chain returning the first *AppError, directly or via ToAppError() *AppError.
func AsAppError(err error) (*AppError, bool) {
	for e := err; e != nil; e = errors.Unwrap(e) {
		//nolint:errorlint // hop-by-hop walk: need per-layer discovery of ToAppError producers.
		if ae, ok := e.(*AppError); ok {
			return ae, true
		}
		//nolint:errorlint // same rationale — checking the current hop, not the whole chain.
		if p, ok := e.(interface{ ToAppError() *AppError }); ok {
			return p.ToAppError(), true
		}
	}
	return nil, false
}
