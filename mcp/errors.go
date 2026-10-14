package mcp

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"altalune.id/oagent/apperror"
)

// DefaultUnmappedCode is the Code for a tool failure no ErrorMapper claimed.
const DefaultUnmappedCode = string(apperror.CodeUnexpected)

const unmappedMessage = "unexpected error"

const decoderPrefix = "proto:"

const (
	maxDetailBytes = 200
	ellipsis       = "…"
)

var decodedField = regexp.MustCompile(`(?:unknown field "([^"]+)"|invalid value for \S+ field (\S+?):)`)

// ErrorPayload is the JSON body of a failed tool call.
type ErrorPayload struct {
	Code       string            `json:"code"`
	Message    string            `json:"message"`
	Meta       map[string]string `json:"meta,omitempty"`
	Violations []Violation       `json:"violations,omitempty"`
	RequestID  string            `json:"request_id,omitempty"`
	TraceID    string            `json:"trace_id,omitempty"`
}

// Violation is one refused request field of a failed tool call; it never carries the field's value.
type Violation struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
	Param  string `json:"param,omitempty"`
}

// ScopeDeniedError reports a call missing Scope, one of the scopes the tool needs.
type ScopeDeniedError struct {
	Tool  string
	Scope string
}

func (e *ScopeDeniedError) Error() string {
	return fmt.Sprintf("mcp: tool %q requires scope %q", e.Tool, e.Scope)
}

// IsScopeDeniedError reports whether err is a ScopeDeniedError.
func IsScopeDeniedError(err error) bool {
	var target *ScopeDeniedError
	return errors.As(err, &target)
}

// ScopeUndeclaredError reports a tool registered without Scopes (always denied).
type ScopeUndeclaredError struct {
	Tool string
}

func (e *ScopeUndeclaredError) Error() string {
	return fmt.Sprintf("mcp: tool %q declares no scope", e.Tool)
}

// IsScopeUndeclaredError reports whether err is a ScopeUndeclaredError.
func IsScopeUndeclaredError(err error) bool {
	var target *ScopeUndeclaredError
	return errors.As(err, &target)
}

// InvalidArgumentsError reports tool arguments that do not decode.
type InvalidArgumentsError struct {
	Tool   string
	Field  string
	Reason string
}

func (e *InvalidArgumentsError) Error() string {
	return fmt.Sprintf("mcp: tool %q: invalid arguments: %s", e.Tool, e.Reason)
}

// IsInvalidArgumentsError reports whether err is an InvalidArgumentsError.
func IsInvalidArgumentsError(err error) bool {
	var target *InvalidArgumentsError
	return errors.As(err, &target)
}

// NewInvalidArgumentsError builds the InvalidArgumentsError for tool from the decoder's failure.
func NewInvalidArgumentsError(tool string, cause error) *InvalidArgumentsError {
	reason := strings.TrimLeftFunc(strings.TrimPrefix(cause.Error(), decoderPrefix), unicode.IsSpace)
	e := &InvalidArgumentsError{Tool: tool, Reason: capped(reason)}
	m := decodedField.FindStringSubmatch(reason)
	if m == nil {
		return e
	}
	e.Field = capped(m[1] + m[2])
	return e
}

func capped(s string) string {
	if len(s) <= maxDetailBytes {
		return s
	}
	cut := maxDetailBytes - len(ellipsis)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + ellipsis
}
