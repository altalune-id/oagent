package pg

import (
	"errors"
	"fmt"
)

// InvalidIdentifierError reports a name that is not usable as a Postgres identifier.
type InvalidIdentifierError struct {
	Name   string
	Reason string
}

func (e *InvalidIdentifierError) Error() string {
	return fmt.Sprintf("pg: invalid identifier %q: %s", e.Name, e.Reason)
}

// IsInvalidIdentifierError reports whether err wraps an *InvalidIdentifierError.
func IsInvalidIdentifierError(err error) bool {
	_, ok := errors.AsType[*InvalidIdentifierError](err)
	return ok
}

// LoginRoleError reports a connection that logged in as a role other than the configured one.
type LoginRoleError struct {
	Want string
	Got  string
}

func (e *LoginRoleError) Error() string {
	return fmt.Sprintf("pg: logged in as %q, but the role is %q: log in as that role; a SET ROLE does not survive a pooler", e.Got, e.Want)
}

// IsLoginRoleError reports whether err wraps a *LoginRoleError.
func IsLoginRoleError(err error) bool {
	_, ok := errors.AsType[*LoginRoleError](err)
	return ok
}
