package apperror

import (
	"errors"
	"maps"
	"slices"
	"strings"
)

// AppError is the wire error envelope.
type AppError struct {
	code       Code
	message    string
	meta       map[string]string
	violations []Violation
	cause      error
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

// Violations returns a copy of the refused request fields, in the order they were found.
func (e *AppError) Violations() []Violation { return slices.Clone(e.violations) }

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

// FieldReason is why a request field was refused, sent as meta.reason with CodeValidation.
type FieldReason string

// The FieldReason values.
const (
	FieldRequired      FieldReason = "required"
	FieldTooShort      FieldReason = "too_short"
	FieldTooLong       FieldReason = "too_long"
	FieldInvalidFormat FieldReason = "invalid_format"
	FieldNotAllowed    FieldReason = "not_allowed"
	FieldTooMany       FieldReason = "too_many"
	FieldInvalid       FieldReason = "invalid"
)

// Violation is one refused request field; Param is the rule's limit, when it has one. It never holds the value.
type Violation struct {
	Field  string
	Reason FieldReason
	Param  string
}

// Invalid reports a malformed request field (a bad id, a missing value) under CodeValidation.
func Invalid(field string, reason FieldReason) *AppError {
	return New(CodeValidation, fieldMessage(field, reason)).
		WithMeta(map[string]string{"field": field, "reason": string(reason)})
}

// Validation reports refused request fields under CodeValidation, the first one in meta.
func Validation(violations []Violation) *AppError {
	if len(violations) == 0 {
		return New(CodeValidation, "request is invalid")
	}
	first := violations[0]
	meta := map[string]string{"field": first.Field, "reason": string(first.Reason)}
	if first.Param != "" {
		meta["param"] = first.Param
	}
	ae := New(CodeValidation, fieldMessage(first.Field, first.Reason)).WithMeta(meta)
	ae.violations = slices.Clone(violations)
	return ae
}

func fieldMessage(field string, reason FieldReason) string {
	phrase, ok := reasonPhrases[reason]
	if !ok {
		phrase = "is " + string(reason)
	}
	return strings.ReplaceAll(field, "_", " ") + " " + phrase
}

//nolint:gochecknoglobals // a fixed lookup table.
var reasonPhrases = map[FieldReason]string{
	FieldRequired:      "is required",
	FieldTooShort:      "is too short",
	FieldTooLong:       "is too long",
	FieldInvalidFormat: "is not in the right format",
	FieldNotAllowed:    "is not an allowed value",
	FieldTooMany:       "has too many items",
	FieldInvalid:       "is invalid",
}
