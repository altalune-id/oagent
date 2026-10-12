package mailer

import (
	"errors"
	"fmt"
)

// HeaderInjectionError reports a Message header field that contains CR or LF.
type HeaderInjectionError struct{ Field string }

func (e *HeaderInjectionError) Error() string {
	return fmt.Sprintf("mailer: header injection in %s: CR/LF not allowed", e.Field)
}

// IsHeaderInjectionError reports whether err's tree contains a *HeaderInjectionError.
func IsHeaderInjectionError(err error) bool {
	_, ok := errors.AsType[*HeaderInjectionError](err)
	return ok
}

// IncompleteMessageError reports a Message that lacks a required field.
type IncompleteMessageError struct{ Field string }

func (e *IncompleteMessageError) Error() string {
	return fmt.Sprintf("mailer: message %s required", e.Field)
}

// IsIncompleteMessageError reports whether err's tree contains an *IncompleteMessageError.
func IsIncompleteMessageError(err error) bool {
	_, ok := errors.AsType[*IncompleteMessageError](err)
	return ok
}

// InvalidAddressError reports a From or To that does not parse as an email address; the value is left out on purpose.
type InvalidAddressError struct{ Field string }

func (e *InvalidAddressError) Error() string {
	return fmt.Sprintf("mailer: message %s is not a valid address", e.Field)
}

// IsInvalidAddressError reports whether err's tree contains an *InvalidAddressError.
func IsInvalidAddressError(err error) bool {
	_, ok := errors.AsType[*InvalidAddressError](err)
	return ok
}

// MissingConfigError reports a required mailer setting left empty.
type MissingConfigError struct{ Field string }

func (e *MissingConfigError) Error() string {
	return fmt.Sprintf("mailer: %s required", e.Field)
}

// IsMissingConfigError reports whether err's tree contains a *MissingConfigError.
func IsMissingConfigError(err error) bool {
	_, ok := errors.AsType[*MissingConfigError](err)
	return ok
}

// InvalidConfigError reports a mailer setting that holds an unusable value.
type InvalidConfigError struct {
	Field  string
	Value  string
	Reason string
}

func (e *InvalidConfigError) Error() string {
	return fmt.Sprintf("mailer: %s %q: %s", e.Field, e.Value, e.Reason)
}

// IsInvalidConfigError reports whether err's tree contains an *InvalidConfigError.
func IsInvalidConfigError(err error) bool {
	_, ok := errors.AsType[*InvalidConfigError](err)
	return ok
}
